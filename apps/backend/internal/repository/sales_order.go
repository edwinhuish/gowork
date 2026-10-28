package repository

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/errs"
)

// SalesOrderFilter 是销售订单列表的查询条件。
type SalesOrderFilter struct {
	// Keyword 匹配单号与备注；按客户找单走 partner_id。
	Keyword   string
	PartnerID *uint
	Status    *model.DocumentStatus
}

func (f SalesOrderFilter) scopes() []Scope {
	scopes := make([]Scope, 0, 3)
	if f.Keyword != "" {
		scopes = append(scopes, likeAny(f.Keyword, "doc_no", "remark"))
	}
	if f.PartnerID != nil {
		pid := *f.PartnerID
		scopes = append(scopes, func(db *gorm.DB) *gorm.DB {
			return db.Where("partner_id = ?", pid)
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

type SalesOrderRepository interface {
	CreateWithItems(ctx context.Context, o *model.SalesOrder, items []model.SalesOrderItem) error
	UpdateWithItems(ctx context.Context, id uint, fields map[string]any, items []model.SalesOrderItem) error
	Update(ctx context.Context, id uint, fields map[string]any) error
	Delete(ctx context.Context, id uint) error
	FindByID(ctx context.Context, id uint) (*model.SalesOrder, error)
	List(ctx context.Context, f SalesOrderFilter, p Pagination) ([]model.SalesOrder, int64, error)
	// FindItemByID 取销售订单明细行并带出所属订单，供采购单校验「以销定采」的挂接。
	// 采购单要确认的不只是行存在，还有"这张销售单还能不能被备货"——已取消的单不能再采。
	FindItemByID(ctx context.Context, itemID uint) (*model.SalesOrderItem, error)
}

type salesOrderRepo struct {
	*Base[model.SalesOrder]
}

func NewSalesOrderRepository(db *gorm.DB) SalesOrderRepository {
	return &salesOrderRepo{Base: NewBase[model.SalesOrder](db)}
}

func (r *salesOrderRepo) CreateWithItems(
	ctx context.Context, o *model.SalesOrder, items []model.SalesOrderItem,
) error {
	err := r.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(o).Error; err != nil {
			return err
		}
		if len(items) == 0 {
			return nil
		}
		for i := range items {
			items[i].SalesOrderID = o.ID
		}
		return tx.Create(&items).Error
	})
	if err != nil {
		return fmt.Errorf("创建销售订单: %w", err)
	}
	o.Items = items
	return nil
}

func (r *salesOrderRepo) UpdateWithItems(
	ctx context.Context, id uint, fields map[string]any, items []model.SalesOrderItem,
) error {
	err := r.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&model.SalesOrder{}).Where("id = ?", id).Updates(fields)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return errs.ErrNotFound
		}
		// 明细整体替换：行没有稳定身份，按 id 做增量更新会张冠李戴。
		if err := tx.Where("sales_order_id = ?", id).Delete(&model.SalesOrderItem{}).Error; err != nil {
			return err
		}
		if len(items) == 0 {
			return nil
		}
		for i := range items {
			items[i].SalesOrderID = id
		}
		return tx.Create(&items).Error
	})
	if err != nil {
		if errors.Is(err, errs.ErrNotFound) {
			return errs.ErrNotFound
		}
		return fmt.Errorf("更新销售订单 id=%d: %w", id, err)
	}
	return nil
}

func (r *salesOrderRepo) Update(ctx context.Context, id uint, fields map[string]any) error {
	return r.Base.UpdateFields(ctx, id, fields)
}

func (r *salesOrderRepo) Delete(ctx context.Context, id uint) error {
	return r.Base.DeleteByID(ctx, id)
}

func (r *salesOrderRepo) FindByID(ctx context.Context, id uint) (*model.SalesOrder, error) {
	var m model.SalesOrder
	err := r.DB().WithContext(ctx).
		Preload("Partner").
		Preload("Items", func(db *gorm.DB) *gorm.DB { return db.Order("id ASC") }).
		Preload("Items.Product").
		First(&m, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errs.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("查询销售订单 id=%d: %w", id, err)
	}
	return &m, nil
}

func (r *salesOrderRepo) FindItemByID(
	ctx context.Context, itemID uint,
) (*model.SalesOrderItem, error) {
	var m model.SalesOrderItem
	err := r.DB().WithContext(ctx).
		Preload("SalesOrder").
		First(&m, itemID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errs.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("查询销售订单明细 id=%d: %w", itemID, err)
	}
	return &m, nil
}

// List 按下单日倒序，同日按 id 倒序：业务上关心的是"最近下了什么单"。
func (r *salesOrderRepo) List(
	ctx context.Context, f SalesOrderFilter, p Pagination,
) ([]model.SalesOrder, int64, error) {
	scopes := append(f.scopes(), func(db *gorm.DB) *gorm.DB {
		return db.Preload("Partner")
	})
	return r.PaginateOrdered(ctx, p, "order_date DESC, id DESC", scopes...)
}
