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

var errInvalidChargeType = errs.BadRequest("费用类型不合法：10=海运费，20=报关费，30=拖车费，40=保险费，90=其它")

// ShipmentChargeService 管归集到一票货的运费杂费。
//
// **它不校验出货批次的状态。** 货代账单通常晚于出货到达，要求"草稿才能录费用"
// 等于让人把已经发出的货退回草稿——而费用恰恰是事后才准的那部分。
// 这一点与出货数量不同：数量必须在发货前定，金额往往是发货后才结算。
type ShipmentChargeService interface {
	Create(ctx context.Context, in dto.ShipmentChargeInput) (*model.ShipmentCharge, error)
	Update(ctx context.Context, id uint, in dto.ShipmentChargeInput) (*model.ShipmentCharge, error)
	Delete(ctx context.Context, id uint) error
	Get(ctx context.Context, id uint) (*model.ShipmentCharge, error)
	List(ctx context.Context, q dto.ShipmentChargeQuery) (*dto.PageResult[model.ShipmentCharge], error)
	Summary(ctx context.Context, shipmentID uint) (*dto.ShipmentChargeSummary, error)
}

type shipmentChargeService struct {
	repo       repository.ShipmentChargeRepository
	shipments  repository.ShipmentRepository
	partners   repository.PartnerRepository
	currencies CurrencyService
	// clock 注入时钟：费用按录入当日汇率折本币，测试要能指定"今天"。
	clock func() time.Time
	log   *slog.Logger
}

type ShipmentChargeOption func(*shipmentChargeService)

func WithShipmentChargeLogger(l *slog.Logger) ShipmentChargeOption {
	return func(s *shipmentChargeService) { s.log = l }
}

func WithShipmentChargeClock(now func() time.Time) ShipmentChargeOption {
	return func(s *shipmentChargeService) { s.clock = now }
}

func NewShipmentChargeService(
	repo repository.ShipmentChargeRepository,
	shipments repository.ShipmentRepository,
	partners repository.PartnerRepository,
	currencies CurrencyService,
	opts ...ShipmentChargeOption,
) ShipmentChargeService {
	s := &shipmentChargeService{
		repo:       repo,
		shipments:  shipments,
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

func (s *shipmentChargeService) Create(
	ctx context.Context, in dto.ShipmentChargeInput,
) (*model.ShipmentCharge, error) {
	if _, err := s.shipments.FindByID(ctx, in.ShipmentID); err != nil {
		if errors.Is(err, errs.ErrNotFound) {
			return nil, errs.BadRequest("出货批次不存在")
		}
		return nil, err
	}
	terms, err := s.buildTerms(ctx, in)
	if err != nil {
		return nil, err
	}

	m := &model.ShipmentCharge{
		ShipmentID:   in.ShipmentID,
		Type:         terms.Type,
		PartnerID:    terms.PartnerID,
		Amount:       terms.Amount,
		CurrencyCode: terms.CurrencyCode,
		AmountBase:   terms.AmountBase,
		ExchangeRate: terms.ExchangeRate,
		Remark:       terms.Remark,
	}
	if err := s.repo.Create(ctx, m); err != nil {
		return nil, err
	}
	s.log.Info("新增运费杂费", "id", m.ID, "shipment_id", m.ShipmentID,
		"type", m.Type.Label(), "amount_base", m.AmountBase)
	return s.repo.FindByID(ctx, m.ID)
}

func (s *shipmentChargeService) Update(
	ctx context.Context, id uint, in dto.ShipmentChargeInput,
) (*model.ShipmentCharge, error) {
	existing, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	// 换一票货等于这条费用描述的是另一次运输；两票的成本都会因此算错。
	// 让用户在新票下新建一条，旧票那条删掉——历史清单才解释得清。
	if in.ShipmentID != 0 && in.ShipmentID != existing.ShipmentID {
		return nil, errs.BadRequest("费用不能改挂到另一票货，请在新票下新建")
	}
	terms, err := s.buildTerms(ctx, in)
	if err != nil {
		return nil, err
	}

	fields := map[string]any{
		"type":          terms.Type,
		"partner_id":    terms.PartnerID,
		"amount":        terms.Amount,
		"currency_code": terms.CurrencyCode,
		"amount_base":   terms.AmountBase,
		"exchange_rate": terms.ExchangeRate,
		"remark":        terms.Remark,
	}
	if err := s.repo.Update(ctx, id, fields); err != nil {
		return nil, err
	}
	return s.repo.FindByID(ctx, id)
}

func (s *shipmentChargeService) Delete(ctx context.Context, id uint) error {
	return s.repo.Delete(ctx, id)
}

func (s *shipmentChargeService) Get(ctx context.Context, id uint) (*model.ShipmentCharge, error) {
	return s.repo.FindByID(ctx, id)
}

func (s *shipmentChargeService) List(
	ctx context.Context, q dto.ShipmentChargeQuery,
) (*dto.PageResult[model.ShipmentCharge], error) {
	q.PageQuery.Normalize()

	filter := repository.ShipmentChargeFilter{ShipmentID: q.ShipmentID}
	if q.Type != nil {
		t := model.ShipmentChargeType(*q.Type)
		filter.Type = &t
	}
	page := repository.Pagination{Page: q.Page, PageSize: q.PageSize}

	list, total, err := s.repo.List(ctx, filter, page)
	if err != nil {
		return nil, err
	}
	return dto.NewPageResult(list, total, q.Page, q.PageSize), nil
}

func (s *shipmentChargeService) Summary(
	ctx context.Context, shipmentID uint,
) (*dto.ShipmentChargeSummary, error) {
	if _, err := s.shipments.FindByID(ctx, shipmentID); err != nil {
		return nil, err
	}
	total, err := s.repo.SumBaseByShipmentID(ctx, shipmentID)
	if err != nil {
		return nil, err
	}
	return &dto.ShipmentChargeSummary{ShipmentID: shipmentID, TotalBase: total}, nil
}

// shipmentChargeTerms 是校验并折算之后的费用字段。
type shipmentChargeTerms struct {
	Type         model.ShipmentChargeType
	PartnerID    *uint
	Amount       money.Decimal
	CurrencyCode string
	AmountBase   money.Decimal
	ExchangeRate money.Decimal
	Remark       string
}

func (s *shipmentChargeService) buildTerms(
	ctx context.Context, in dto.ShipmentChargeInput,
) (shipmentChargeTerms, error) {
	var out shipmentChargeTerms

	t := model.ShipmentChargeType(in.Type)
	if !t.IsValid() {
		return out, errInvalidChargeType
	}
	if in.Amount.IsNegative() {
		return out, errs.BadRequest("费用金额不能为负；要冲减请删除该条重录")
	}
	if err := s.checkPayee(ctx, in.PartnerID); err != nil {
		return out, err
	}

	// 币种留空即本币。折算走 CurrencyService：缺当日汇率时给 400，
	// 而不是把 0 写进本币金额——那会让这票货的成本凭空少一块。
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

	out = shipmentChargeTerms{
		Type:         t,
		PartnerID:    in.PartnerID,
		Amount:       in.Amount,
		CurrencyCode: code,
		AmountBase:   amountBase,
		ExchangeRate: rate,
		Remark:       in.Remark,
	}
	return out, nil
}

// checkPayee 确认收款方是货代或供应商。
//
// 不校验启用状态：已经停用的货代，手里的旧账单仍要能补录。
// 停用的主体在新建下拉里已经选不到，新引用进不来，这就够了。
func (s *shipmentChargeService) checkPayee(ctx context.Context, id *uint) error {
	if id == nil || *id == 0 {
		return nil
	}
	p, err := s.partners.FindByID(ctx, *id)
	if err != nil {
		if errors.Is(err, errs.ErrNotFound) {
			return errs.BadRequest("指定的收款主体不存在")
		}
		return err
	}
	if !p.Role.Has(model.PartnerRoleForwarder) && !p.Role.Has(model.PartnerRoleSupplier) {
		return errs.BadRequest(fmt.Sprintf("费用只能挂给货代或供应商：%s", p.Name))
	}
	return nil
}
