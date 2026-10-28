package repository

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/errs"
)

// ActivityFilter 是跟进记录的查询条件。
type ActivityFilter struct {
	Keyword   string
	PartnerID *uint
	Type      *model.ActivityType
	// NextFollowBefore 非空时只取 NextFollowAt 早于该日期的记录，用于「已到期未跟进」。
	//
	// 传 YYYY-MM-DD 字符串而不是 time.Time：三库对 date 列的存储形态不同
	// （sqlite 存文本、pg/mysql 存日期），传时间戳时"当天"可能因为带不带时分秒
	// 而时而被算进超期、时而不算。按日期字符串比，三种库结论一致。
	NextFollowBefore string
}

func (f ActivityFilter) scopes() []Scope {
	scopes := make([]Scope, 0, 4)
	if f.Keyword != "" {
		scopes = append(scopes, likeAny(f.Keyword, "subject", "content"))
	}
	if f.PartnerID != nil {
		pid := *f.PartnerID
		scopes = append(scopes, func(db *gorm.DB) *gorm.DB {
			return db.Where("partner_id = ?", pid)
		})
	}
	if f.Type != nil {
		t := *f.Type
		scopes = append(scopes, func(db *gorm.DB) *gorm.DB {
			return db.Where("type = ?", t)
		})
	}
	if f.NextFollowBefore != "" {
		cutoff := f.NextFollowBefore
		// 到期日为空表示不需要跟进，不能被算成「超期」。
		scopes = append(scopes, func(db *gorm.DB) *gorm.DB {
			return db.Where("next_follow_at IS NOT NULL AND next_follow_at < ?", cutoff)
		})
	}
	return scopes
}

type ActivityRepository interface {
	Create(ctx context.Context, m *model.Activity) error
	Update(ctx context.Context, id uint, fields map[string]any) error
	Delete(ctx context.Context, id uint) error
	FindByID(ctx context.Context, id uint) (*model.Activity, error)
	List(ctx context.Context, f ActivityFilter, p Pagination) ([]model.Activity, int64, error)
}

type activityRepo struct {
	*Base[model.Activity]
}

func NewActivityRepository(db *gorm.DB) ActivityRepository {
	return &activityRepo{Base: NewBase[model.Activity](db)}
}

func (r *activityRepo) Create(ctx context.Context, m *model.Activity) error {
	return r.Base.Create(ctx, m)
}

func (r *activityRepo) Update(ctx context.Context, id uint, fields map[string]any) error {
	return r.Base.UpdateFields(ctx, id, fields)
}

func (r *activityRepo) Delete(ctx context.Context, id uint) error {
	return r.Base.DeleteByID(ctx, id)
}

func (r *activityRepo) FindByID(ctx context.Context, id uint) (*model.Activity, error) {
	var m model.Activity
	err := r.DB().WithContext(ctx).
		Preload("Partner").Preload("Contact").
		First(&m, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errs.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("查询跟进记录 id=%d: %w", id, err)
	}
	return &m, nil
}

// List 按发生时间倒序：时间线是本模块唯一的阅读方式。
//
// 顺序用 PaginateOrdered 传，不写在 scope 里——写在 scope 里会被 Paginate
// 内部的 id DESC 覆盖掉（见 PaginateOrdered 的注释）。
func (r *activityRepo) List(
	ctx context.Context, f ActivityFilter, p Pagination,
) ([]model.Activity, int64, error) {
	scopes := append(f.scopes(),
		func(db *gorm.DB) *gorm.DB {
			return db.Preload("Partner").Preload("Contact")
		},
	)
	return r.PaginateOrdered(ctx, p, "occurred_at DESC, id DESC", scopes...)
}
