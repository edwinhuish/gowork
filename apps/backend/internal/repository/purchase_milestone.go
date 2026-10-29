package repository

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/edwinhuish/gowork/internal/model"
)

// PurchaseMilestoneRepository 是采购跟单节点的持久化入口（T-38）。
type PurchaseMilestoneRepository interface {
	// ListByOrder 取一张采购单的全部节点（按行、按节点顺序）。
	ListByOrder(ctx context.Context, orderID uint) ([]model.PurchaseMilestone, error)
	// Upsert 按 (行, 节点) 建或更新；同一事务，部分补录不会动到别的节点。
	Upsert(ctx context.Context, rows []model.PurchaseMilestone) error
	// OverdueNodes 取这些采购单里"计划日已过但还没完成"的节点，
	// 每张单只留**最靠前**的那一个（最该先处理的），供列表投影。
	OverdueNodes(
		ctx context.Context, orderIDs []uint, asOf time.Time,
	) (map[uint]model.PurchaseNode, error)
}

type purchaseMilestoneRepo struct {
	*Base[model.PurchaseMilestone]
}

func NewPurchaseMilestoneRepository(db *gorm.DB) PurchaseMilestoneRepository {
	return &purchaseMilestoneRepo{Base: NewBase[model.PurchaseMilestone](db)}
}

func (r *purchaseMilestoneRepo) ListByOrder(
	ctx context.Context, orderID uint,
) ([]model.PurchaseMilestone, error) {
	list := make([]model.PurchaseMilestone, 0, 16)
	err := r.conn(ctx).
		Where("purchase_order_id = ?", orderID).
		Order("purchase_order_item_id ASC, node ASC").
		Find(&list).Error
	if err != nil {
		return nil, fmt.Errorf("查询采购 %d 的跟单节点: %w", orderID, err)
	}
	return list, nil
}

func (r *purchaseMilestoneRepo) Upsert(
	ctx context.Context, rows []model.PurchaseMilestone,
) error {
	if len(rows) == 0 {
		return nil
	}
	err := r.conn(ctx).Transaction(func(tx *gorm.DB) error {
		for i := range rows {
			row := &rows[i]
			var existing model.PurchaseMilestone
			err := tx.Where("purchase_order_item_id = ? AND node = ?",
				row.PurchaseOrderItemID, row.Node).First(&existing).Error
			switch {
			case err == nil:
				// 只更新业务字段：主键、归属与软删标记一律以库里为准。
				if err := tx.Model(&model.PurchaseMilestone{}).
					Where("id = ?", existing.ID).
					Updates(map[string]any{
						"plan_date": row.PlanDate,
						"done_date": row.DoneDate,
						"operator":  row.Operator,
						"remark":    row.Remark,
					}).Error; err != nil {
					return err
				}
				row.ID = existing.ID
			case err == gorm.ErrRecordNotFound:
				if err := tx.Create(row).Error; err != nil {
					return err
				}
			default:
				return err
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("保存跟单节点: %w", err)
	}
	return nil
}

func (r *purchaseMilestoneRepo) OverdueNodes(
	ctx context.Context, orderIDs []uint, asOf time.Time,
) (map[uint]model.PurchaseNode, error) {
	out := make(map[uint]model.PurchaseNode, len(orderIDs))
	if len(orderIDs) == 0 {
		return out, nil
	}

	var rows []struct {
		PurchaseOrderID uint
		Node            model.PurchaseNode
	}
	err := r.conn(ctx).
		Model(&model.PurchaseMilestone{}).
		Select("purchase_order_id, node").
		Where("purchase_order_id IN ?", orderIDs).
		Where("done_date IS NULL").
		Where("plan_date IS NOT NULL AND plan_date < ?", asOf).
		Order("purchase_order_id ASC, node ASC").
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("查询逾期跟单节点: %w", err)
	}
	// 按 node 升序扫出来，第一行就是这张单最靠前的逾期节点。
	for _, row := range rows {
		if _, ok := out[row.PurchaseOrderID]; !ok {
			out[row.PurchaseOrderID] = row.Node
		}
	}
	return out, nil
}
