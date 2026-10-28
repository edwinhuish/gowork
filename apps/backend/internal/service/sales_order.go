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

// SalesOrderService 管销售订单。
//
// 订单只解决"客户要什么、多少、什么价、什么时候交"。成本不在这里：
// 采购价在 T-12、运杂费在出货环节，毛利要等两者落定才算（T-22）。
// 在订单上手填一个成本，等于给毛利留了第二个来源。
type SalesOrderService interface {
	Create(ctx context.Context, in dto.SalesOrderInput) (*model.SalesOrder, error)
	Update(ctx context.Context, id uint, in dto.SalesOrderInput) (*model.SalesOrder, error)
	Delete(ctx context.Context, id uint) error
	Get(ctx context.Context, id uint) (*model.SalesOrder, error)
	List(ctx context.Context, q dto.SalesOrderQuery) (*dto.PageResult[model.SalesOrder], error)
	UpdateStatus(ctx context.Context, id uint, in dto.SalesOrderStatusInput) (*model.SalesOrder, error)
}

type salesOrderService struct {
	orders     repository.SalesOrderRepository
	partners   repository.PartnerRepository
	products   repository.ProductRepository
	currencies CurrencyService
	docNos     DocNoService
	clock      func() time.Time
	log        *slog.Logger
}

type SalesOrderOption func(*salesOrderService)

func WithSalesOrderLogger(l *slog.Logger) SalesOrderOption {
	return func(s *salesOrderService) { s.log = l }
}

// WithSalesOrderClock 注入时钟：下单日默认值、单号年月、汇率取哪一天都要可断言。
func WithSalesOrderClock(now func() time.Time) SalesOrderOption {
	return func(s *salesOrderService) { s.clock = now }
}

func NewSalesOrderService(
	orders repository.SalesOrderRepository,
	partners repository.PartnerRepository,
	products repository.ProductRepository,
	currencies CurrencyService,
	docNos DocNoService,
	opts ...SalesOrderOption,
) SalesOrderService {
	s := &salesOrderService{
		orders:     orders,
		partners:   partners,
		products:   products,
		currencies: currencies,
		docNos:     docNos,
		clock:      time.Now,
		log:        slog.Default(),
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *salesOrderService) Create(
	ctx context.Context, in dto.SalesOrderInput,
) (*model.SalesOrder, error) {
	if err := ensureCustomerRole(ctx, s.partners, in.PartnerID); err != nil {
		return nil, err
	}
	terms, err := s.buildTerms(ctx, in)
	if err != nil {
		return nil, err
	}
	items, totals, err := s.buildItems(ctx, in.Items, terms)
	if err != nil {
		return nil, err
	}

	// 取号放最后：前面任何一步失败都不该消耗一个单号。
	docNo, err := s.docNos.Next(ctx, model.DocTypeSalesOrder)
	if err != nil {
		return nil, err
	}

	o := &model.SalesOrder{
		DocNo:           docNo,
		PartnerID:       in.PartnerID,
		TradeTerm:       terms.TradeTerm,
		Status:          model.DocumentStatusDraft,
		OrderDate:       terms.OrderDate,
		DeliveryDate:    terms.DeliveryDate,
		CurrencyCode:    terms.CurrencyCode,
		ExchangeRate:    terms.ExchangeRate,
		AmountTotal:     totals.Amount,
		AmountBaseTotal: totals.AmountBase,
		Remark:          terms.Remark,
	}
	if err := s.orders.CreateWithItems(ctx, o, items); err != nil {
		return nil, err
	}
	s.log.Info("新增销售订单", "id", o.ID, "doc_no", o.DocNo,
		"partner_id", o.PartnerID, "amount_base", o.AmountBaseTotal)
	return s.orders.FindByID(ctx, o.ID)
}

func (s *salesOrderService) Update(
	ctx context.Context, id uint, in dto.SalesOrderInput,
) (*model.SalesOrder, error) {
	existing, err := s.orders.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if !existing.IsEditable() {
		return nil, errs.ErrConflict.WithMsg(
			"只有草稿可以修改；已确认的订单是对客户的承诺，改动要走变更单（T-24）")
	}
	if in.PartnerID != 0 && in.PartnerID != existing.PartnerID {
		return nil, errs.BadRequest("订单不能改客户，请新建一张")
	}
	terms, err := s.buildTerms(ctx, in)
	if err != nil {
		return nil, err
	}
	items, totals, err := s.buildItems(ctx, in.Items, terms)
	if err != nil {
		return nil, err
	}

	fields := map[string]any{
		"trade_term":        terms.TradeTerm,
		"order_date":        terms.OrderDate,
		"delivery_date":     terms.DeliveryDate,
		"currency_code":     terms.CurrencyCode,
		"exchange_rate":     terms.ExchangeRate,
		"amount_total":      totals.Amount,
		"amount_base_total": totals.AmountBase,
		"remark":            terms.Remark,
	}
	if err := s.orders.UpdateWithItems(ctx, id, fields, items); err != nil {
		return nil, err
	}
	return s.orders.FindByID(ctx, id)
}

func (s *salesOrderService) Delete(ctx context.Context, id uint) error {
	existing, err := s.orders.FindByID(ctx, id)
	if err != nil {
		return err
	}
	if !existing.IsEditable() {
		// 已确认的订单留在库里供追溯；不要了走"取消"，而不是抹掉。
		return errs.ErrConflict.WithMsg("只有草稿可以删除；已确认的订单请改为已取消")
	}
	return s.orders.Delete(ctx, id)
}

func (s *salesOrderService) Get(ctx context.Context, id uint) (*model.SalesOrder, error) {
	return s.orders.FindByID(ctx, id)
}

func (s *salesOrderService) List(
	ctx context.Context, q dto.SalesOrderQuery,
) (*dto.PageResult[model.SalesOrder], error) {
	q.PageQuery.Normalize()

	filter := repository.SalesOrderFilter{Keyword: q.Keyword, PartnerID: q.PartnerID}
	if q.Status != nil {
		st := model.DocumentStatus(*q.Status)
		if !st.IsValid() {
			return nil, errs.BadRequest("status 不合法")
		}
		filter.Status = &st
	}
	page := repository.Pagination{Page: q.Page, PageSize: q.PageSize}

	list, total, err := s.orders.List(ctx, filter, page)
	if err != nil {
		return nil, err
	}
	return dto.NewPageResult(list, total, q.Page, q.PageSize), nil
}

// UpdateStatus 走完整状态机：草稿 → 已确认 → 备货中 → 已出货 → 已完成，
// 未出货前可取消。跳步（草稿直接到已出货）与往回退都由 checkDocumentTransit 拒绝。
func (s *salesOrderService) UpdateStatus(
	ctx context.Context, id uint, in dto.SalesOrderStatusInput,
) (*model.SalesOrder, error) {
	existing, err := s.orders.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	next := model.DocumentStatus(in.Status)
	if !next.IsValid() {
		return nil, errs.BadRequest("status 不合法")
	}
	if err := checkDocumentTransit(existing.Status, next); err != nil {
		return nil, err
	}
	if err := s.orders.Update(ctx, id, map[string]any{"status": next}); err != nil {
		return nil, err
	}
	s.log.Info("销售订单状态变更", "id", id, "from", existing.Status, "to", next)
	return s.orders.FindByID(ctx, id)
}

// salesOrderTerms 是校验并归一化之后的表头字段。
type salesOrderTerms struct {
	TradeTerm        model.TradeTerm
	CurrencyCode     string
	ExchangeRate     float64
	CurrencyDecimals int
	BaseDecimals     int
	OrderDate        time.Time
	DeliveryDate     *time.Time
	Remark           string
}

func (s *salesOrderService) buildTerms(
	ctx context.Context, in dto.SalesOrderInput,
) (salesOrderTerms, error) {
	var out salesOrderTerms

	term := model.TradeTerm(strings.ToUpper(strings.TrimSpace(in.TradeTerm)))
	if !term.IsValid() {
		return out, errInvalidTradeTerm
	}

	orderDate := model.DateOnly(s.clock())
	if strings.TrimSpace(in.OrderDate) != "" {
		d, err := parseDateOnly(in.OrderDate)
		if err != nil {
			return out, err
		}
		orderDate = d
	}

	var deliveryDate *time.Time
	if strings.TrimSpace(in.DeliveryDate) != "" {
		d, err := parseDateOnly(in.DeliveryDate)
		if err != nil {
			return out, err
		}
		if d.Before(orderDate) {
			return out, errs.BadRequest("交期不能早于下单日")
		}
		deliveryDate = &d
	}

	code, rate, curDP, baseDP, err := resolveCurrency(ctx, s.currencies, in.CurrencyCode, s.clock())
	if err != nil {
		return out, err
	}

	out = salesOrderTerms{
		TradeTerm:        term,
		CurrencyCode:     code,
		ExchangeRate:     rate,
		CurrencyDecimals: curDP,
		BaseDecimals:     baseDP,
		OrderDate:        orderDate,
		DeliveryDate:     deliveryDate,
		Remark:           in.Remark,
	}
	return out, nil
}

// buildItems 逐行算金额并汇总整单合计。
//
// 合计只在这里产生：入参里没有总额字段，前端传什么都到不了这一步。
func (s *salesOrderService) buildItems(
	ctx context.Context, in []dto.SalesOrderItemInput, t salesOrderTerms,
) ([]model.SalesOrderItem, salesOrderTotals, error) {
	var totals salesOrderTotals
	items := make([]model.SalesOrderItem, 0, len(in))

	for _, row := range in {
		if row.Qty <= 0 {
			return nil, totals, errs.BadRequest("明细数量必须大于 0")
		}
		if row.UnitPrice < 0 {
			return nil, totals, errs.BadRequest("明细单价不能为负")
		}
		product, err := s.products.FindByID(ctx, row.ProductID)
		if err != nil {
			if errors.Is(err, errs.ErrNotFound) {
				return nil, totals, errs.BadRequest(fmt.Sprintf("产品不存在（id=%d）", row.ProductID))
			}
			return nil, totals, err
		}

		amount := money.Round(row.UnitPrice*row.Qty, t.CurrencyDecimals)
		amountBase := money.Round(amount*t.ExchangeRate, t.BaseDecimals)

		items = append(items, model.SalesOrderItem{
			ProductID:  product.ID,
			SKUCode:    product.SKUCode,
			NameCN:     product.NameCN,
			Spec:       product.Spec,
			Unit:       product.Unit,
			Qty:        row.Qty,
			UnitPrice:  row.UnitPrice,
			Amount:     amount,
			AmountBase: amountBase,
			Remark:     row.Remark,
		})

		totals.Amount += amount
		totals.AmountBase += amountBase
	}

	totals.Amount = money.Round(totals.Amount, t.CurrencyDecimals)
	totals.AmountBase = money.Round(totals.AmountBase, t.BaseDecimals)
	return items, totals, nil
}

// salesOrderTotals 是订单合计。
type salesOrderTotals struct {
	Amount     float64
	AmountBase float64
}
