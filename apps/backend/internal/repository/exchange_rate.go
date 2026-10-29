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
	// FindEffectiveFor 按指定汇率类型取价（T-60）：收款取买入价、付款取卖出价。
	// 该类型当天没有时回退中间价——存量数据全是中间价，不回退等于全线报缺汇率。
	FindEffectiveFor(
		ctx context.Context, currencyCode, baseCode string, on time.Time, rateType string,
	) (*model.ExchangeRate, error)
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
	return r.FindEffectiveFor(ctx, currencyCode, baseCode, on, model.RateTypeMiddle)
}

// FindEffectiveFor 按类型取价，缺该类型时回退中间价（T-60）。
//
// 回退而不是报错：存量汇率全是中间价，硬要求先补一份买入价才能收款，
// 等于把一次功能升级变成一次数据迁移。
func (r *exchangeRateRepo) FindEffectiveFor(
	ctx context.Context, currencyCode, baseCode string, on time.Time, rateType string,
) (*model.ExchangeRate, error) {
	if rateType == "" {
		rateType = model.RateTypeMiddle
	}
	tried := map[string]bool{}
	for _, rt := range []string{rateType, model.RateTypeMiddle} {
		if tried[rt] {
			continue
		}
		tried[rt] = true
		m, err := r.findByType(ctx, currencyCode, baseCode, on, rt)
		if err == nil {
			return m, nil
		}
		if !errors.Is(err, errs.ErrNotFound) {
			return nil, err
		}
	}
	return nil, errs.ErrNotFound
}

func (r *exchangeRateRepo) findByType(
	ctx context.Context, currencyCode, baseCode string, on time.Time, rateType string,
) (*model.ExchangeRate, error) {
	next := model.DateOnly(on).AddDate(0, 0, 1)

	var m model.ExchangeRate
	err := r.conn(ctx).
		Where("currency_code = ? AND base_code = ? AND effective_date < ?",
			currencyCode, baseCode, next).
		Scopes(requestRateType(rateType)).
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

// requestRateType 限定汇率类型（T-60）。
//
// 中间价同时接受空串：列有 default 'middle'，但建表早于这一列的老库
// 或直接用 SQL 插入的测试数据会得到 ”，按严格等值取会让它们全部消失。
func requestRateType(rateType string) func(*gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		// 中间价**不加列条件**：与改造前完全等价，存量与测试数据一定命中。
		// 只在显式要买入 / 卖出价时才过滤——那两个类型是新加的，
		// 加过滤最多是"没有这一侧"，而中间价加错过滤会让所有历史数据消失。
		if rateType == model.RateTypeMiddle || rateType == "" {
			return db
		}
		return db.Where("rate_type = ?", rateType)
	}
}
