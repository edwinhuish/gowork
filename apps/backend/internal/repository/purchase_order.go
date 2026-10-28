package repository

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/errs"
)

// PurchaseOrderFilter 是采购订单列表的查询条件。
type PurchaseOrderFilter struct {
	Keyword    string
	SupplierID *uint
	Status     *model.DocumentStatus
}

func (f PurchaseOrderFilter) scopes() []Scope {
	scopes := make([]Scope, 0, 3)
	if f.Keyword != "" {
		scopes = append(scopes, likeAny(f.Keyword, "doc_no", "remark"))
	}
	if f.SupplierID != nil {
		sid := *f.SupplierID
		scopes = append(scopes, func(db *gorm.DB) *gorm.DB {
			return db.Where("supplier_id = ?", sid)
		})
	}
	if f.Status != nil {
		st := *f.Status
		scopes = append(scopes, func(db *gorm.DB) *gorm.DB {
			return db.Where("status = ?", st)
		})
	}
	return scopes
}

type PurchaseOrderRepository interface {
	CreateWithItems(ctx context.Context, o *model.PurchaseOrder, items []model.PurchaseOrderItem) error
	UpdateWithItems(ctx context.Context, id uint, fields map[string]any, items []model.PurchaseOrderItem) error
	Update(ctx context.Context, id uint, fields map[string]any) error
	Delete(ctx context.Context, id uint) error
	FindByID(ctx context.Context, id uint) (*model.PurchaseOrder, error)
	List(ctx context.Context, f PurchaseOrderFilter, p Pagination) ([]model.PurchaseOrder, int64, error)
}

type purchaseOrderRepo struct {
	*Base[model.PurchaseOrder]
}

func NewPurchaseOrderRepository(db *gorm.DB) PurchaseOrderRepository {
	return &purchaseOrderRepo{Base: NewBase[model.PurchaseOrder](db)}
}

func (r *purchaseOrderRepo) CreateWithItems(
	ctx context.Context, o *model.PurchaseOrder, items []model.PurchaseOrderItem,
) error {
	err := r.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(o).Error; err != nil {
			return err
		}
		if len(items) == 0 {
			return nil
		}
		for i := range items {
			items[i].PurchaseOrderID = o.ID
		}
		return tx.Create(&items).Error
	})
	if err != nil {
		return fmt.Errorf("创建采购订单: %w", err)
	}
	o.Items = items
	return nil
}

func (r *purchaseOrderRepo) UpdateWithItems(
	ctx context.Context, id uint, fields map[string]any, items []model.PurchaseOrderItem,
) error {
	err := r.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&model.PurchaseOrder{}).Where("id = ?", id).Updates(fields)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return errs.ErrNotFound
		}
		if err := tx.Where("purchase_order_id = ?", id).
			Delete(&model.PurchaseOrderItem{}).Error; err != nil {
			return err
		}
		if len(items) == 0 {
			return nil
		}
		for i := range items {
			items[i].PurchaseOrderID = id
		}
		return tx.Create(&items).Error
	})
	if err != nil {
		if errors.Is(err, errs.ErrNotFound) {
			return errs.ErrNotFound
		}
		return fmt.Errorf("更新采购订单 id=%d: %w", id, err)
	}
	return nil
}

func (r *purchaseOrderRepo) Update(ctx context.Context, id uint, fields map[string]any) error {
	return r.Base.UpdateFields(ctx, id, fields)
}

func (r *purchaseOrderRepo) Delete(ctx context.Context, id uint) error {
	return r.Base.DeleteByID(ctx, id)
}

func (r *purchaseOrderRepo) FindByID(ctx context.Context, id uint) (*model.PurchaseOrder, error) {
	var m model.PurchaseOrder
	err := r.DB().WithContext(ctx).
		Preload("Supplier").
		Preload("Items", func(db *gorm.DB) *gorm.DB { return db.Order("id ASC") }).
		Preload("Items.Product").
		First(&m, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errs.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("查询采购订单 id=%d: %w", id, err)
	}
	return &m, nil
}

func (r *purchaseOrderRepo) List(
	ctx context.Context, f PurchaseOrderFilter, p Pagination,
) ([]model.PurchaseOrder, int64, error) {
	scopes := append(f.scopes(), func(db *gorm.DB) *gorm.DB {
		return db.Preload("Supplier")
	})
	return r.PaginateOrdered(ctx, p, "order_date DESC, id DESC", scopes...)
}
