package repository

import (
	"context"

	"gorm.io/gorm"

	"github.com/edwinhuish/gowork/internal/model"
)

// CustomsDeclarationFilter 是报关单的查询条件。
//
// ShipmentID 是主维度：报关单总挂在某一票货下被查看与维护。
type CustomsDeclarationFilter struct {
	ShipmentID *uint
	Status     *model.CustomsStatus
}

func (f CustomsDeclarationFilter) scopes() []Scope {
	scopes := make([]Scope, 0, 2)
	if f.ShipmentID != nil {
		sid := *f.ShipmentID
		scopes = append(scopes, func(db *gorm.DB) *gorm.DB {
			return db.Where("shipment_id = ?", sid)
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

type CustomsDeclarationRepository interface {
	Create(ctx context.Context, m *model.CustomsDeclaration) error
	Update(ctx context.Context, id uint, fields map[string]any) error
	Delete(ctx context.Context, id uint) error
	FindByID(ctx context.Context, id uint) (*model.CustomsDeclaration, error)
	List(
		ctx context.Context, f CustomsDeclarationFilter, p Pagination,
	) ([]model.CustomsDeclaration, int64, error)
	// CountByShipmentID 统计一票货挂了多少张报关单，供删除出货批次前判断引用。
	CountByShipmentID(ctx context.Context, shipmentID uint) (int64, error)
}

type customsDeclarationRepo struct {
	*Base[model.CustomsDeclaration]
}

func NewCustomsDeclarationRepository(db *gorm.DB) CustomsDeclarationRepository {
	return &customsDeclarationRepo{Base: NewBase[model.CustomsDeclaration](db)}
}

func (r *customsDeclarationRepo) Create(ctx context.Context, m *model.CustomsDeclaration) error {
	return r.Base.Create(ctx, m)
}

func (r *customsDeclarationRepo) Update(ctx context.Context, id uint, fields map[string]any) error {
	return r.Base.UpdateFields(ctx, id, fields)
}

func (r *customsDeclarationRepo) Delete(ctx context.Context, id uint) error {
	return r.Base.DeleteByID(ctx, id)
}

func (r *customsDeclarationRepo) FindByID(
	ctx context.Context, id uint,
) (*model.CustomsDeclaration, error) {
	return r.Base.FindByID(ctx, id)
}

// List 按申报日期升序、再按 id 升序：同一票货的多张报关单，
// 读者想按"先报哪张"的顺序看，而不是按录入顺序。
// 日期为空的（还没申报）排在最后——NULL 在升序里本来就在最后，不用额外处理。
func (r *customsDeclarationRepo) List(
	ctx context.Context, f CustomsDeclarationFilter, p Pagination,
) ([]model.CustomsDeclaration, int64, error) {
	return r.PaginateOrdered(ctx, p, "decl_date ASC, id ASC", f.scopes()...)
}

func (r *customsDeclarationRepo) CountByShipmentID(
	ctx context.Context, shipmentID uint,
) (int64, error) {
	return r.Base.Count(ctx, func(db *gorm.DB) *gorm.DB {
		return db.Where("shipment_id = ?", shipmentID)
	})
}
