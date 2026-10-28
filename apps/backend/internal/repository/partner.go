package repository

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/errs"
)

// PartnerFilter 是商业伙伴列表的查询条件。
//
// Role 是"包含"语义而不是"等于"：筛供应商时，既做客户又做供应商的主体
// 必须一起出来，否则它在采购侧的选择列表里会消失。
type PartnerFilter struct {
	Keyword    string
	Role       *model.PartnerRole
	EntityType *model.PartnerEntityType
	Country    string
	Status     *model.EnableStatus
}

func (f PartnerFilter) scopes() []Scope {
	scopes := make([]Scope, 0, 5)

	if f.Keyword != "" {
		scopes = append(scopes, likeAny(f.Keyword, "code", "name", "contact"))
	}
	if f.Role != nil {
		// 展开成 IN 集合而不是写 `role & ? <> 0`：位运算在 PostgreSQL 和 MySQL
		// 上都用不上 role 上的 btree 索引，而按角色过滤是列表页的默认条件。
		values := model.RoleValuesContaining(*f.Role)
		scopes = append(scopes, func(db *gorm.DB) *gorm.DB {
			return db.Where("role IN ?", values)
		})
	}
	if f.EntityType != nil {
		t := *f.EntityType
		scopes = append(scopes, func(db *gorm.DB) *gorm.DB {
			return db.Where("entity_type = ?", t)
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

type PartnerRepository interface {
	Create(ctx context.Context, m *model.Partner) error
	Update(ctx context.Context, id uint, fields map[string]any) error
	Delete(ctx context.Context, id uint) error
	FindByID(ctx context.Context, id uint) (*model.Partner, error)
	ExistsByCode(ctx context.Context, code string, excludeID uint) (bool, error)
	// FindByNameAndCountry 按"公司名 + 国家"查同名主体，供线索转化时提示重复。
	// 找不到返回 errs.ErrNotFound —— 外贸同名公司确实存在，所以调用方只提示、不阻断。
	FindByNameAndCountry(ctx context.Context, name, country string) (*model.Partner, error)
	List(ctx context.Context, f PartnerFilter, p Pagination) ([]model.Partner, int64, error)
	Count(ctx context.Context) (int64, error)
	CountByRole(ctx context.Context, role model.PartnerRole) (int64, error)
}

type partnerRepo struct {
	*Base[model.Partner]
}

func NewPartnerRepository(db *gorm.DB) PartnerRepository {
	return &partnerRepo{Base: NewBase[model.Partner](db)}
}

func (r *partnerRepo) Create(ctx context.Context, m *model.Partner) error {
	return r.Base.Create(ctx, m)
}

func (r *partnerRepo) Update(ctx context.Context, id uint, fields map[string]any) error {
	return r.Base.UpdateFields(ctx, id, fields)
}

func (r *partnerRepo) Delete(ctx context.Context, id uint) error {
	return r.Base.DeleteByID(ctx, id)
}

func (r *partnerRepo) FindByID(ctx context.Context, id uint) (*model.Partner, error) {
	return r.Base.FindByID(ctx, id)
}

// ExistsByCode 做编码唯一性前置校验，excludeID 供编辑时排除自身。
//
// 合并后编码在客户与供应商之间也唯一：同一个主体只有一个编码，
// 这正是合并要解决的问题（原先 CUS001 与 SUP001 可能指向同一家公司）。
func (r *partnerRepo) ExistsByCode(ctx context.Context, code string, excludeID uint) (bool, error) {
	return r.Exists(ctx, func(db *gorm.DB) *gorm.DB {
		q := db.Where("code = ?", code)
		if excludeID > 0 {
			q = q.Where("id <> ?", excludeID)
		}
		return q
	})
}

// FindByNameAndCountry 精确匹配名称，国家为空时只按名称匹配。
func (r *partnerRepo) FindByNameAndCountry(
	ctx context.Context, name, country string,
) (*model.Partner, error) {
	var m model.Partner
	q := r.DB().WithContext(ctx).Where("name = ?", name)
	if country != "" {
		q = q.Where("country = ?", country)
	}
	err := q.First(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errs.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("按名称查主体 name=%s: %w", name, err)
	}
	return &m, nil
}

func (r *partnerRepo) List(ctx context.Context, f PartnerFilter, p Pagination) ([]model.Partner, int64, error) {
	return r.Paginate(ctx, p, f.scopes()...)
}

func (r *partnerRepo) Count(ctx context.Context) (int64, error) {
	return r.Base.Count(ctx)
}

// CountByRole 统计承担了某个角色的主体数（含同时承担另一个角色的）。
//
// 用 RoleValuesContaining 而不是 Count(role = ?)：后者会漏掉 role=3 的"客供两用"
// 主体，导致概览里的供应商数比供应商列表实际少了。
func (r *partnerRepo) CountByRole(ctx context.Context, role model.PartnerRole) (int64, error) {
	values := model.RoleValuesContaining(role)
	return r.Base.Count(ctx, func(db *gorm.DB) *gorm.DB {
		return db.Where("role IN ?", values)
	})
}
