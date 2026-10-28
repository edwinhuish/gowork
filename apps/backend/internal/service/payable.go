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

// PayableService 管应付单：由采购订单生成、锁汇率、支持分期。
//
// 与应收镜像，但方向相反：对方是供应商、来源只有采购订单。
// 应付的币种通常与销售不同（收美元、付人民币），所以采购单上的汇率
// 必须原样锁进应付单——拿销售的汇率去折，等于把两笔独立的汇兑风险混成一笔。
type PayableService interface {
	Create(ctx context.Context, in dto.PayableInput) (*model.Payable, error)
	Update(ctx context.Context, id uint, in dto.PayableInput) (*model.Payable, error)
	Delete(ctx context.Context, id uint) error
	Get(ctx context.Context, id uint) (*model.Payable, error)
	List(ctx context.Context, q dto.PayableQuery) (*dto.PageResult[model.Payable], error)
}

type payableService struct {
	repo     repository.PayableRepository
	orders   repository.PurchaseOrderRepository
	partners repository.PartnerRepository
	docNos   DocNoService
	clock    func() time.Time
	log      *slog.Logger
}

type PayableOption func(*payableService)

func WithPayableClock(now func() time.Time) PayableOption {
	return func(s *payableService) { s.clock = now }
}

func WithPayableLogger(l *slog.Logger) PayableOption {
	return func(s *payableService) { s.log = l }
}

func NewPayableService(
	repo repository.PayableRepository,
	orders repository.PurchaseOrderRepository,
	partners repository.PartnerRepository,
	docNos DocNoService,
	opts ...PayableOption,
) PayableService {
	s := &payableService{
		repo:     repo,
		orders:   orders,
		partners: partners,
		docNos:   docNos,
		clock:    time.Now,
		log:      slog.Default(),
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *payableService) Create(
	ctx context.Context, in dto.PayableInput,
) (*model.Payable, error) {
	terms, err := s.buildTerms(ctx, in, 0)
	if err != nil {
		return nil, err
	}
	docNo, err := s.docNos.Next(ctx, model.DocTypePayable)
	if err != nil {
		return nil, err
	}

	m := &model.Payable{Settlement: model.Settlement{
		DocNo:        docNo,
		PartnerID:    terms.PartnerID,
		SourceType:   model.SettlementSourcePurchaseOrder,
		SourceID:     in.SourceID,
		SourceDocNo:  terms.SourceDocNo,
		CurrencyCode: terms.CurrencyCode,
		ExchangeRate: terms.ExchangeRate,
		Amount:       terms.Amount,
		AmountBase:   terms.AmountBase,
		DueDate:      terms.DueDate,
		Status:       model.SettlementStatusOpen,
		Remark:       in.Remark,
	}}
	if err := s.repo.Create(ctx, m); err != nil {
		return nil, err
	}
	s.log.Info("新增应付单", "doc_no", m.DocNo, "source", m.SourceDocNo,
		"amount", m.Amount, "currency", m.CurrencyCode)
	return s.repo.FindByID(ctx, m.ID)
}

func (s *payableService) Update(
	ctx context.Context, id uint, in dto.PayableInput,
) (*model.Payable, error) {
	existing, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if in.SourceID != 0 && in.SourceID != existing.SourceID {
		return nil, errs.BadRequest("应付单不能改挂到另一张采购单，请在新采购单下新建")
	}
	if existing.Status == model.SettlementStatusVoid {
		return nil, errs.ErrConflict.WithMsg("应付单已作废，不能再改")
	}
	if existing.SettledAmount > 0 {
		return nil, errs.ErrConflict.WithMsg(
			fmt.Sprintf("应付单已有付款核销（%s %.4f），金额与到期日不能再改",
				existing.CurrencyCode, existing.SettledAmount))
	}

	terms, err := s.buildTerms(ctx, in, existing.ID)
	if err != nil {
		return nil, err
	}

	status := existing.Status
	if in.Status != nil {
		if model.SettlementStatus(*in.Status) != model.SettlementStatusVoid {
			return nil, errs.BadRequest("应付单手改状态只支持作废（90）；结清状态由付款核销推导")
		}
		status = model.SettlementStatusVoid
	}

	fields := map[string]any{
		"amount":        terms.Amount,
		"amount_base":   terms.AmountBase,
		"currency_code": terms.CurrencyCode,
		"exchange_rate": terms.ExchangeRate,
		"due_date":      terms.DueDate,
		"status":        status,
		"remark":        in.Remark,
	}
	if err := s.repo.Update(ctx, id, fields); err != nil {
		return nil, err
	}
	return s.repo.FindByID(ctx, id)
}

func (s *payableService) Delete(ctx context.Context, id uint) error {
	existing, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return err
	}
	if existing.SettledAmount > 0 {
		return errs.ErrConflict.WithMsg("已有付款核销的应付单不能删除，请改为作废")
	}
	return s.repo.Delete(ctx, id)
}

func (s *payableService) Get(ctx context.Context, id uint) (*model.Payable, error) {
	return s.repo.FindByID(ctx, id)
}

func (s *payableService) List(
	ctx context.Context, q dto.PayableQuery,
) (*dto.PageResult[model.Payable], error) {
	q.PageQuery.Normalize()

	filter := repository.PayableFilter{PartnerID: q.PartnerID}
	if q.Status != nil {
		st := model.SettlementStatus(*q.Status)
		filter.Status = &st
	}
	if q.Overdue != nil && *q.Overdue == 1 {
		today := model.DateOnly(s.clock())
		filter.OverdueBefore = &today
	}
	page := repository.Pagination{Page: q.Page, PageSize: q.PageSize}

	list, total, err := s.repo.List(ctx, filter, page)
	if err != nil {
		return nil, err
	}
	return dto.NewPageResult(list, total, q.Page, q.PageSize), nil
}

type payableTerms struct {
	PartnerID    uint
	SourceDocNo  string
	CurrencyCode string
	ExchangeRate float64
	Amount       float64
	AmountBase   float64
	DueDate      time.Time
}

func (s *payableService) buildTerms(
	ctx context.Context, in dto.PayableInput, excludeID uint,
) (payableTerms, error) {
	var out payableTerms

	if model.SettlementSource(in.SourceType) != model.SettlementSourcePurchaseOrder {
		return out, errs.BadRequest("应付只能由采购订单生成（source_type=30）")
	}

	order, err := s.orders.FindByID(ctx, in.SourceID)
	if err != nil {
		if errors.Is(err, errs.ErrNotFound) {
			return out, errs.BadRequest("来源采购订单不存在")
		}
		return out, err
	}
	if order.Status == model.DocumentStatusDraft || order.Status == model.DocumentStatusCancelled {
		return out, errs.BadRequest(
			fmt.Sprintf("采购订单 %s 是%s，不能开应付", order.DocNo, order.Status.Label()))
	}
	if order.AmountTotal.Cmp(money.Zero()) <= 0 {
		return out, errs.BadRequest(fmt.Sprintf("采购订单 %s 金额为 0，不能开应付", order.DocNo))
	}

	issued, err := s.repo.SumAmountBySource(
		ctx, model.SettlementSourcePurchaseOrder, order.ID, excludeID)
	if err != nil {
		return out, err
	}
	// 已开票金额还在结算表里（float64，改造到资金那一片时去掉这次往返）。
	remaining := order.AmountTotal.Sub(money.FromFloat(issued)).Round(4).Float64()
	if remaining <= 0 {
		return out, errs.ErrConflict.WithMsg(
			fmt.Sprintf("采购订单 %s 已全额开票（%s %.4f）", order.DocNo, order.CurrencyCode, issued))
	}

	amount := in.Amount
	if amount == 0 {
		amount = remaining
	}
	amount = money.Round(amount, 4)
	if amount > remaining {
		return out, errs.BadRequest(
			fmt.Sprintf("金额超出未开票余额：%s 最多可开 %.4f", order.DocNo, remaining))
	}

	// 供应商账期与销售侧是两套：付工厂通常是月结 30 天，收客户可能是即期。
	dueDate, err := s.resolveDueDate(ctx, in.DueDate, order.OrderDate, order.SupplierID)
	if err != nil {
		return out, err
	}

	out = payableTerms{
		PartnerID:    order.SupplierID,
		SourceDocNo:  order.DocNo,
		CurrencyCode: order.CurrencyCode,
		ExchangeRate: order.ExchangeRate.Float64(),
		Amount:       amount,
		AmountBase:   money.Round(amount*order.ExchangeRate.Float64(), 4),
		DueDate:      dueDate,
	}
	return out, nil
}

func (s *payableService) resolveDueDate(
	ctx context.Context, raw string, base time.Time, partnerID uint,
) (time.Time, error) {
	if strings.TrimSpace(raw) != "" {
		d, err := parseDateOnly(raw)
		if err != nil {
			return time.Time{}, errs.BadRequest("到期日格式应为 YYYY-MM-DD")
		}
		return d, nil
	}
	p, err := s.partners.FindByID(ctx, partnerID)
	if err != nil {
		return time.Time{}, err
	}
	return model.DateOnly(base).AddDate(0, 0, p.CreditDays), nil
}
