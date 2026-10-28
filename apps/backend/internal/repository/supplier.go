package repository

import (
	"context"

	"gorm.io/gorm"

	"github.com/edwinhuish/gowork/internal/model"
)

type SupplierFilter struct {
	Keyword string
	Status  *model.EnableStatus
}

func (f SupplierFilter) scopes() []Scope {
	scopes := make([]Scope, 0, 2)

	if f.Keyword != "" {
		scopes = append(scopes, likeAny(f.Keyword, "code", "name", "contact"))
	}
	if f.Status != nil {
		st := *f.Status
		scopes = append(scopes, func(db *gorm.DB) *gorm.DB {
			return db.Where("status = ?", st)
		})
	}
	return scopes
}

type SupplierRepository interface {
	Create(ctx context.Context, m *model.Supplier) error
	Update(ctx context.Context, id uint, fields map[string]any) error
	Delete(ctx context.Context, id uint) error
	FindByID(ctx context.Context, id uint) (*model.Supplier, error)
	ExistsByCode(ctx context.Context, code string, excludeID uint) (bool, error)
	List(ctx context.Context, f SupplierFilter, p Pagination) ([]model.Supplier, int64, error)
	Count(ctx context.Context) (int64, error)
}

type supplierRepo struct {
	*Base[model.Supplier]
}

func NewSupplierRepository(db *gorm.DB) SupplierRepository {
	return &supplierRepo{Base: NewBase[model.Supplier](db)}
}

func (r *supplierRepo) Create(ctx context.Context, m *model.Supplier) error {
	return r.Base.Create(ctx, m)
}

func (r *supplierRepo) Update(ctx context.Context, id uint, fields map[string]any) error {
	return r.Base.UpdateFields(ctx, id, fields)
}

func (r *supplierRepo) Delete(ctx context.Context, id uint) error {
	return r.Base.DeleteByID(ctx, id)
}

func (r *supplierRepo) FindByID(ctx context.Context, id uint) (*model.Supplier, error) {
	return r.Base.FindByID(ctx, id)
}

func (r *supplierRepo) ExistsByCode(ctx context.Context, code string, excludeID uint) (bool, error) {
	return r.Exists(ctx, func(db *gorm.DB) *gorm.DB {
		q := db.Where("code = ?", code)
		if excludeID > 0 {
			q = q.Where("id <> ?", excludeID)
		}
		return q
	})
}

func (r *supplierRepo) List(ctx context.Context, f SupplierFilter, p Pagination) ([]model.Supplier, int64, error) {
	return r.Paginate(ctx, p, f.scopes()...)
}

func (r *supplierRepo) Count(ctx context.Context) (int64, error) {
	return r.Base.Count(ctx)
}
