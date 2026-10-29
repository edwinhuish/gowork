package repository

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"gorm.io/gorm"

	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/errs"
	"github.com/edwinhuish/gowork/internal/pkg/money"
)

// OpportunityFilter 是商机列表与汇总共用的查询条件。
//
// 两处共用同一个结构是刻意的：汇总的口径必须等于列表的口径，
// 否则会出现"列表 8 条、汇总 9 条"这种没人能解释的差异。
type OpportunityFilter struct {
	Keyword   string
	PartnerID *uint
	Stage     *model.OpportunityStage
	// OpenOnly 只看未结商机（排除赢单与输单），用于"当前在谈的还有多少"。
	OpenOnly bool
}

func (f OpportunityFilter) scopes() []Scope {
	scopes := make([]Scope, 0, 4)
	if f.Keyword != "" {
		scopes = append(scopes, likeAny(f.Keyword, "subject"))
	}
	if f.PartnerID != nil {
		pid := *f.PartnerID
		scopes = append(scopes, func(db *gorm.DB) *gorm.DB {
			return db.Where("partner_id = ?", pid)
		})
	}
	if f.Stage != nil {
		st := *f.Stage
		scopes = append(scopes, func(db *gorm.DB) *gorm.DB {
			return db.Where("stage = ?", st)
		})
	}
	if f.OpenOnly {
		scopes = append(scopes, func(db *gorm.DB) *gorm.DB {
			return db.Where("stage NOT IN ?", []model.OpportunityStage{
				model.OpportunityStageWon, model.OpportunityStageLost,
			})
		})
	}
	return scopes
}

// StageSummary 是按阶段聚合的结果。
//
// 只给本币金额：把 USD 的原币金额与 CNY 的原币金额相加没有意义，
// 而按币种分别汇总又要在界面上排两行同义数据。加权金额同理由本币算。
type StageSummary struct {
	Stage      model.OpportunityStage `gorm:"column:stage" json:"stage"`
	Count      int64                  `gorm:"column:cnt" json:"count"`
	AmountBase money.Decimal          `json:"amount_base"`
	Weighted   money.Decimal          `json:"weighted"`
}

type OpportunityRepository interface {
	Create(ctx context.Context, m *model.Opportunity) error
	Update(ctx context.Context, id uint, fields map[string]any) error
	Delete(ctx context.Context, id uint) error
	FindByID(ctx context.Context, id uint) (*model.Opportunity, error)
	List(ctx context.Context, f OpportunityFilter, p Pagination) ([]model.Opportunity, int64, error)
	// Summary 按阶段汇总条数、本币金额与加权金额。
	Summary(ctx context.Context, f OpportunityFilter) ([]StageSummary, error)
}

type opportunityRepo struct {
	*Base[model.Opportunity]
}

func NewOpportunityRepository(db *gorm.DB) OpportunityRepository {
	return &opportunityRepo{Base: NewBase[model.Opportunity](db)}
}

func (r *opportunityRepo) Create(ctx context.Context, m *model.Opportunity) error {
	return r.Base.Create(ctx, m)
}

func (r *opportunityRepo) Update(ctx context.Context, id uint, fields map[string]any) error {
	return r.Base.UpdateFields(ctx, id, fields)
}

func (r *opportunityRepo) Delete(ctx context.Context, id uint) error {
	return r.Base.DeleteByID(ctx, id)
}

func (r *opportunityRepo) FindByID(ctx context.Context, id uint) (*model.Opportunity, error) {
	var m model.Opportunity
	err := r.conn(ctx).Preload("Partner").First(&m, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errs.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("查询商机 id=%d: %w", id, err)
	}
	return &m, nil
}

// List 按预计成交日升序：最近要成交的排前面，这才是销售每天要看的顺序。
func (r *opportunityRepo) List(
	ctx context.Context, f OpportunityFilter, p Pagination,
) ([]model.Opportunity, int64, error) {
	scopes := append(f.scopes(), func(db *gorm.DB) *gorm.DB {
		return db.Preload("Partner")
	})
	return r.PaginateOrdered(ctx, p, "expected_close_date ASC, id DESC", scopes...)
}

// Summary 按阶段汇总。
//
// 汇总在 Go 里做，不下推到 SQL：`SUM(amount_base * probability / 100.0)` 在库里
// 走的是 REAL/DOUBLE，三库的浮点累加顺序还不一样，结果是"看板上权重加起来比总额
// 多几分钱"。拉开这些列在 CRM 的量级（万级以内）完全不是问题。
func (r *opportunityRepo) Summary(
	ctx context.Context, f OpportunityFilter,
) ([]StageSummary, error) {
	var rows []struct {
		Stage       model.OpportunityStage
		AmountBase  money.Decimal
		Probability int
	}
	err := r.conn(ctx).Model(&model.Opportunity{}).
		Scopes(f.scopes()...).
		Select("stage, amount_base, probability").
		Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("按阶段汇总商机: %w", err)
	}

	byStage := make(map[model.OpportunityStage]*StageSummary, len(rows))
	for _, row := range rows {
		agg, ok := byStage[row.Stage]
		if !ok {
			agg = &StageSummary{Stage: row.Stage}
			byStage[row.Stage] = agg
		}
		agg.Count++
		agg.AmountBase = agg.AmountBase.Add(row.AmountBase)
		// 单条加权按 2 位舍入再累加：与商机详情上显示的那个数保持一致，
		// 否则「各阶段加起来」与「逐条相加」会差几分钱。
		weighted := row.AmountBase.MulInt(int64(row.Probability)).
			Div(money.FromInt(100)).Round(2)
		agg.Weighted = agg.Weighted.Add(weighted)
	}

	out := make([]StageSummary, 0, len(byStage))
	for _, agg := range byStage {
		out = append(out, *agg)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Stage < out[j].Stage })
	return out, nil
}
