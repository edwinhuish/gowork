package repository

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/errs"
	"github.com/edwinhuish/gowork/internal/pkg/money"
)

// ShipmentChargeFilter 是运费杂费的查询条件。
//
// ShipmentID 是主维度：费用总是"挂在这一票货下"被查看与维护。
// 单独按类型筛（Type）留给将来的费用分析，现在只有面板在用，但仍支持。
type ShipmentChargeFilter struct {
	ShipmentID *uint
	Type       *model.ShipmentChargeType
}

func (f ShipmentChargeFilter) scopes() []Scope {
	scopes := make([]Scope, 0, 2)
	if f.ShipmentID != nil {
		sid := *f.ShipmentID
		scopes = append(scopes, func(db *gorm.DB) *gorm.DB {
			return db.Where("shipment_id = ?", sid)
		})
	}
	if f.Type != nil {
		t := *f.Type
		scopes = append(scopes, func(db *gorm.DB) *gorm.DB {
			return db.Where("type = ?", t)
		})
	}
	return scopes
}

type ShipmentChargeRepository interface {
	Create(ctx context.Context, m *model.ShipmentCharge) error
	Update(ctx context.Context, id uint, fields map[string]any) error
	Delete(ctx context.Context, id uint) error
	FindByID(ctx context.Context, id uint) (*model.ShipmentCharge, error)
	List(ctx context.Context, f ShipmentChargeFilter, p Pagination) ([]model.ShipmentCharge, int64, error)
	// SumBaseByShipmentID 是一票货的本币费用合计，供出货详情与单票毛利（T-22）使用。
	//
	// 用 SQL 求和而不是把列表取回来在内存里加：费用可能几十条、也可能分页，
	// 取回来再算就要先保证"取全了"，而分页列表恰恰不保证。
	SumBaseByShipmentID(ctx context.Context, shipmentID uint) (money.Decimal, error)
	// CountByShipmentID 统计一票货挂了多少条费用，供删除出货批次前判断有没有引用。
	CountByShipmentID(ctx context.Context, shipmentID uint) (int64, error)
}

type shipmentChargeRepo struct {
	*Base[model.ShipmentCharge]
}

func NewShipmentChargeRepository(db *gorm.DB) ShipmentChargeRepository {
	return &shipmentChargeRepo{Base: NewBase[model.ShipmentCharge](db)}
}

func (r *shipmentChargeRepo) Create(ctx context.Context, m *model.ShipmentCharge) error {
	return r.Base.Create(ctx, m)
}

func (r *shipmentChargeRepo) Update(ctx context.Context, id uint, fields map[string]any) error {
	return r.Base.UpdateFields(ctx, id, fields)
}

func (r *shipmentChargeRepo) Delete(ctx context.Context, id uint) error {
	return r.Base.DeleteByID(ctx, id)
}

func (r *shipmentChargeRepo) FindByID(ctx context.Context, id uint) (*model.ShipmentCharge, error) {
	var m model.ShipmentCharge
	err := r.DB().WithContext(ctx).Preload("Partner").First(&m, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errs.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("查询运费杂费 id=%d: %w", id, err)
	}
	return &m, nil
}

// List 按类型升序、再按 id 升序：同一类费用排在一起，像一张货代账单。
// 按录入时间倒序的读法是"我刚才录了什么"，而看这张表的人想知道的是"钱花在哪"。
func (r *shipmentChargeRepo) List(
	ctx context.Context, f ShipmentChargeFilter, p Pagination,
) ([]model.ShipmentCharge, int64, error) {
	scopes := append(f.scopes(), func(db *gorm.DB) *gorm.DB {
		return db.Preload("Partner")
	})
	return r.PaginateOrdered(ctx, p, "type ASC, id ASC", scopes...)
}

func (r *shipmentChargeRepo) CountByShipmentID(ctx context.Context, shipmentID uint) (int64, error) {
	return r.Base.Count(ctx, func(db *gorm.DB) *gorm.DB {
		return db.Where("shipment_id = ?", shipmentID)
	})
}

func (r *shipmentChargeRepo) SumBaseByShipmentID(
	ctx context.Context, shipmentID uint,
) (money.Decimal, error) {
	var row struct{ Total money.Decimal }
	err := r.DB().WithContext(ctx).
		Model(&model.ShipmentCharge{}).
		Where("shipment_id = ?", shipmentID).
		Select("COALESCE(SUM(amount_base), 0) AS total").
		Scan(&row).Error
	if err != nil {
		return money.Zero(), fmt.Errorf("汇总出货批次 id=%d 的费用: %w", shipmentID, err)
	}
	return row.Total, nil
}
