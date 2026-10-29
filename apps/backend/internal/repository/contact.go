package repository

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/errs"
)

// ContactFilter 是联系人列表的查询条件。
type ContactFilter struct {
	Keyword   string
	PartnerID *uint
	Status    *model.EnableStatus
}

func (f ContactFilter) scopes() []Scope {
	scopes := make([]Scope, 0, 3)
	if f.Keyword != "" {
		scopes = append(scopes, likeAny(f.Keyword, "name", "phone", "email", "title"))
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

type ContactRepository interface {
	Create(ctx context.Context, m *model.Contact) error
	Update(ctx context.Context, id uint, fields map[string]any) error
	Delete(ctx context.Context, id uint) error
	FindByID(ctx context.Context, id uint) (*model.Contact, error)
	List(ctx context.Context, f ContactFilter, p Pagination) ([]model.Contact, int64, error)
	CountByPartnerID(ctx context.Context, partnerID uint) (int64, error)
	// SetPrimary 在事务里把某个联系人设为主联系人：先清掉同主体下的旧主联系人。
	SetPrimary(ctx context.Context, partnerID, id uint) error
}

type contactRepo struct {
	*Base[model.Contact]
}

func NewContactRepository(db *gorm.DB) ContactRepository {
	return &contactRepo{Base: NewBase[model.Contact](db)}
}

func (r *contactRepo) Create(ctx context.Context, m *model.Contact) error {
	return r.Base.Create(ctx, m)
}

func (r *contactRepo) Update(ctx context.Context, id uint, fields map[string]any) error {
	return r.Base.UpdateFields(ctx, id, fields)
}

func (r *contactRepo) Delete(ctx context.Context, id uint) error {
	return r.Base.DeleteByID(ctx, id)
}

// FindByID 带出主体，详情页与列表都要显示"这个人是哪家的"。
func (r *contactRepo) FindByID(ctx context.Context, id uint) (*model.Contact, error) {
	var m model.Contact
	err := r.conn(ctx).Preload("Partner").First(&m, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errs.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("查询联系人 id=%d: %w", id, err)
	}
	return &m, nil
}

func (r *contactRepo) List(ctx context.Context, f ContactFilter, p Pagination) ([]model.Contact, int64, error) {
	scopes := append(f.scopes(), func(db *gorm.DB) *gorm.DB {
		return db.Preload("Partner")
	})
	return r.Paginate(ctx, p, scopes...)
}

func (r *contactRepo) CountByPartnerID(ctx context.Context, partnerID uint) (int64, error) {
	return r.Base.Count(ctx, func(db *gorm.DB) *gorm.DB {
		return db.Where("partner_id = ?", partnerID)
	})
}

// SetPrimary 切换主联系人。
//
// 与币种的 SetBase 同一个套路：两条 UPDATE 必须同事务，否则中途失败会留下
// "一个主体没有主联系人"或"两个主联系人"的状态，而列表页看起来完全正常，
// 只有发邮件选收件人时才会发现选错了人。
func (r *contactRepo) SetPrimary(ctx context.Context, partnerID, id uint) error {
	return r.conn(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.Contact{}).
			Where("partner_id = ? AND is_primary = ? AND id <> ?", partnerID, true, id).
			Update("is_primary", false).Error; err != nil {
			return fmt.Errorf("清除原主联系人: %w", err)
		}
		res := tx.Model(&model.Contact{}).
			Where("id = ? AND partner_id = ?", id, partnerID).
			Update("is_primary", true)
		if res.Error != nil {
			return fmt.Errorf("设置主联系人 id=%d: %w", id, res.Error)
		}
		if res.RowsAffected == 0 {
			return errs.ErrNotFound
		}
		return nil
	})
}
