package service

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/errs"
	"github.com/edwinhuish/gowork/internal/repository"
)

const rateDateLayout = "2006-01-02"

// ExchangeRateService 管汇率。
//
// 只提供新增、查询、删除，**没有编辑**：历史汇率是单据成本的依据，
// 允许改一条三个月前的价，等于让那段时间所有报价的毛利在事后变成另一个数。
// 录错了就删掉重录——删除会留下软删除记录，需要时仍可追查。
type ExchangeRateService interface {
	Create(ctx context.Context, in dto.ExchangeRateInput) (*model.ExchangeRate, error)
	Delete(ctx context.Context, id uint) error
	Get(ctx context.Context, id uint) (*model.ExchangeRate, error)
	List(ctx context.Context, q dto.ExchangeRateQuery) (*dto.PageResult[model.ExchangeRate], error)
}

type exchangeRateService struct {
	rates      repository.ExchangeRateRepository
	currencies repository.CurrencyRepository
	log        *slog.Logger
}

type ExchangeRateOption func(*exchangeRateService)

func WithExchangeRateLogger(l *slog.Logger) ExchangeRateOption {
	return func(s *exchangeRateService) { s.log = l }
}

func NewExchangeRateService(
	rates repository.ExchangeRateRepository,
	currencies repository.CurrencyRepository,
	opts ...ExchangeRateOption,
) ExchangeRateService {
	s := &exchangeRateService{rates: rates, currencies: currencies, log: slog.Default()}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *exchangeRateService) Create(
	ctx context.Context, in dto.ExchangeRateInput,
) (*model.ExchangeRate, error) {
	code, err := normCurrencyCode(in.CurrencyCode)
	if err != nil {
		return nil, err
	}
	if in.Rate <= 0 {
		return nil, errs.BadRequest("汇率必须大于 0")
	}
	date, err := parseRateDate(in.EffectiveDate)
	if err != nil {
		return nil, err
	}

	if _, err := s.currencies.FindByCode(ctx, code); err != nil {
		if errors.Is(err, errs.ErrNotFound) {
			return nil, errs.BadRequest("币种不存在，请先新增币种：" + code)
		}
		return nil, err
	}
	base, err := s.currencies.FindBase(ctx)
	if err != nil {
		if errors.Is(err, errs.ErrNotFound) {
			return nil, errs.BadRequest("尚未设置本币，无法登记汇率")
		}
		return nil, err
	}
	if code == base.Code {
		// 本币相对自身恒为 1，存一条"1 比 1"的汇率只会让人以为它是可配的。
		return nil, errs.BadRequest("本币相对自身不需要汇率：" + code)
	}

	dup, err := s.rates.ExistsByPair(ctx, code, base.Code, date, 0)
	if err != nil {
		return nil, err
	}
	if dup {
		return nil, errs.ErrConflict.WithMsg("该币种在这一天已有汇率：" +
			code + " " + date.Format(rateDateLayout))
	}

	m := &model.ExchangeRate{
		CurrencyCode:  code,
		BaseCode:      base.Code,
		Rate:          in.Rate,
		EffectiveDate: date,
		Source:        in.Source,
		Remark:        in.Remark,
	}
	if err := s.rates.Create(ctx, m); err != nil {
		return nil, err
	}

	s.log.Info("新增汇率", "id", m.ID, "pair", code+"/"+base.Code, "rate", m.Rate,
		"date", date.Format(rateDateLayout))
	return m, nil
}

func (s *exchangeRateService) Delete(ctx context.Context, id uint) error {
	return s.rates.Delete(ctx, id)
}

func (s *exchangeRateService) Get(ctx context.Context, id uint) (*model.ExchangeRate, error) {
	return s.rates.FindByID(ctx, id)
}

func (s *exchangeRateService) List(
	ctx context.Context, q dto.ExchangeRateQuery,
) (*dto.PageResult[model.ExchangeRate], error) {
	q.PageQuery.Normalize()

	filter := repository.ExchangeRateFilter{
		CurrencyCode: strings.ToUpper(strings.TrimSpace(q.CurrencyCode)),
		BaseCode:     strings.ToUpper(strings.TrimSpace(q.BaseCode)),
	}
	page := repository.Pagination{Page: q.Page, PageSize: q.PageSize}

	list, total, err := s.rates.List(ctx, filter, page)
	if err != nil {
		return nil, err
	}
	return dto.NewPageResult(list, total, q.Page, q.PageSize), nil
}

// parseRateDate 只取日期部分，顺手兼容前端可能带上来的完整时间串。
func parseRateDate(raw string) (time.Time, error) {
	s := strings.TrimSpace(raw)
	if len(s) > len(rateDateLayout) {
		s = s[:len(rateDateLayout)]
	}
	d, err := time.Parse(rateDateLayout, s)
	if err != nil {
		return time.Time{}, errs.BadRequest("生效日期格式应为 YYYY-MM-DD")
	}
	return model.DateOnly(d), nil
}
