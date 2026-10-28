package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/errs"
)

// LeadFilter 是线索列表的查询条件。
type LeadFilter struct {
	Keyword string
	Status  *model.LeadStatus
	Source  *model.LeadSource
	Country string
	// OpenOnly 只看未转化且未废弃的线索，即"还在跟的"。
	OpenOnly bool
}

func (f LeadFilter) scopes() []Scope {
	scopes := make([]Scope, 0, 4)
	if f.Keyword != "" {
		scopes = append(scopes, likeAny(f.Keyword, "company", "contact_name", "phone", "email"))
	}
	if f.Status != nil {
		st := *f.Status
		scopes = append(scopes, func(db *gorm.DB) *gorm.DB {
			return db.Where("status = ?", st)
		})
	}
	if f.Source != nil {
		src := *f.Source
		scopes = append(scopes, func(db *gorm.DB) *gorm.DB {
			return db.Where("source = ?", src)
		})
	}
	if f.Country != "" {
		country := f.Country
		scopes = append(scopes, func(db *gorm.DB) *gorm.DB {
			return db.Where("country = ?", country)
		})
	}
	if f.OpenOnly {
		scopes = append(scopes, func(db *gorm.DB) *gorm.DB {
			return db.Where("status IN ?", []model.LeadStatus{
				model.LeadStatusNew, model.LeadStatusFollowing,
			})
		})
	}
	return scopes
}

type LeadRepository interface {
	Create(ctx context.Context, m *model.Lead) error
	Update(ctx context.Context, id uint, fields map[string]any) error
	Delete(ctx context.Context, id uint) error
	FindByID(ctx context.Context, id uint) (*model.Lead, error)
	List(ctx context.Context, f LeadFilter, p Pagination) ([]model.Lead, int64, error)
	// MarkConverted 把线索置为已转化并指向主体。
	//
	// 条件里带 `status <> 已转化`：两个请求同时点转化时，只有一个能改成功，
	// 另一个拿到 0 行，于是返回 409 而不是生成第二个主体。
	MarkConverted(ctx context.Context, id, partnerID uint, at time.Time) error
}

type leadRepo struct {
	*Base[model.Lead]
}

func NewLeadRepository(db *gorm.DB) LeadRepository {
	return &leadRepo{Base: NewBase[model.Lead](db)}
}

func (r *leadRepo) Create(ctx context.Context, m *model.Lead) error {
	return r.Base.Create(ctx, m)
}

func (r *leadRepo) Update(ctx context.Context, id uint, fields map[string]any) error {
	return r.Base.UpdateFields(ctx, id, fields)
}

func (r *leadRepo) Delete(ctx context.Context, id uint) error {
	return r.Base.DeleteByID(ctx, id)
}

func (r *leadRepo) FindByID(ctx context.Context, id uint) (*model.Lead, error) {
	var m model.Lead
	err := r.DB().WithContext(ctx).Preload("Partner").First(&m, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errs.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("查询线索 id=%d: %w", id, err)
	}
	return &m, nil
}

// List 按创建时间倒序：新询盘在最上面，这是线索列表唯一合理的顺序。
func (r *leadRepo) List(ctx context.Context, f LeadFilter, p Pagination) ([]model.Lead, int64, error) {
	scopes := append(f.scopes(), func(db *gorm.DB) *gorm.DB {
		return db.Preload("Partner")
	})
	return r.PaginateOrdered(ctx, p, "id DESC", scopes...)
}

func (r *leadRepo) MarkConverted(ctx context.Context, id, partnerID uint, at time.Time) error {
	res := r.DB().WithContext(ctx).Model(&model.Lead{}).
		Where("id = ? AND status <> ?", id, model.LeadStatusConverted).
		Updates(map[string]any{
			"status":       model.LeadStatusConverted,
			"partner_id":   partnerID,
			"converted_at": at,
		})
	if res.Error != nil {
		return fmt.Errorf("标记线索已转化 id=%d: %w", id, res.Error)
	}
	if res.RowsAffected == 0 {
		return errs.ErrConflict.WithMsg("该线索已转化，不能重复转化")
	}
	return nil
}
