package repository

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/errs"
)

// ProductFilter 是产品列表的查询条件。
type ProductFilter struct {
	Keyword    string
	Category   string
	SupplierID *uint
	Status     *model.EnableStatus
}

func (f ProductFilter) scopes() []Scope {
	scopes := make([]Scope, 0, 4)

	if f.Keyword != "" {
		scopes = append(scopes, likeAny(f.Keyword, "sku_code", "name_cn", "name_en"))
	}
	if f.Category != "" {
		cat := f.Category
		scopes = append(scopes, func(db *gorm.DB) *gorm.DB {
			return db.Where("category = ?", cat)
		})
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

type ProductRepository interface {
	Create(ctx context.Context, m *model.Product) error
	Update(ctx context.Context, id uint, fields map[string]any) error
	Delete(ctx context.Context, id uint) error
	FindByID(ctx context.Context, id uint) (*model.Product, error)
	ExistsByCode(ctx context.Context, code string, excludeID uint) (bool, error)
	List(ctx context.Context, f ProductFilter, p Pagination) ([]model.Product, int64, error)
	Count(ctx context.Context) (int64, error)
	CountByEnableStatus(ctx context.Context, status model.EnableStatus) (int64, error)
}

type productRepo struct {
	*Base[model.Product]
}

func NewProductRepository(db *gorm.DB) ProductRepository {
	return &productRepo{Base: NewBase[model.Product](db)}
}

func (r *productRepo) Create(ctx context.Context, m *model.Product) error {
	return r.Base.Create(ctx, m)
}

func (r *productRepo) Update(ctx context.Context, id uint, fields map[string]any) error {
	return r.Base.UpdateFields(ctx, id, fields)
}

func (r *productRepo) Delete(ctx context.Context, id uint) error {
	return r.Base.DeleteByID(ctx, id)
}

// FindByID 覆盖基类实现，带出供应商信息供列表展示。
func (r *productRepo) FindByID(ctx context.Context, id uint) (*model.Product, error) {
	var m model.Product
	err := r.DB().WithContext(ctx).Preload("Supplier").First(&m, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errs.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("查询产品 id=%d: %w", id, err)
	}
	return &m, nil
}

// ExistsByCode 用于 SkuCode 唯一性前置校验，excludeID 供编辑时排除自身。
func (r *productRepo) ExistsByCode(ctx context.Context, code string, excludeID uint) (bool, error) {
	return r.Exists(ctx, func(db *gorm.DB) *gorm.DB {
		q := db.Where("sku_code = ?", code)
		if excludeID > 0 {
			q = q.Where("id <> ?", excludeID)
		}
		return q
	})
}

// List 列表带上供应商，便于前端直接展示名称。
// Preload 只在 Find 时生效，Count 阶段会被 gorm 忽略，不影响统计。
func (r *productRepo) List(ctx context.Context, f ProductFilter, p Pagination) ([]model.Product, int64, error) {
	scopes := append(f.scopes(), func(db *gorm.DB) *gorm.DB {
		return db.Preload("Supplier")
	})
	return r.Paginate(ctx, p, scopes...)
}

func (r *productRepo) Count(ctx context.Context) (int64, error) {
	return r.Base.Count(ctx)
}

func (r *productRepo) CountByEnableStatus(ctx context.Context, status model.EnableStatus) (int64, error) {
	return r.Base.Count(ctx, func(db *gorm.DB) *gorm.DB {
		return db.Where("status = ?", status)
	})
}
