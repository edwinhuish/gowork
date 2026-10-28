package repository

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/errs"
)

// ProductSourceFilter 是产品货源的查询条件。
type ProductSourceFilter struct {
	ProductID *uint
	PartnerID *uint
	// Keyword 只搜供应商货号与备注：产品名在货源行里是冗余展示，
	// 按产品名搜应该回到产品列表，两个入口搜同一件事会让人分不清自己在哪。
	Keyword string
	Status  *model.EnableStatus
}

func (f ProductSourceFilter) scopes() []Scope {
	scopes := make([]Scope, 0, 4)
	if f.ProductID != nil {
		pid := *f.ProductID
		scopes = append(scopes, func(db *gorm.DB) *gorm.DB {
			return db.Where("product_id = ?", pid)
		})
	}
	if f.PartnerID != nil {
		sid := *f.PartnerID
		scopes = append(scopes, func(db *gorm.DB) *gorm.DB {
			return db.Where("partner_id = ?", sid)
		})
	}
	if f.Keyword != "" {
		scopes = append(scopes, likeAny(f.Keyword, "supplier_sku", "remark"))
	}
	if f.Status != nil {
		st := *f.Status
		scopes = append(scopes, func(db *gorm.DB) *gorm.DB {
			return db.Where("status = ?", st)
		})
	}
	return scopes
}

type ProductSourceRepository interface {
	Create(ctx context.Context, m *model.ProductSource) error
	Update(ctx context.Context, id uint, fields map[string]any) error
	Delete(ctx context.Context, id uint) error
	FindByID(ctx context.Context, id uint) (*model.ProductSource, error)
	List(ctx context.Context, f ProductSourceFilter, p Pagination) ([]model.ProductSource, int64, error)
	// ExistsBySupplierSKU 判断"同一产品在同一供应商下是否已有这个货号"。
	//
	// 查重维度是「产品 + 供应商 + 货号」而不是全局货号：我方一个 SKU 拆成
	// 散装与彩盒两个编码、都从同一家同一个货号进货，是真实存在的做法；
	// 按全局货号查重会把这种拆分判成重复录入。
	//
	// 货号传空串时不跳过检查：同产品同供应商两条空货号的行也是重复录入。
	ExistsBySupplierSKU(
		ctx context.Context, productID, partnerID uint, supplierSKU string, excludeID uint,
	) (bool, error)
	// CountByProductID 统计某产品下的货源条数，供产品删除前判断有没有引用。
	CountByProductID(ctx context.Context, productID uint) (int64, error)
	// FindCheapestByProductID 取该产品本币价最低的**启用**货源，供报价单落成本快照。
	//
	// 取最低价而不是"产品上的默认供应商"：报价时要比的就是"现在从谁家进最便宜"，
	// 默认供应商是会过期的历史选择（旺季分单、对方已经涨价）。
	// 找不到返回 errs.ErrNotFound，由调用方给出"请先维护货源"的提示。
	FindCheapestByProductID(ctx context.Context, productID uint) (*model.ProductSource, error)
}

type productSourceRepo struct {
	*Base[model.ProductSource]
}

func NewProductSourceRepository(db *gorm.DB) ProductSourceRepository {
	return &productSourceRepo{Base: NewBase[model.ProductSource](db)}
}

func (r *productSourceRepo) Create(ctx context.Context, m *model.ProductSource) error {
	return r.Base.Create(ctx, m)
}

func (r *productSourceRepo) Update(ctx context.Context, id uint, fields map[string]any) error {
	return r.Base.UpdateFields(ctx, id, fields)
}

func (r *productSourceRepo) Delete(ctx context.Context, id uint) error {
	return r.Base.DeleteByID(ctx, id)
}

func (r *productSourceRepo) FindByID(ctx context.Context, id uint) (*model.ProductSource, error) {
	var m model.ProductSource
	err := r.DB().WithContext(ctx).Preload("Product").Preload("Partner").First(&m, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errs.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("查询产品货源 id=%d: %w", id, err)
	}
	return &m, nil
}

// List 按本币价升序：这张表的第一用途是比价，最便宜的排前面。
// 同价时按 id 兜底，避免翻页时两条记录在页边界反复横跳。
func (r *productSourceRepo) List(
	ctx context.Context, f ProductSourceFilter, p Pagination,
) ([]model.ProductSource, int64, error) {
	scopes := append(f.scopes(), func(db *gorm.DB) *gorm.DB {
		return db.Preload("Product").Preload("Partner")
	})
	return r.PaginateOrdered(ctx, p, "purchase_price_base ASC, id ASC", scopes...)
}

func (r *productSourceRepo) ExistsBySupplierSKU(
	ctx context.Context, productID, partnerID uint, supplierSKU string, excludeID uint,
) (bool, error) {
	return r.Exists(ctx, func(db *gorm.DB) *gorm.DB {
		q := db.Where("product_id = ? AND partner_id = ? AND supplier_sku = ?",
			productID, partnerID, supplierSKU)
		if excludeID > 0 {
			q = q.Where("id <> ?", excludeID)
		}
		return q
	})
}

func (r *productSourceRepo) CountByProductID(ctx context.Context, productID uint) (int64, error) {
	return r.Base.Count(ctx, func(db *gorm.DB) *gorm.DB {
		return db.Where("product_id = ?", productID)
	})
}

func (r *productSourceRepo) FindCheapestByProductID(
	ctx context.Context, productID uint,
) (*model.ProductSource, error) {
	var m model.ProductSource
	err := r.DB().WithContext(ctx).
		Preload("Partner").
		Where("product_id = ? AND status = ?", productID, model.EnableStatusOn).
		Order("purchase_price_base ASC, id ASC").
		First(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errs.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("查询产品 id=%d 的最低货源: %w", productID, err)
	}
	return &m, nil
}
