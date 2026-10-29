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
	// CreateFromQuotation 按报价单锁定的币种、汇率与明细生成订单草稿（T-13）。
	//
	// 它不重取当日汇率：报价单上的价与汇率是一个整体，拆开重算等于在转单的一瞬间改价。
	// 这个方法只给转换流程用，不进 HTTP 入参，所以客户端无法指定汇率。
	CreateFromQuotation(ctx context.Context, in QuotationOrderInput) (*model.SalesOrder, error)
}

// QuotationOrderInput 是"按报价单生成订单"的入参：字段全部来自已确认的报价单。
type QuotationOrderInput struct {
	PartnerID uint
	// BaseNo 与 DealSeq 来自来源报价单：一笔生意一张案号，报价转订单沿用同一个，
	// 序号也不变（QT03 转出必得 SO03）。
	// 存量的报价单（T-47 之前）这两项为空，此时订单取一笔新生意。
	BaseNo       string
	DealSeq      uint
	TradeTerm    model.TradeTerm
	CurrencyCode string
	ExchangeRate money.Decimal
	OrderDate    time.Time
	DeliveryDate *time.Time
	Remark       string
	Items        []QuotationOrderLine
}

// QuotationOrderLine 是转换过来的一行：产品、数量、单价都是报价单上的原值。
type QuotationOrderLine struct {
	ProductID uint
	Qty       float64
	UnitPrice money.Decimal
	Remark    string
}

type salesOrderService struct {
	orders     repository.SalesOrderRepository
	partners   repository.PartnerRepository
	products   repository.ProductRepository
	shipments  repository.ShipmentRepository
	currencies CurrencyService
	docNos     DocNoService
	// credits 在确认时校验客户额度（T-39）。草稿不占额度，所以 Create 不调它。
	credits CreditService
	clock   func() time.Time
	log     *slog.Logger
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
	shipments repository.ShipmentRepository,
	currencies CurrencyService,
	docNos DocNoService,
	credits CreditService,
	opts ...SalesOrderOption,
) SalesOrderService {
	s := &salesOrderService{
		orders:     orders,
		partners:   partners,
		products:   products,
		shipments:  shipments,
		currencies: currencies,
		docNos:     docNos,
		credits:    credits,
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
	partner, err := loadCustomer(ctx, s.partners, in.PartnerID)
	if err != nil {
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
	// 手建订单就算一笔新生意——它没有报价单可以继承案号。
	deal, err := s.docNos.NextDeal(ctx, DealKey{
		PartnerID:   partner.ID,
		PartnerCode: partner.Code,
		Year:        s.clock().Year(),
	}, model.DocTypeSalesOrder)
	if err != nil {
		return nil, err
	}

	o := &model.SalesOrder{
		DocNo:           deal.DocNo,
		BaseNo:          deal.BaseNo,
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
		"partner_id", o.PartnerID, "amount_base", o.AmountBaseTotal.String())
	return s.Get(ctx, o.ID)
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
	return s.Get(ctx, id)
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

// Get 在详情里补上每行的已出 / 未出数量。
//
// 这两个数只能由出货分配算出来，订单行上不存：一旦存了，出货单作废、
// 改数量、软删除任一动作都要记得回写，漏一次就永久对不上。
// 列表接口不填——一页 20 张单就是 20 次聚合，而列表并不显示这个数。
//
// 新建、编辑、改状态也都从这里返回：只填详情、别处返回零值的话，
// 刚建的订单一出接口就显示「未出 0」，而实际还能出 100。
func (s *salesOrderService) Get(ctx context.Context, id uint) (*model.SalesOrder, error) {
	o, err := s.orders.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	shipped, err := s.shipments.ShippedQtyByOrder(ctx, id)
	if err != nil {
		return nil, err
	}
	for i := range o.Items {
		qty := shipped[o.Items[i].ID]
		o.Items[i].ShippedQty = qty
		o.Items[i].UnshippedQty = money.Round(o.Items[i].Qty-qty, 3)
	}
	return o, nil
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
	// 只有"确认"这一步占额度：草稿可改可删，先放行；确认之后这单就是对客户的承诺，
	// 从这一刻起它和它的应收一起算进敞口，所以校验也放在这一刻。
	if next == model.DocumentStatusConfirmed {
		if err := s.credits.CheckConfirm(ctx, CreditCheck{
			PartnerID: existing.PartnerID,
			DocNo:     existing.DocNo,
			AmountBase: existing.AmountBaseTotal,
			Force:      in.ForceCredit,
		}); err != nil {
			return nil, err
		}
	}
	if err := s.orders.Update(ctx, id, map[string]any{"status": next}); err != nil {
		return nil, err
	}
	s.log.Info("销售订单状态变更", "id", id, "from", existing.Status, "to", next)
	return s.Get(ctx, id)
}

func (s *salesOrderService) CreateFromQuotation(
	ctx context.Context, in QuotationOrderInput,
) (*model.SalesOrder, error) {
	if in.ExchangeRate.Cmp(money.Zero()) <= 0 {
		return nil, errs.BadRequest("报价单缺少有效汇率，无法转订单")
	}
	baseCode, err := s.currencies.BaseCode(ctx)
	if err != nil {
		return nil, err
	}
	curDP, err := s.currencies.Decimals(ctx, in.CurrencyCode)
	if err != nil {
		return nil, err
	}
	baseDP, err := s.currencies.Decimals(ctx, baseCode)
	if err != nil {
		return nil, err
	}

	terms := salesOrderTerms{
		TradeTerm:        in.TradeTerm,
		CurrencyCode:     in.CurrencyCode,
		ExchangeRate:     in.ExchangeRate,
		CurrencyDecimals: curDP,
		BaseDecimals:     baseDP,
		OrderDate:        in.OrderDate,
		DeliveryDate:     in.DeliveryDate,
		Remark:           in.Remark,
	}
	rows := make([]dto.SalesOrderItemInput, 0, len(in.Items))
	for _, l := range in.Items {
		rows = append(rows, dto.SalesOrderItemInput{
			ProductID: l.ProductID,
			Qty:       l.Qty,
			UnitPrice: l.UnitPrice,
			Remark:    l.Remark,
		})
	}
	// 金额仍走 buildItems：转单的金额与手建单的金额必须是同一套算法，
	// 否则同一批货按两条路径进来会得到两个本币总额。
	items, totals, err := s.buildItems(ctx, rows, terms)
	if err != nil {
		return nil, err
	}

	baseNo, dealSeq := in.BaseNo, in.DealSeq
	if baseNo == "" || dealSeq == 0 {
		// 存量报价单没有案号（T-47 之前生成的）。不给它回填单号：那个号早就发给
		// 客户了，改号等于换了身份。这张订单单独取一笔新生意，账面上会多一个案号，
		// 但比"两个客户看到两个不同的报价单号"要好。
		partner, err := s.partners.FindByID(ctx, in.PartnerID)
		if err != nil {
			return nil, err
		}
		deal, err := s.docNos.NextDeal(ctx, DealKey{
			PartnerID:   partner.ID,
			PartnerCode: partner.Code,
			Year:        s.clock().Year(),
		}, model.DocTypeSalesOrder)
		if err != nil {
			return nil, err
		}
		baseNo, dealSeq = deal.BaseNo, deal.Seq
	}

	o := &model.SalesOrder{
		DocNo:           model.FormatDealDocNo(baseNo, model.DocTypeSalesOrder, dealSeq),
		BaseNo:          baseNo,
		PartnerID:       in.PartnerID,
		TradeTerm:       in.TradeTerm,
		Status:          model.DocumentStatusDraft,
		OrderDate:       in.OrderDate,
		DeliveryDate:    in.DeliveryDate,
		CurrencyCode:    in.CurrencyCode,
		ExchangeRate:    in.ExchangeRate,
		AmountTotal:     totals.Amount,
		AmountBaseTotal: totals.AmountBase,
		Remark:          in.Remark,
	}
	if err := s.orders.CreateWithItems(ctx, o, items); err != nil {
		return nil, err
	}
	s.log.Info("由报价单生成销售订单", "id", o.ID, "doc_no", o.DocNo,
		"partner_id", o.PartnerID, "amount_base", o.AmountBaseTotal.String())
	return s.Get(ctx, o.ID)
}

// salesOrderTerms 是校验并归一化之后的表头字段。
type salesOrderTerms struct {
	TradeTerm        model.TradeTerm
	CurrencyCode     string
	ExchangeRate     money.Decimal
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

		// 数量仍是 float64（qty 不在 T-37 范围），乘之前转一次定点，其后全在定点里算。
		qty := money.FromFloat(row.Qty)
		amount := row.UnitPrice.Mul(qty).Round(t.CurrencyDecimals)
		amountBase := amount.Mul(t.ExchangeRate).Round(t.BaseDecimals)

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

		totals.Amount = totals.Amount.Add(amount)
		totals.AmountBase = totals.AmountBase.Add(amountBase)
	}

	totals.Amount = totals.Amount.Round(t.CurrencyDecimals)
	totals.AmountBase = totals.AmountBase.Round(t.BaseDecimals)
	return items, totals, nil
}

// salesOrderTotals 是订单合计。
type salesOrderTotals struct {
	Amount     money.Decimal
	AmountBase money.Decimal
}
