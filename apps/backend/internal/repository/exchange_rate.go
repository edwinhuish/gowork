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

// ExchangeRateFilter 是汇率列表的查询条件。币种是必填维度，
// 不分币种地列全部汇率没有业务含义（一天几十条，看不出什么）。
type ExchangeRateFilter struct {
	CurrencyCode string
	BaseCode     string
}

func (f ExchangeRateFilter) scopes() []Scope {
	scopes := make([]Scope, 0, 2)
	if f.CurrencyCode != "" {
		code := f.CurrencyCode
		scopes = append(scopes, func(db *gorm.DB) *gorm.DB {
			return db.Where("currency_code = ?", code)
		})
	}
	if f.BaseCode != "" {
		code := f.BaseCode
		scopes = append(scopes, func(db *gorm.DB) *gorm.DB {
			return db.Where("base_code = ?", code)
		})
	}
	return scopes
}

type ExchangeRateRepository interface {
	Create(ctx context.Context, m *model.ExchangeRate) error
	Delete(ctx context.Context, id uint) error
	FindByID(ctx context.Context, id uint) (*model.ExchangeRate, error)
	ExistsByPair(ctx context.Context, currencyCode, baseCode string, effectiveDate time.Time, excludeID uint) (bool, error)
	List(ctx context.Context, f ExchangeRateFilter, p Pagination) ([]model.ExchangeRate, int64, error)
	// CountByCurrencyCode 统计某外币的汇率条数，供"能不能删这个币种"判断。
	CountByCurrencyCode(ctx context.Context, currencyCode string) (int64, error)
	// FindEffective 取 on 当日生效的汇率：生效日不晚于 on 的最近一条。
	// 找不到返回 errs.ErrNotFound。
	FindEffective(ctx context.Context, currencyCode, baseCode string, on time.Time) (*model.ExchangeRate, error)
}

type exchangeRateRepo struct {
	*Base[model.ExchangeRate]
}

func NewExchangeRateRepository(db *gorm.DB) ExchangeRateRepository {
	return &exchangeRateRepo{Base: NewBase[model.ExchangeRate](db)}
}

func (r *exchangeRateRepo) Create(ctx context.Context, m *model.ExchangeRate) error {
	return r.Base.Create(ctx, m)
}

func (r *exchangeRateRepo) Delete(ctx context.Context, id uint) error {
	return r.Base.DeleteByID(ctx, id)
}

func (r *exchangeRateRepo) FindByID(ctx context.Context, id uint) (*model.ExchangeRate, error) {
	return r.Base.FindByID(ctx, id)
}

func (r *exchangeRateRepo) ExistsByPair(
	ctx context.Context, currencyCode, baseCode string, effectiveDate time.Time, excludeID uint,
) (bool, error) {
	return r.Exists(ctx, func(db *gorm.DB) *gorm.DB {
		q := db.Where("currency_code = ? AND base_code = ? AND effective_date = ?",
			currencyCode, baseCode, effectiveDate)
		if excludeID > 0 {
			q = q.Where("id <> ?", excludeID)
		}
		return q
	})
}

// List 按生效日倒序：看汇率时关心的永远是"最新那条"。
//
// 排序走 PaginateOrdered 而不是塞进 scope：Paginate 内部还会 Order("id DESC")，
// 后写的会覆盖前面的，写在 scope 里的排序会被静默吃掉。
func (r *exchangeRateRepo) List(
	ctx context.Context, f ExchangeRateFilter, p Pagination,
) ([]model.ExchangeRate, int64, error) {
	return r.PaginateOrdered(ctx, p, "effective_date DESC, id DESC", f.scopes()...)
}

func (r *exchangeRateRepo) CountByCurrencyCode(ctx context.Context, currencyCode string) (int64, error) {
	return r.Base.Count(ctx, func(db *gorm.DB) *gorm.DB {
		return db.Where("currency_code = ?", currencyCode)
	})
}

// FindEffective 用半开区间 [起, on+1天) 而不是 effective_date <= on。
//
// 列类型是 date，但三库对 date 的比较口径不同：sqlite 把它存成文本、pg 存成 date，
// 直接拿"当天 00:00"去比，sqlite 上会因文本后缀把当天的记录排除掉。
// 比"次日零点"小则三种库都成立，且不需要给参数补时分秒。
func (r *exchangeRateRepo) FindEffective(
	ctx context.Context, currencyCode, baseCode string, on time.Time,
) (*model.ExchangeRate, error) {
	next := model.DateOnly(on).AddDate(0, 0, 1)

	var m model.ExchangeRate
	err := r.conn(ctx).
		Where("currency_code = ? AND base_code = ? AND effective_date < ?", currencyCode, baseCode, next).
		Order("effective_date DESC").
		First(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errs.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("查询 %s/%s 于 %s 的汇率: %w",
			currencyCode, baseCode, model.DateOnly(on).Format("2006-01-02"), err)
	}
	return &m, nil
}
