package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/errs"
	"github.com/edwinhuish/gowork/internal/pkg/money"
	"github.com/edwinhuish/gowork/internal/repository"
)

var errInvalidStage = errs.BadRequest(
	"stage 不合法：10 初步接洽 / 20 需求确认 / 30 已报价 / 40 谈判中 / 50 赢单 / 60 输单")

// OpportunityService 管商机。
//
// 它依赖币种服务来落「本币金额 + 汇率」：只存原币金额的话，
// 加权预测就没法把不同币种的商机加在一起，而那是看板唯一要回答的问题。
type OpportunityService interface {
	Create(ctx context.Context, in dto.OpportunityInput) (*model.Opportunity, error)
	Update(ctx context.Context, id uint, in dto.OpportunityInput) (*model.Opportunity, error)
	Delete(ctx context.Context, id uint) error
	Get(ctx context.Context, id uint) (*model.Opportunity, error)
	List(ctx context.Context, q dto.OpportunityQuery) (*dto.PageResult[model.Opportunity], error)
	Summary(ctx context.Context, q dto.OpportunityQuery) (*dto.OpportunitySummary, error)
}

type opportunityService struct {
	repo       repository.OpportunityRepository
	partners   repository.PartnerRepository
	currencies CurrencyService
	clock      func() time.Time
	log        *slog.Logger
}

type OpportunityOption func(*opportunityService)

func WithOpportunityLogger(l *slog.Logger) OpportunityOption {
	return func(s *opportunityService) { s.log = l }
}

// WithOpportunityClock 注入时钟：折算按当日汇率，测试要能指定"今天"。
func WithOpportunityClock(now func() time.Time) OpportunityOption {
	return func(s *opportunityService) { s.clock = now }
}

func NewOpportunityService(
	repo repository.OpportunityRepository,
	partners repository.PartnerRepository,
	currencies CurrencyService,
	opts ...OpportunityOption,
) OpportunityService {
	s := &opportunityService{
		repo:       repo,
		partners:   partners,
		currencies: currencies,
		clock:      time.Now,
		log:        slog.Default(),
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *opportunityService) Create(
	ctx context.Context, in dto.OpportunityInput,
) (*model.Opportunity, error) {
	if _, err := s.partners.FindByID(ctx, in.PartnerID); err != nil {
		if errors.Is(err, errs.ErrNotFound) {
			return nil, errs.BadRequest("往来单位不存在")
		}
		return nil, err
	}
	terms, err := s.buildTerms(ctx, in, nil)
	if err != nil {
		return nil, err
	}

	m := &model.Opportunity{
		PartnerID:         in.PartnerID,
		Subject:           terms.Subject,
		Stage:             terms.Stage,
		Amount:            terms.Amount,
		CurrencyCode:      terms.CurrencyCode,
		AmountBase:        terms.AmountBase,
		ExchangeRate:      terms.ExchangeRate,
		Probability:       terms.Probability,
		ExpectedCloseDate: terms.ExpectedCloseDate,
		Remark:            terms.Remark,
	}
	if err := s.repo.Create(ctx, m); err != nil {
		return nil, err
	}
	s.log.Info("新增商机", "id", m.ID, "partner_id", m.PartnerID,
		"stage", m.Stage.Label(), "amount_base", m.AmountBase.String())
	return m, nil
}

func (s *opportunityService) Update(
	ctx context.Context, id uint, in dto.OpportunityInput,
) (*model.Opportunity, error) {
	existing, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if in.PartnerID != 0 && in.PartnerID != existing.PartnerID {
		// 商机换主体等于把一段销售历史嫁接给另一家客户，跟进记录也会跟着失真。
		return nil, errs.BadRequest("商机不能转移到另一个往来单位，请新建")
	}
	terms, err := s.buildTerms(ctx, in, existing)
	if err != nil {
		return nil, err
	}
	if err := s.repo.Update(ctx, id, terms.fields()); err != nil {
		return nil, err
	}
	return s.repo.FindByID(ctx, id)
}

func (s *opportunityService) Delete(ctx context.Context, id uint) error {
	if _, err := s.repo.FindByID(ctx, id); err != nil {
		return err
	}
	return s.repo.Delete(ctx, id)
}

func (s *opportunityService) Get(ctx context.Context, id uint) (*model.Opportunity, error) {
	return s.repo.FindByID(ctx, id)
}

func (s *opportunityService) List(
	ctx context.Context, q dto.OpportunityQuery,
) (*dto.PageResult[model.Opportunity], error) {
	q.PageQuery.Normalize()
	filter, err := opportunityFilter(q)
	if err != nil {
		return nil, err
	}
	page := repository.Pagination{Page: q.Page, PageSize: q.PageSize}

	list, total, err := s.repo.List(ctx, filter, page)
	if err != nil {
		return nil, err
	}
	return dto.NewPageResult(list, total, q.Page, q.PageSize), nil
}

// Summary 按阶段汇总。与列表共用同一组筛选条件，口径不会分叉。
func (s *opportunityService) Summary(
	ctx context.Context, q dto.OpportunityQuery,
) (*dto.OpportunitySummary, error) {
	filter, err := opportunityFilter(q)
	if err != nil {
		return nil, err
	}
	rows, err := s.repo.Summary(ctx, filter)
	if err != nil {
		return nil, err
	}

	out := &dto.OpportunitySummary{Stages: make([]dto.OpportunityStageSummary, 0, len(rows))}
	for _, row := range rows {
		out.Stages = append(out.Stages, dto.OpportunityStageSummary{
			Stage:      int8(row.Stage),
			Count:      row.Count,
			AmountBase: row.AmountBase,
			Weighted:   row.Weighted,
		})
		out.TotalCount += row.Count
		out.TotalAmountBase = out.TotalAmountBase.Add(row.AmountBase)
		out.TotalWeighted = out.TotalWeighted.Add(row.Weighted)
	}
	return out, nil
}

// opportunityTerms 是校验并归一化之后的商机字段。
//
// 用结构体而不是 map[string]any 传给新建路径：map 取值要写类型断言，
// 断错了是运行时 panic，而结构体在编译期就能发现字段对不上。
type opportunityTerms struct {
	Subject           string
	Stage             model.OpportunityStage
	Amount            money.Decimal
	CurrencyCode      string
	AmountBase        money.Decimal
	ExchangeRate      money.Decimal
	Probability       int
	ExpectedCloseDate time.Time
	Remark            string
}

// fields 供编辑路径按列更新。不含 partner_id：主体不可改。
func (t opportunityTerms) fields() map[string]any {
	return map[string]any{
		"subject":             t.Subject,
		"stage":               t.Stage,
		"amount":              t.Amount,
		"currency_code":       t.CurrencyCode,
		"amount_base":         t.AmountBase,
		"exchange_rate":       t.ExchangeRate,
		"probability":         t.Probability,
		"expected_close_date": t.ExpectedCloseDate,
		"remark":              t.Remark,
	}
}

// buildTerms 校验并归一化。existing 为 nil 表示新建（没有"从哪个阶段来"可校验）。
func (s *opportunityService) buildTerms(
	ctx context.Context, in dto.OpportunityInput, existing *model.Opportunity,
) (opportunityTerms, error) {
	var out opportunityTerms

	stage := model.OpportunityStage(in.Stage)
	if !stage.IsValid() {
		return out, errInvalidStage
	}
	if existing != nil && stage != existing.Stage && !existing.Stage.CanTransitTo(stage) {
		return out, errs.ErrConflict.WithMsg(fmt.Sprintf(
			"商机阶段不能从「%s」变为「%s」", existing.Stage.Label(), stage.Label()))
	}
	if in.Amount.IsNegative() {
		return out, errs.BadRequest("预计金额不能为负")
	}

	closeDate, err := parseDateOnly(in.ExpectedCloseDate)
	if err != nil {
		return out, err
	}

	// 币种留空即本币。折算走 CurrencyService：缺当日汇率时给 400，
	// 而不是把 0 写进本币金额——那会让加权预测静默变少。
	baseCode, err := s.currencies.BaseCode(ctx)
	if err != nil {
		return out, err
	}
	code := strings.ToUpper(strings.TrimSpace(in.CurrencyCode))
	if code == "" {
		code = baseCode
	}
	amountBase, rate, err := s.currencies.ToBaseDecimal(ctx, in.Amount, code, s.clock())
	if err != nil {
		return out, err
	}

	out = opportunityTerms{
		Subject:           in.Subject,
		Stage:             stage,
		Amount:            in.Amount,
		CurrencyCode:      code,
		AmountBase:        amountBase,
		ExchangeRate:      rate,
		Probability:       normalizeProbability(stage, in.Probability),
		ExpectedCloseDate: closeDate,
		Remark:            in.Remark,
	}
	return out, nil
}

// normalizeProbability 决定最终的赢单概率。
//
// 赢单与输单的概率是定义出来的（100 / 0），不接受手填：
// 允许"赢单但 60%"会让加权金额永远对不上已成交金额。
func normalizeProbability(stage model.OpportunityStage, in *int) int {
	if stage == model.OpportunityStageWon {
		return 100
	}
	if stage == model.OpportunityStageLost {
		return 0
	}
	if in == nil {
		return stage.DefaultProbability()
	}
	p := *in
	if p < 0 {
		p = 0
	}
	if p > 100 {
		p = 100
	}
	return p
}

func opportunityFilter(q dto.OpportunityQuery) (repository.OpportunityFilter, error) {
	f := repository.OpportunityFilter{
		Keyword:   q.Keyword,
		PartnerID: q.PartnerID,
		OpenOnly:  q.OpenOnly != nil && *q.OpenOnly == 1,
	}
	if q.Stage != nil {
		stage := model.OpportunityStage(*q.Stage)
		if !stage.IsValid() {
			return f, errInvalidStage
		}
		f.Stage = &stage
	}
	return f, nil
}
