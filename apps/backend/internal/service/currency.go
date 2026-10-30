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
	// ToBaseDecimalFor 按指定汇率类型折本币（T-60）：收款取买入价、付款取卖出价。
	ToBaseDecimalFor(
		ctx context.Context, amount money.Decimal, currencyCode string, on time.Time, rateType string,
	) (money.Decimal, money.Decimal, error)
	Create(ctx context.Context, in dto.CurrencyInput) (*model.Currency, error)
	Update(ctx context.Context, id uint, in dto.CurrencyInput) (*model.Currency, error)
	Delete(ctx context.Context, id uint) error
	Restore(ctx context.Context, id uint) (*model.Currency, error)
	Get(ctx context.Context, id uint) (*model.Currency, error)
	List(ctx context.Context, q dto.CurrencyQuery) (*dto.PageResult[model.Currency], error)
	// ConvertDecimal 把 amount 从 from 折算成 to，汇率取 on 当日生效的那条。
	// 返回的 rate 要一并写进单据：事后汇率表怎么变，都不该影响已成交的单子。
	//
	// 只有定点这一个版本（T-37 第 6 片）：汇率与金额都是 Decimal，
	// 留一个 float 版的重载等于给"中途转回 float64"留了后门。
	ConvertDecimal(
		ctx context.Context, amount money.Decimal, from, to string, on time.Time,
	) (converted, rate money.Decimal, err error)
	// ToBaseDecimal 折算到本币，供"只知道原币、不关心本币是哪个"的调用方使用。
	//
	// 调用方只需要汇率时传 money.FromInt(1)：这一路同时完成了
	// "币种是否存在"与"当日汇率是否已录"两项校验。
	ToBaseDecimal(
		ctx context.Context, amount money.Decimal, currencyCode string, on time.Time,
	) (base, rate money.Decimal, err error)
	// BaseCode 返回本币代码。币种留空表示"用本币"时，需要它来落字段。
	BaseCode(ctx context.Context) (string, error)
	// Decimals 返回该币种的金额小数位，报价单与订单算行金额时按它舍入。
	// 不把小数位写死在调用处：日元 0 位、多数币种 2 位，写死 2 会让日元单据多出两位小数。
	Decimals(ctx context.Context, code string) (int, error)
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
		return nil, errs.BadRequest("币种代码是业务身份，不可修改；请归档后新增一个币种")
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
	if in.UpdatedAt != nil {
		fields[repository.FieldUpdatedAt] = *in.UpdatedAt
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
	// 有汇率记录的币种只能归档。软删除币种而把汇率留着，
	// 之后按代码折算会命中"币种不存在"，而汇率页里那些记录看起来完全正常。
	n, err := s.rates.CountByCurrencyCode(ctx, cur.Code)
	if err != nil {
		return err
	}
	if n > 0 {
		return errs.ErrConflict.WithMsg(fmt.Sprintf("该币种已有 %d 条汇率记录，请归档而不是删除", n))
	}
	return s.repo.Delete(ctx, id)
}

// Restore 把软删的币种放回来。
//
// 先查代码是否被占：删掉之后可以重新录一个同代码的币种，
// 那时恢复会让两张单据指向不同的 id，而按代码折算又分不出来。
func (s *currencyService) Restore(ctx context.Context, id uint) (*model.Currency, error) {
	cur, err := s.repo.FindByIDIncludingDeleted(ctx, id)
	if err != nil {
		return nil, err
	}
	if cur.DeletedAt == 0 {
		return nil, errs.BadRequest("该币种未被删除，无需恢复")
	}
	dup, err := s.repo.ExistsByCode(ctx, cur.Code, id)
	if err != nil {
		return nil, err
	}
	if dup {
		return nil, errs.ErrConflict.WithMsg("币种代码已被占用：" + cur.Code)
	}
	if err := s.repo.Restore(ctx, id); err != nil {
		return nil, err
	}
	s.log.Info("恢复币种", "id", id, "code", cur.Code)
	return s.repo.FindByID(ctx, id)
}

func (s *currencyService) Get(ctx context.Context, id uint) (*model.Currency, error) {
	return s.repo.FindByID(ctx, id)
}

func (s *currencyService) List(ctx context.Context, q dto.CurrencyQuery) (*dto.PageResult[model.Currency], error) {
	q.PageQuery.Normalize()

	filter := repository.CurrencyFilter{
		Keyword:        q.Keyword,
		Status:         enableStatusPtr(q.Status),
		IncludeDeleted: q.IncludeDeleted,
	}
	page := repository.Pagination{Page: q.Page, PageSize: q.PageSize}

	list, total, err := s.repo.List(ctx, filter, page)
	if err != nil {
		return nil, err
	}
	return dto.NewPageResult(list, total, q.Page, q.PageSize), nil
}

// ConvertDecimal 是全站唯一的折算口径：同一套汇率查找、同一个舍入位数。
func (s *currencyService) ConvertDecimal(
	ctx context.Context, amount money.Decimal, from, to string, on time.Time,
) (money.Decimal, money.Decimal, error) {
	fromCode, err := normCurrencyCode(from)
	if err != nil {
		return money.Zero(), money.Zero(), err
	}
	toCode, err := normCurrencyCode(to)
	if err != nil {
		return money.Zero(), money.Zero(), err
	}
	if fromCode == toCode {
		return amount, money.FromInt(1), nil
	}

	base, err := s.repo.FindBase(ctx)
	if err != nil {
		if errors.Is(err, errs.ErrNotFound) {
			return money.Zero(), money.Zero(), errs.BadRequest("尚未设置本币，无法折算汇率")
		}
		return money.Zero(), money.Zero(), err
	}
	for _, code := range []string{fromCode, toCode} {
		if code == base.Code {
			continue
		}
		if _, err := s.repo.FindByCode(ctx, code); err != nil {
			if errors.Is(err, errs.ErrNotFound) {
				return money.Zero(), money.Zero(), errs.BadRequest("币种不存在：" + code)
			}
			return money.Zero(), money.Zero(), err
		}
	}

	target, err := s.repo.FindByCode(ctx, toCode)
	if err != nil {
		return money.Zero(), money.Zero(), err
	}

	fromRate, err := s.rateOn(ctx, fromCode, base.Code, on)
	if err != nil {
		return money.Zero(), money.Zero(), err
	}
	toRate, err := s.rateOn(ctx, toCode, base.Code, on)
	if err != nil {
		return money.Zero(), money.Zero(), err
	}

	// 两个币种都相对本币有价，交叉汇率由两者相除得到。
	// 直接除以 toRate 而不是"先折本币再折目标币"，少一次舍入。
	rate := fromRate.Div(toRate)
	converted := amount.Mul(rate).Round(target.DecimalPlaces)
	return converted, rate, nil
}

func (s *currencyService) ToBaseDecimal(
	ctx context.Context, amount money.Decimal, currencyCode string, on time.Time,
) (money.Decimal, money.Decimal, error) {
	baseCode, err := s.BaseCode(ctx)
	if err != nil {
		return money.Zero(), money.Zero(), err
	}
	return s.ConvertDecimal(ctx, amount, currencyCode, baseCode, on)
}

func (s *currencyService) BaseCode(ctx context.Context) (string, error) {
	base, err := s.repo.FindBase(ctx)
	if err != nil {
		if errors.Is(err, errs.ErrNotFound) {
			return "", errs.BadRequest("尚未设置本币，无法折算")
		}
		return "", err
	}
	return base.Code, nil
}

func (s *currencyService) Decimals(ctx context.Context, code string) (int, error) {
	c, err := s.repo.FindByCode(ctx, code)
	if err != nil {
		if errors.Is(err, errs.ErrNotFound) {
			return 0, errs.BadRequest("币种不存在：" + code)
		}
		return 0, err
	}
	return c.DecimalPlaces, nil
}

// rateOn 取某币种相对本币在 on 当日的折算率，本币自身恒为 1。
func (s *currencyService) rateOn(
	ctx context.Context, code, baseCode string, on time.Time,
) (money.Decimal, error) {
	if code == baseCode {
		return money.FromInt(1), nil
	}
	r, err := s.rates.FindEffective(ctx, code, baseCode, on)
	if err != nil {
		if errors.Is(err, errs.ErrNotFound) {
			return money.Zero(), errs.BadRequest(fmt.Sprintf("缺少 %s 在 %s 的汇率",
				code, model.DateOnly(on).Format("2006-01-02")))
		}
		return money.Zero(), err
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

// ToBaseDecimalFor 按指定汇率类型折本币（T-60）。
//
// 与 ToBaseDecimal 的差别只有一处：取价时带上类型。收款是"把外币卖给它"，
// 用买入价；付款是"向它买外币"，用卖出价。两者差 0.1%～0.5%，
// 一年累计下来足以让单票毛利的方向性偏差变成一条能被客户问到的数。
//
// 不走 ConvertDecimal：那条路要处理交叉汇率（两个币种各折一次再相除），
// 而这里的目标恒为本币，直接用一条汇率更少一次舍入。
func (s *currencyService) ToBaseDecimalFor(
	ctx context.Context, amount money.Decimal, currencyCode string, on time.Time, rateType string,
) (money.Decimal, money.Decimal, error) {
	baseCode, err := s.BaseCode(ctx)
	if err != nil {
		return money.Zero(), money.Zero(), err
	}
	code := strings.ToUpper(strings.TrimSpace(currencyCode))
	if code == "" || code == baseCode {
		return amount, money.FromInt(1), nil
	}
	rate, err := s.rates.FindEffectiveFor(ctx, code, baseCode, on, rateType)
	if err != nil {
		if errors.Is(err, errs.ErrNotFound) {
			return money.Zero(), money.Zero(), errs.BadRequest(fmt.Sprintf(
				"缺少 %s 在 %s 的汇率", code, model.DateOnly(on).Format("2006-01-02")))
		}
		return money.Zero(), money.Zero(), err
	}
	baseDP, err := s.Decimals(ctx, baseCode)
	if err != nil {
		return money.Zero(), money.Zero(), err
	}
	return amount.Mul(rate.Rate).Round(baseDP), rate.Rate, nil
}
