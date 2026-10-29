package repository

import (
	"context"
	"fmt"
	"strings"

	"gorm.io/gorm"

	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/errs"
)

// partnerRef 是一张引用主体 id 的表，以及那一列的名字。
type partnerRef struct {
	table  string
	column string
}

// partnerRefTables 是合并时必须整体迁过去的引用。
//
// 这张清单是合并的唯一事实来源：新增任何一个指向 md_partner 的外键，都要在这里补一行，
// 否则那条数据会留在被软删的主体上，表现是"某张单的客户变成了空白"。
// 清单与 model 里的 foreignKey tag 一一对应，改 model 时同步看一眼这里。
//
// 附件不在这里：它用 (entity_type, entity_id) 两列定位，条件不一样，单独处理。
var partnerRefTables = []partnerRef{
	{"crm_partner_contact", "partner_id"},
	{"crm_activity", "partner_id"},
	{"crm_lead", "partner_id"},
	{"crm_opportunity", "partner_id"},
	{"biz_quotation", "partner_id"},
	{"biz_sales_order", "partner_id"},
	{"biz_purchase_order", "supplier_id"},
	{"biz_contract", "partner_id"},
	{"biz_receivable", "partner_id"},
	{"biz_payable", "partner_id"},
	{"biz_payment", "partner_id"},
	{"biz_shipment_charge", "partner_id"},
	{"inv_stock_document", "partner_id"},
	{"md_product", "supplier_id"},
	{"md_product_source", "partner_id"},
}

// PartnerMergePlan 是一次合并要执行的全部写入：由 service 决定做什么，
// 仓储只负责在一个事务里做完。
type PartnerMergePlan struct {
	SurvivorID uint
	SourceID   uint
	// Fields 是写到主记录上的字段（角色并集 + 从被合并方补来的空字段），
	// 可以带 FieldUpdatedAt 走乐观锁。
	Fields map[string]any
}

// Merge 在一个事务里把被合并方的全部引用迁到主记录，然后软删被合并方。
//
// 为什么是一个事务：中途失败留下"一半引用指向被删主体"的状态，比整笔不合并难查得多
// ——界面上看不到那条主体了，单据里却还挂着一个查不到的 id。
func (r *partnerRepo) Merge(ctx context.Context, plan PartnerMergePlan) (map[string]int64, error) {
	moved := make(map[string]int64, len(partnerRefTables)+1)

	err := dbFor(ctx, r.db).Transaction(func(tx *gorm.DB) error {
		// 主联系人：一个主体只能有一个。主记录已经有了，就把被合并方那个降级为普通联系人；
		// 主记录没有则让被合并方的顶上来——合并之后这家客户不该突然没有主联系人。
		var primaries int64
		if err := tx.Table("crm_partner_contact").
			Where("partner_id = ? AND is_primary = ? AND deleted_at = 0", plan.SurvivorID, true).
			Count(&primaries).Error; err != nil {
			return fmt.Errorf("统计主体 %d 的主联系人: %w", plan.SurvivorID, err)
		}

		// 降级必须发生在联系人迁移之前：迁移完再降级，就分不清哪条是谁带过来的了。
		if primaries > 0 {
			if err := demotePrimaryContacts(tx, plan.SourceID); err != nil {
				return err
			}
		}

		for _, ref := range partnerRefTables {
			res := tx.Table(ref.table).Where(ref.column+" = ?", plan.SourceID).
				Update(ref.column, plan.SurvivorID)
			if res.Error != nil {
				return fmt.Errorf("迁移 %s.%s: %w", ref.table, ref.column, res.Error)
			}
			if res.RowsAffected > 0 {
				moved[ref.table] = res.RowsAffected
			}
		}

		// 附件只认 (entity_type, entity_id)，不带外键，所以不在上面的清单里。
		res := tx.Table("sys_attachment").
			Where("entity_type = ? AND entity_id = ?", model.AttachPartner, plan.SourceID).
			Update("entity_id", plan.SurvivorID)
		if res.Error != nil {
			return fmt.Errorf("迁移主体附件: %w", res.Error)
		}
		if res.RowsAffected > 0 {
			moved["sys_attachment"] = res.RowsAffected
		}

		// 主记录：与编辑同一套乐观锁，两个人同时合并/编辑时后一个应当被拒绝。
		q := tx.Model(&model.Partner{}).Where("id = ?", plan.SurvivorID)
		expected, hasLock := takeOptimisticLock(plan.Fields)
		if hasLock {
			q = q.Where("updated_at = ?", expected)
		}
		updated := q.Updates(plan.Fields)
		if updated.Error != nil {
			return fmt.Errorf("更新主记录 %d: %w", plan.SurvivorID, updated.Error)
		}
		if updated.RowsAffected == 0 {
			if hasLock {
				return errs.ErrConflict.WithMsg("这条主体已被他人修改，请刷新后重试")
			}
			return errs.ErrNotFound
		}

		// 被合并方软删而不是硬删：合并是"这次判断它是一家"，判断可能会错，
		// 硬删之后没法把引用再搬回去。恢复入口仍在列表里。
		if err := tx.Delete(&model.Partner{}, plan.SourceID).Error; err != nil {
			return fmt.Errorf("软删被合并的主体 %d: %w", plan.SourceID, err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return moved, nil
}

func demotePrimaryContacts(tx *gorm.DB, partnerID uint) error {
	err := tx.Table("crm_partner_contact").
		Where("partner_id = ? AND is_primary = ?", partnerID, true).
		Update("is_primary", false).Error
	if err != nil {
		return fmt.Errorf("降级主体 %d 的主联系人: %w", partnerID, err)
	}
	return nil
}

// ProductSourceConflicts 找出"合并后会在同一产品上撞成两条同货号"的货源。
//
// 只读，供 service 在动手之前拦下来：货源是报价的成本来源，自动丢掉其中一条，
// 等于悄悄改了某张报价的成本依据，而那张报价可能已经发给客户了。
// 冲突交给人工处理——通常是先看一眼两家的报价哪个还算数。
func (r *partnerRepo) ProductSourceConflicts(
	ctx context.Context, survivorID, sourceID uint,
) ([]string, error) {
	survivor, err := r.sourceRowsOf(ctx, survivorID)
	if err != nil {
		return nil, err
	}
	source, err := r.sourceRowsOf(ctx, sourceID)
	if err != nil {
		return nil, err
	}

	// 键是「产品 + 归一化货号」：与 ProductSourceRepository.ExistsBySupplierSKU 同一口径。
	type key struct {
		productID uint
		skuNorm   string
	}
	taken := make(map[key]bool, len(survivor))
	for _, row := range survivor {
		taken[key{row.ProductID, row.SKUNorm}] = true
	}

	conflicts := make([]string, 0, 4)
	for _, row := range source {
		if !taken[key{row.ProductID, row.SKUNorm}] {
			continue
		}
		sku := strings.TrimSpace(row.SupplierSKU)
		if sku == "" {
			sku = "（空货号）"
		}
		conflicts = append(conflicts, fmt.Sprintf("%s / %s", row.ProductSKU, sku))
	}
	return conflicts, nil
}

type productSourceRow struct {
	ProductID   uint
	ProductSKU  string
	SupplierSKU string
	SKUNorm     string
}

func (r *partnerRepo) sourceRowsOf(ctx context.Context, partnerID uint) ([]productSourceRow, error) {
	var rows []productSourceRow
	err := dbFor(ctx, r.db).
		Table("md_product_source AS s").
		Select("s.product_id, p.sku_code AS product_sku, s.supplier_sku, s.supplier_sku_norm AS sku_norm").
		Joins("JOIN md_product AS p ON p.id = s.product_id").
		Where("s.partner_id = ? AND s.deleted_at = 0", partnerID).
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("查询主体 %d 的产品货源: %w", partnerID, err)
	}
	return rows, nil
}
