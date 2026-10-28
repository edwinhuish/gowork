package repository

import (
	"context"

	"gorm.io/gorm"

	"github.com/edwinhuish/gowork/internal/model"
)

type CustomerFilter struct {
	Keyword string
	Country string
	Status  *int8
}

func (f CustomerFilter) scopes() []Scope {
	scopes := make([]Scope, 0, 3)

	if f.Keyword != "" {
		kw := "%" + f.Keyword + "%"
		scopes = append(scopes, func(db *gorm.DB) *gorm.DB {
			return db.Where("code LIKE ? OR name LIKE ? OR contact LIKE ?", kw, kw, kw)
		})
	}
	if f.Country != "" {
		c := f.Country
		scopes = append(scopes, func(db *gorm.DB) *gorm.DB {
			return db.Where("country = ?", c)
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

type CustomerRepository interface {
	Create(ctx context.Context, m *model.Customer) error
	Update(ctx context.Context, id uint, fields map[string]any) error
	Delete(ctx context.Context, id uint) error
	FindByID(ctx context.Context, id uint) (*model.Customer, error)
	ExistsByCode(ctx context.Context, code string, excludeID uint) (bool, error)
	List(ctx context.Context, f CustomerFilter, p Pagination) ([]model.Customer, int64, error)
	Count(ctx context.Context) (int64, error)
}

type customerRepo struct {
	*Base[model.Customer]
}

func NewCustomerRepository(db *gorm.DB) CustomerRepository {
	return &customerRepo{Base: NewBase[model.Customer](db)}
}

func (r *customerRepo) Create(ctx context.Context, m *model.Customer) error {
	return r.Base.Create(ctx, m)
}

func (r *customerRepo) Update(ctx context.Context, id uint, fields map[string]any) error {
	return r.Base.UpdateFields(ctx, id, fields)
}

func (r *customerRepo) Delete(ctx context.Context, id uint) error {
	return r.Base.DeleteByID(ctx, id)
}

func (r *customerRepo) FindByID(ctx context.Context, id uint) (*model.Customer, error) {
	return r.Base.FindByID(ctx, id)
}

func (r *customerRepo) ExistsByCode(ctx context.Context, code string, excludeID uint) (bool, error) {
	return r.Exists(ctx, func(db *gorm.DB) *gorm.DB {
		q := db.Where("code = ?", code)
		if excludeID > 0 {
			q = q.Where("id <> ?", excludeID)
		}
		return q
	})
}

func (r *customerRepo) List(ctx context.Context, f CustomerFilter, p Pagination) ([]model.Customer, int64, error) {
	return r.Paginate(ctx, p, f.scopes()...)
}

func (r *customerRepo) Count(ctx context.Context) (int64, error) {
	return r.Base.Count(ctx)
}
