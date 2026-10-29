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

// PurchaseOrderService 管采购订单。
//
// 它是销售订单的镜像，也是毛利的另一半来源：销售侧收多少钱、采购侧付多少钱，
// 两者折到本币才能算出真正赚了多少。所以采购价必须落在这里，而不是回到产品表上。
type PurchaseOrderService interface {
	Create(ctx context.Context, in dto.PurchaseOrderInput) (*model.PurchaseOrder, error)
	Update(ctx context.Context, id uint, in dto.PurchaseOrderInput) (*model.PurchaseOrder, error)
	Delete(ctx context.Context, id uint) error
	Get(ctx context.Context, id uint) (*model.PurchaseOrder, error)
	List(ctx context.Context, q dto.PurchaseOrderQuery) (*dto.PageResult[model.PurchaseOrder], error)
	UpdateStatus(
		ctx context.Context, id uint, in dto.PurchaseOrderStatusInput,
	) (*model.PurchaseOrder, error)
}

type purchaseOrderService struct {
	orders     repository.PurchaseOrderRepository
	partners   repository.PartnerRepository
	products   repository.ProductRepository
	sources    repository.ProductSourceRepository
	salesOrds  repository.SalesOrderRepository
	currencies CurrencyService
	docNos     DocNoService
	clock      func() time.Time
	log        *slog.Logger
}

type PurchaseOrderOption func(*purchaseOrderService)

func WithPurchaseOrderLogger(l *slog.Logger) PurchaseOrderOption {
	return func(s *purchaseOrderService) { s.log = l }
}

// WithPurchaseOrderClock 注入时钟：下单日默认值、单号年月、汇率取哪一天都要可断言。
func WithPurchaseOrderClock(now func() time.Time) PurchaseOrderOption {
	return func(s *purchaseOrderService) { s.clock = now }
}

func NewPurchaseOrderService(
	orders repository.PurchaseOrderRepository,
	partners repository.PartnerRepository,
	products repository.ProductRepository,
	sources repository.ProductSourceRepository,
	salesOrds repository.SalesOrderRepository,
	currencies CurrencyService,
	docNos DocNoService,
	opts ...PurchaseOrderOption,
) PurchaseOrderService {
	s := &purchaseOrderService{
		orders:     orders,
		partners:   partners,
		products:   products,
		sources:    sources,
		salesOrds:  salesOrds,
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

func (s *purchaseOrderService) Create(
	ctx context.Context, in dto.PurchaseOrderInput,
) (*model.PurchaseOrder, error) {
	if err := ensureSupplierRole(ctx, s.partners, &in.SupplierID); err != nil {
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
	docNo, err := s.docNos.Next(ctx, model.DocTypePurchaseOrder)
	if err != nil {
		return nil, err
	}

	o := &model.PurchaseOrder{
		DocNo:           docNo,
		SupplierID:      in.SupplierID,
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
	s.log.Info("新增采购订单", "id", o.ID, "doc_no", o.DocNo,
		"supplier_id", o.SupplierID, "amount_base", o.AmountBaseTotal)
	return s.orders.FindByID(ctx, o.ID)
}

func (s *purchaseOrderService) Update(
	ctx context.Context, id uint, in dto.PurchaseOrderInput,
) (*model.PurchaseOrder, error) {
	existing, err := s.orders.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if !existing.IsEditable() {
		return nil, errs.ErrConflict.WithMsg(
			"只有草稿可以修改；已确认的采购单是给工厂的承诺，改动要走变更单（T-24）")
	}
	if in.SupplierID != 0 && in.SupplierID != existing.SupplierID {
		return nil, errs.BadRequest("采购单不能换供应商，请新建一张")
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

func (s *purchaseOrderService) Delete(ctx context.Context, id uint) error {
	existing, err := s.orders.FindByID(ctx, id)
	if err != nil {
		return err
	}
	if !existing.IsEditable() {
		return errs.ErrConflict.WithMsg("只有草稿可以删除；已确认的采购单请改为已取消")
	}
	return s.orders.Delete(ctx, id)
}

func (s *purchaseOrderService) Get(ctx context.Context, id uint) (*model.PurchaseOrder, error) {
	return s.orders.FindByID(ctx, id)
}

func (s *purchaseOrderService) List(
	ctx context.Context, q dto.PurchaseOrderQuery,
) (*dto.PageResult[model.PurchaseOrder], error) {
	q.PageQuery.Normalize()

	filter := repository.PurchaseOrderFilter{Keyword: q.Keyword, SupplierID: q.SupplierID}
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

// UpdateStatus 与销售订单共用同一套状态机与同一条跳转规则。
func (s *purchaseOrderService) UpdateStatus(
	ctx context.Context, id uint, in dto.PurchaseOrderStatusInput,
) (*model.PurchaseOrder, error) {
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
	s.log.Info("采购订单状态变更", "id", id, "from", existing.Status, "to", next)
	return s.orders.FindByID(ctx, id)
}

// purchaseOrderTerms 是校验并归一化之后的表头字段。
type purchaseOrderTerms struct {
	TradeTerm        model.TradeTerm
	CurrencyCode     string
	ExchangeRate     money.Decimal
	CurrencyDecimals int
	BaseDecimals     int
	OrderDate        time.Time
	DeliveryDate     *time.Time
	Remark           string
}

func (s *purchaseOrderService) buildTerms(
	ctx context.Context, in dto.PurchaseOrderInput,
) (purchaseOrderTerms, error) {
	var out purchaseOrderTerms

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
			return out, errs.BadRequest("到货日不能早于下单日")
		}
		deliveryDate = &d
	}

	// 采购币种与销售币种往往是两个（收美元、付人民币），
	// 因此这里单独锁一份汇率，不复用销售单上的那个。
	code, rate, curDP, baseDP, err := resolveCurrency(ctx, s.currencies, in.CurrencyCode, s.clock())
	if err != nil {
		return out, err
	}

	out = purchaseOrderTerms{
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

// buildItems 逐行校验引用并算金额，合计只在这里产生。
func (s *purchaseOrderService) buildItems(
	ctx context.Context, in []dto.PurchaseOrderItemInput, t purchaseOrderTerms,
) ([]model.PurchaseOrderItem, purchaseOrderTotals, error) {
	var totals purchaseOrderTotals
	items := make([]model.PurchaseOrderItem, 0, len(in))

	for _, row := range in {
		if row.Qty.Cmp(money.Zero()) <= 0 {
			return nil, totals, errs.BadRequest("明细数量必须大于 0")
		}
		if row.UnitPrice.IsNegative() {
			return nil, totals, errs.BadRequest("明细单价不能为负")
		}
		product, err := s.products.FindByID(ctx, row.ProductID)
		if err != nil {
			if errors.Is(err, errs.ErrNotFound) {
				return nil, totals, errs.BadRequest(fmt.Sprintf("产品不存在（id=%d）", row.ProductID))
			}
			return nil, totals, err
		}

		if err := s.checkSourceRef(ctx, product, row.SourceID); err != nil {
			return nil, totals, err
		}
		if err := s.checkSalesOrderRef(ctx, product, row.SalesOrderItemID); err != nil {
			return nil, totals, err
		}

		// 数量列是 decimal(14,3)：先按 3 位定下来，再乘单价与汇率。
		qty := row.Qty.Round(3)
		amount := row.UnitPrice.Mul(qty).Round(t.CurrencyDecimals)
		amountBase := amount.Mul(t.ExchangeRate).Round(t.BaseDecimals)

		items = append(items, model.PurchaseOrderItem{
			ProductID:        product.ID,
			SKUCode:          product.SKUCode,
			NameCN:           product.NameCN,
			Spec:             product.Spec,
			Unit:             product.Unit,
			Qty:              qty,
			UnitPrice:        row.UnitPrice,
			Amount:           amount,
			AmountBase:       amountBase,
			SalesOrderItemID: row.SalesOrderItemID,
			SourceID:         row.SourceID,
			Remark:           row.Remark,
		})

		totals.Amount = totals.Amount.Add(amount)
		totals.AmountBase = totals.AmountBase.Add(amountBase)
	}

	totals.Amount = totals.Amount.Round(t.CurrencyDecimals)
	totals.AmountBase = totals.AmountBase.Round(t.BaseDecimals)
	return items, totals, nil
}

// checkSourceRef 校验货源引用：必须存在、属于这个产品、且没停用。
func (s *purchaseOrderService) checkSourceRef(
	ctx context.Context, product *model.Product, sourceID *uint,
) error {
	if sourceID == nil || *sourceID == 0 {
		return nil
	}
	src, err := s.sources.FindByID(ctx, *sourceID)
	if err != nil {
		if errors.Is(err, errs.ErrNotFound) {
			return errs.BadRequest("指定的货源不存在")
		}
		return err
	}
	if src.ProductID != product.ID {
		return errs.BadRequest("指定的货源不属于这个产品：" + product.SKUCode)
	}
	if src.Status != model.EnableStatusOn {
		return errs.BadRequest("指定的货源已停用，不能再据此下单")
	}
	return nil
}

// checkSalesOrderRef 校验「以销定采」的挂接：行存在、产品一致、销售单没被取消。
//
// 不校验"累计采购量是否超过销售数量"：分批向两家工厂下单是常态，
// 卡在采购单上会逼人拆单造假。用量对账属于订单跟进（T-22），不是下单时的门禁。
func (s *purchaseOrderService) checkSalesOrderRef(
	ctx context.Context, product *model.Product, soItemID *uint,
) error {
	if soItemID == nil || *soItemID == 0 {
		return nil
	}
	item, err := s.salesOrds.FindItemByID(ctx, *soItemID)
	if err != nil {
		if errors.Is(err, errs.ErrNotFound) {
			return errs.BadRequest("关联的销售订单行不存在")
		}
		return err
	}
	if item.ProductID != product.ID {
		return errs.BadRequest("关联的销售订单行属于另一个产品：" + product.SKUCode)
	}
	if item.SalesOrder != nil && item.SalesOrder.Status == model.DocumentStatusCancelled {
		return errs.ErrConflict.WithMsg("关联的销售订单已取消，不能据此采购")
	}
	return nil
}

// purchaseOrderTotals 是采购单合计。
type purchaseOrderTotals struct {
	Amount     money.Decimal
	AmountBase money.Decimal
}
