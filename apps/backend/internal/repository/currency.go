package repository

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/errs"
)

// CurrencyFilter 是币种列表的查询条件。
type CurrencyFilter struct {
	Keyword string
	Status  *model.EnableStatus
}

func (f CurrencyFilter) scopes() []Scope {
	scopes := make([]Scope, 0, 2)
	if f.Keyword != "" {
		scopes = append(scopes, likeAny(f.Keyword, "code", "name", "symbol"))
	}
	if f.Status != nil {
		st := *f.Status
		scopes = append(scopes, func(db *gorm.DB) *gorm.DB {
			return db.Where("status = ?", st)
		})
	}
	return scopes
}

type CurrencyRepository interface {
	Create(ctx context.Context, m *model.Currency) error
	Update(ctx context.Context, id uint, fields map[string]any) error
	Delete(ctx context.Context, id uint) error
	FindByID(ctx context.Context, id uint) (*model.Currency, error)
	// FindByCode 按三位代码查，用于单据折算与汇率校验。
	FindByCode(ctx context.Context, code string) (*model.Currency, error)
	// FindBase 返回本币。没有设置时返回 errs.ErrNotFound。
	FindBase(ctx context.Context) (*model.Currency, error)
	ExistsByCode(ctx context.Context, code string, excludeID uint) (bool, error)
	List(ctx context.Context, f CurrencyFilter, p Pagination) ([]model.Currency, int64, error)
	Count(ctx context.Context) (int64, error)
	// SetBase 在一个事务里把本币切换到 id：先清掉旧的，再置新的。
	SetBase(ctx context.Context, id uint) error
}

type currencyRepo struct {
	*Base[model.Currency]
}

func NewCurrencyRepository(db *gorm.DB) CurrencyRepository {
	return &currencyRepo{Base: NewBase[model.Currency](db)}
}

func (r *currencyRepo) Create(ctx context.Context, m *model.Currency) error {
	return r.Base.Create(ctx, m)
}

func (r *currencyRepo) Update(ctx context.Context, id uint, fields map[string]any) error {
	return r.Base.UpdateFields(ctx, id, fields)
}

func (r *currencyRepo) Delete(ctx context.Context, id uint) error {
	return r.Base.DeleteByID(ctx, id)
}

func (r *currencyRepo) FindByID(ctx context.Context, id uint) (*model.Currency, error) {
	return r.Base.FindByID(ctx, id)
}

func (r *currencyRepo) FindByCode(ctx context.Context, code string) (*model.Currency, error) {
	var m model.Currency
	err := r.conn(ctx).Where("code = ?", code).First(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errs.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("查询币种 code=%s: %w", code, err)
	}
	return &m, nil
}

func (r *currencyRepo) FindBase(ctx context.Context) (*model.Currency, error) {
	var m model.Currency
	err := r.conn(ctx).Where("is_base = ?", true).First(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errs.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("查询本币: %w", err)
	}
	return &m, nil
}

func (r *currencyRepo) ExistsByCode(ctx context.Context, code string, excludeID uint) (bool, error) {
	return r.Exists(ctx, func(db *gorm.DB) *gorm.DB {
		q := db.Where("code = ?", code)
		if excludeID > 0 {
			q = q.Where("id <> ?", excludeID)
		}
		return q
	})
}

func (r *currencyRepo) List(ctx context.Context, f CurrencyFilter, p Pagination) ([]model.Currency, int64, error) {
	return r.Paginate(ctx, p, f.scopes()...)
}

func (r *currencyRepo) Count(ctx context.Context) (int64, error) {
	return r.Base.Count(ctx)
}

// SetBase 切换本币。
//
// 两条 UPDATE 必须在同一个事务里：中间态曾经由应用层"先清后置"两次调用产生，
// 一旦第二步失败，库里会没有任何本币，之后所有折算都报错，而错误信息指向的是
// 汇率缺失——排查会走很远。
func (r *currencyRepo) SetBase(ctx context.Context, id uint) error {
	return r.conn(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.Currency{}).
			Where("is_base = ? AND id <> ?", true, id).
			Update("is_base", false).Error; err != nil {
			return fmt.Errorf("清除原本币: %w", err)
		}
		res := tx.Model(&model.Currency{}).Where("id = ?", id).Update("is_base", true)
		if res.Error != nil {
			return fmt.Errorf("设置本币 id=%d: %w", id, res.Error)
		}
		if res.RowsAffected == 0 {
			return errs.ErrNotFound
		}
		return nil
	})
}
