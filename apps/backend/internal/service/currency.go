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

// CurrencyService 管币种，并提供全站唯一的跨币种折算入口。
//
// 折算之所以必须收在这一个方法里：报价单算成本、订单算毛利、收款算汇兑损益，
// 三处若各自"乘以汇率"，对"乘以哪一天的汇率、结果保留几位"就会各有一套答案，
// 同一笔生意在三张单上差几分钱，对账时无法解释差额来自哪里。
type CurrencyService interface {
	Create(ctx context.Context, in dto.CurrencyInput) (*model.Currency, error)
	Update(ctx context.Context, id uint, in dto.CurrencyInput) (*model.Currency, error)
	Delete(ctx context.Context, id uint) error
	Get(ctx context.Context, id uint) (*model.Currency, error)
	List(ctx context.Context, q dto.CurrencyQuery) (*dto.PageResult[model.Currency], error)
	// Convert 把 amount 从 from 折算成 to，汇率取 on 当日生效的那条。
	// 返回的 rate 要一并写进单据：事后汇率表怎么变，都不该影响已成交的单子。
	Convert(ctx context.Context, amount float64, from, to string, on time.Time) (converted, rate float64, err error)
}

type currencyService struct {
	repo  repository.CurrencyRepository
	rates repository.ExchangeRateRepository
	log   *slog.Logger
}

type CurrencyOption func(*currencyService)

func WithCurrencyLogger(l *slog.Logger) CurrencyOption {
	return func(s *currencyService) { s.log = l }
}

func NewCurrencyService(
	repo repository.CurrencyRepository,
	rates repository.ExchangeRateRepository,
	opts ...CurrencyOption,
) CurrencyService {
	s := &currencyService{repo: repo, rates: rates, log: slog.Default()}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *currencyService) Create(ctx context.Context, in dto.CurrencyInput) (*model.Currency, error) {
	code, err := normCurrencyCode(in.Code)
	if err != nil {
		return nil, err
	}
	dp, err := normDecimalPlaces(in.DecimalPlaces, model.DefaultDecimalPlaces)
	if err != nil {
		return nil, err
	}
	status, err := normEnableStatus(in.Status, model.EnableStatusOn)
	if err != nil {
		return nil, err
	}

	dup, err := s.repo.ExistsByCode(ctx, code, 0)
	if err != nil {
		return nil, err
	}
	if dup {
		return nil, errs.ErrConflict.WithMsg("币种代码已存在：" + code)
	}

	m := &model.Currency{
		Code:          code,
		Name:          in.Name,
		Symbol:        in.Symbol,
		DecimalPlaces: dp,
		Status:        status,
		Remark:        in.Remark,
	}
	if err := s.repo.Create(ctx, m); err != nil {
		return nil, err
	}

	// 先落库再设本币：SetBase 是两条 UPDATE（清旧、置新），
	// 需要一个已存在的 id 才能在同一事务里完成。
	if in.IsBase != nil && *in.IsBase {
		if err := s.repo.SetBase(ctx, m.ID); err != nil {
			return nil, err
		}
		m.IsBase = true
	}

	s.log.Info("新增币种", "id", m.ID, "code", m.Code, "is_base", m.IsBase)
	return m, nil
}

func (s *currencyService) Update(ctx context.Context, id uint, in dto.CurrencyInput) (*model.Currency, error) {
	cur, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	code, err := normCurrencyCode(in.Code)
	if err != nil {
		return nil, err
	}
	if code != cur.Code {
		return nil, errs.BadRequest("币种代码是业务身份，不可修改；请停用后新增一个币种")
	}
	dp, err := normDecimalPlaces(in.DecimalPlaces, cur.DecimalPlaces)
	if err != nil {
		return nil, err
	}
	status, err := normEnableStatus(in.Status, model.EnableStatusOn)
	if err != nil {
		return nil, err
	}

	if in.IsBase != nil {
		switch {
		case *in.IsBase && !cur.IsBase:
			if err := s.repo.SetBase(ctx, id); err != nil {
				return nil, err
			}
		case !*in.IsBase && cur.IsBase:
			// 允许"换一个本币"（上面那支），但不允许"把本币摘掉"：
			// 没有本币时所有折算都会失败，而报错信息指向的是缺汇率。
			return nil, errs.ErrConflict.WithMsg("不能取消本币标记，请改把另一个币种设为本币")
		}
	}

	fields := map[string]any{
		"name":           in.Name,
		"symbol":         in.Symbol,
		"decimal_places": dp,
		"status":         status,
		"remark":         in.Remark,
	}
	if err := s.repo.Update(ctx, id, fields); err != nil {
		return nil, err
	}
	return s.repo.FindByID(ctx, id)
}

func (s *currencyService) Delete(ctx context.Context, id uint) error {
	cur, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return err
	}
	if cur.IsBase {
		return errs.ErrConflict.WithMsg("本币不能删除，请先把另一个币种设为本币")
	}
	// 有汇率记录的币种只能停用。软删除币种而把汇率留着，
	// 之后按代码折算会命中"币种不存在"，而汇率页里那些记录看起来完全正常。
	n, err := s.rates.CountByCurrencyCode(ctx, cur.Code)
	if err != nil {
		return err
	}
	if n > 0 {
		return errs.ErrConflict.WithMsg(fmt.Sprintf("该币种已有 %d 条汇率记录，请停用而不是删除", n))
	}
	return s.repo.Delete(ctx, id)
}

func (s *currencyService) Get(ctx context.Context, id uint) (*model.Currency, error) {
	return s.repo.FindByID(ctx, id)
}

func (s *currencyService) List(ctx context.Context, q dto.CurrencyQuery) (*dto.PageResult[model.Currency], error) {
	q.PageQuery.Normalize()

	filter := repository.CurrencyFilter{
		Keyword: q.Keyword,
		Status:  enableStatusPtr(q.Status),
	}
	page := repository.Pagination{Page: q.Page, PageSize: q.PageSize}

	list, total, err := s.repo.List(ctx, filter, page)
	if err != nil {
		return nil, err
	}
	return dto.NewPageResult(list, total, q.Page, q.PageSize), nil
}

func (s *currencyService) Convert(
	ctx context.Context, amount float64, from, to string, on time.Time,
) (float64, float64, error) {
	fromCode, err := normCurrencyCode(from)
	if err != nil {
		return 0, 0, err
	}
	toCode, err := normCurrencyCode(to)
	if err != nil {
		return 0, 0, err
	}
	if fromCode == toCode {
		return amount, 1, nil
	}

	base, err := s.repo.FindBase(ctx)
	if err != nil {
		if errors.Is(err, errs.ErrNotFound) {
			return 0, 0, errs.BadRequest("尚未设置本币，无法折算汇率")
		}
		return 0, 0, err
	}
	target, err := s.repo.FindByCode(ctx, toCode)
	if err != nil {
		if errors.Is(err, errs.ErrNotFound) {
			return 0, 0, errs.BadRequest("币种不存在：" + toCode)
		}
		return 0, 0, err
	}

	fromRate, err := s.rateOn(ctx, fromCode, base.Code, on)
	if err != nil {
		return 0, 0, err
	}
	toRate, err := s.rateOn(ctx, toCode, base.Code, on)
	if err != nil {
		return 0, 0, err
	}

	// 两个币种都相对本币有价，交叉汇率由两者相除得到。
	// 直接除以 toRate 而不是"先折本币再折目标币"，少一次舍入。
	rate := fromRate / toRate
	return money.Round(amount*rate, target.DecimalPlaces), rate, nil
}

// rateOn 取某币种相对本币在 on 当日的折算率，本币自身恒为 1。
func (s *currencyService) rateOn(
	ctx context.Context, code, baseCode string, on time.Time,
) (float64, error) {
	if code == baseCode {
		return 1, nil
	}
	r, err := s.rates.FindEffective(ctx, code, baseCode, on)
	if err != nil {
		if errors.Is(err, errs.ErrNotFound) {
			return 0, errs.BadRequest(fmt.Sprintf("缺少 %s 在 %s 的汇率",
				code, model.DateOnly(on).Format("2006-01-02")))
		}
		return 0, err
	}
	return r.Rate, nil
}

var errInvalidCurrencyCode = errs.BadRequest("币种代码必须是 3 位字母，例如 USD")

// normCurrencyCode 统一大写去空格，并挡掉 "US"、"USDT1" 这类无法与单据对上的编码。
func normCurrencyCode(raw string) (string, error) {
	code := strings.ToUpper(strings.TrimSpace(raw))
	if len(code) != 3 {
		return "", errInvalidCurrencyCode
	}
	for _, r := range code {
		if r < 'A' || r > 'Z' {
			return "", errInvalidCurrencyCode
		}
	}
	return code, nil
}

// normDecimalPlaces 校验小数位，nil 表示未提交，退回默认值。
func normDecimalPlaces(p *int, def int) (int, error) {
	if p == nil {
		return def, nil
	}
	if *p < money.MinDecimalPlaces || *p > money.MaxDecimalPlaces {
		return 0, errs.BadRequest(fmt.Sprintf("小数位只能在 %d 到 %d 之间",
			money.MinDecimalPlaces, money.MaxDecimalPlaces))
	}
	return *p, nil
}
