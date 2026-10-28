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

// QuotationService 管报价单。
//
// 三件事必须由它算，前端一律不算：金额（数量 × 单价 × 单据汇率）、
// 成本（从货源取本币价并快照）、毛利（本币口径的报价额 − 成本）。
// 交给浏览器算等于把毛利口径交给用户，而"这单赚没赚"是要拿去决策的。
type QuotationService interface {
	Create(ctx context.Context, in dto.QuotationInput) (*dto.QuotationDetail, error)
	Update(ctx context.Context, id uint, in dto.QuotationInput) (*dto.QuotationDetail, error)
	Delete(ctx context.Context, id uint) error
	Get(ctx context.Context, id uint) (*dto.QuotationDetail, error)
	List(ctx context.Context, q dto.QuotationQuery) (*dto.PageResult[model.Quotation], error)
	UpdateStatus(ctx context.Context, id uint, in dto.QuotationStatusInput) (*dto.QuotationDetail, error)
	NewVersion(ctx context.Context, id uint, in dto.QuotationVersionInput) (*dto.QuotationDetail, error)
	Preview(ctx context.Context, in dto.QuotationPreviewInput) (*dto.QuotationPreview, error)
	// Convert 把已确认且未过期的报价单转成销售订单草稿，并回填订单引用。
	// 一张报价单只能转一次（并发下靠条件更新保证）。
	Convert(
		ctx context.Context, id uint, in dto.QuotationConvertInput,
	) (*dto.QuotationDetail, error)
}

type quotationService struct {
	quotations repository.QuotationRepository
	partners   repository.PartnerRepository
	products   repository.ProductRepository
	sources    repository.ProductSourceRepository
	currencies CurrencyService
	docNos     DocNoService
	// orders 只用于转订单（T-13）：建单逻辑留在销售订单服务里，
	// 报价这边只负责判定能不能转、转完之后回填引用。
	orders SalesOrderService
	clock  func() time.Time
	log    *slog.Logger
}

type QuotationOption func(*quotationService)

func WithQuotationLogger(l *slog.Logger) QuotationOption {
	return func(s *quotationService) { s.log = l }
}

// WithQuotationClock 注入时钟：单号年月、有效期判定、汇率取哪一天都要可断言。
func WithQuotationClock(now func() time.Time) QuotationOption {
	return func(s *quotationService) { s.clock = now }
}

func NewQuotationService(
	quotations repository.QuotationRepository,
	partners repository.PartnerRepository,
	products repository.ProductRepository,
	sources repository.ProductSourceRepository,
	currencies CurrencyService,
	docNos DocNoService,
	orders SalesOrderService,
	opts ...QuotationOption,
) QuotationService {
	s := &quotationService{
		quotations: quotations,
		partners:   partners,
		products:   products,
		sources:    sources,
		currencies: currencies,
		docNos:     docNos,
		orders:     orders,
		clock:      time.Now,
		log:        slog.Default(),
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *quotationService) Create(
	ctx context.Context, in dto.QuotationInput,
) (*dto.QuotationDetail, error) {
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

	// 取号排在最后：前面任何一步失败都不该消耗掉一个单号。
	// 单号有缺口本身无害，但缺口会让人怀疑"是不是丢了一张单"。
	deal, err := s.docNos.NextDeal(ctx, DealKey{
		PartnerID:   partner.ID,
		PartnerCode: partner.Code,
		Year:        s.clock().Year(),
	}, model.DocTypeQuotation)
	if err != nil {
		return nil, err
	}

	q := &model.Quotation{
		DocNo:           deal.DocNo,
		BaseNo:          deal.BaseNo,
		Version:         1,
		PartnerID:       in.PartnerID,
		TradeTerm:       terms.TradeTerm,
		Status:          model.DocumentStatusDraft,
		CurrencyCode:    terms.CurrencyCode,
		ExchangeRate:    terms.ExchangeRate,
		ValidUntil:      terms.ValidUntil,
		AmountTotal:     totals.Amount,
		AmountBaseTotal: totals.AmountBase,
		CostTotal:       totals.Cost,
		GrossProfit:     totals.Gross,
		MarginRate:      totals.Margin,
		Remark:          terms.Remark,
	}
	if err := s.quotations.CreateWithItems(ctx, q, items); err != nil {
		return nil, err
	}

	s.log.Info("新增报价单", "id", q.ID, "doc_no", q.DocNo,
		"partner_id", q.PartnerID, "amount_base", q.AmountBaseTotal, "margin", q.MarginRate)
	return s.Get(ctx, q.ID)
}

func (s *quotationService) Update(
	ctx context.Context, id uint, in dto.QuotationInput,
) (*dto.QuotationDetail, error) {
	existing, err := s.quotations.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if !existing.IsEditable() {
		return nil, errs.ErrConflict.WithMsg(
			"只有草稿可以修改；已确认的报价单是发给客户的承诺，请另存新版本")
	}
	// 换客户等于另一笔生意：旧单的成本、条款、有效期都不再适用，
	// 而且已经发出去的 PDF 还指向原客户。
	if in.PartnerID != 0 && in.PartnerID != existing.PartnerID {
		return nil, errs.BadRequest("报价单不能改客户，请新建一张")
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
		"currency_code":     terms.CurrencyCode,
		"exchange_rate":     terms.ExchangeRate,
		"valid_until":       terms.ValidUntil,
		"remark":            terms.Remark,
		"amount_total":      totals.Amount,
		"amount_base_total": totals.AmountBase,
		"cost_total":        totals.Cost,
		"gross_profit":      totals.Gross,
		"margin_rate":       totals.Margin,
	}
	if err := s.quotations.UpdateWithItems(ctx, id, fields, items); err != nil {
		return nil, err
	}
	return s.Get(ctx, id)
}

func (s *quotationService) Delete(ctx context.Context, id uint) error {
	existing, err := s.quotations.FindByID(ctx, id)
	if err != nil {
		return err
	}
	if !existing.IsEditable() {
		// 已确认的报价单留在库里供追溯；不要了走"取消"，而不是抹掉。
		return errs.ErrConflict.WithMsg("只有草稿可以删除；已确认的报价单请改为已取消")
	}
	return s.quotations.Delete(ctx, id)
}

func (s *quotationService) Get(ctx context.Context, id uint) (*dto.QuotationDetail, error) {
	m, err := s.quotations.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return s.detail(m), nil
}

func (s *quotationService) List(
	ctx context.Context, q dto.QuotationQuery,
) (*dto.PageResult[model.Quotation], error) {
	q.PageQuery.Normalize()

	filter := repository.QuotationFilter{Keyword: q.Keyword, PartnerID: q.PartnerID}
	if q.Status != nil {
		st := model.DocumentStatus(*q.Status)
		if !st.IsValid() {
			return nil, errs.BadRequest("status 不合法")
		}
		filter.Status = &st
	}
	page := repository.Pagination{Page: q.Page, PageSize: q.PageSize}

	list, total, err := s.quotations.List(ctx, filter, page)
	if err != nil {
		return nil, err
	}
	return dto.NewPageResult(list, total, q.Page, q.PageSize), nil
}

// UpdateStatus 只放行「确认」与「取消」。
//
// 报价单不是订单，不该出现备货中 / 已出货；不拦住的话，
// DocumentStatus 里那些阶段迟早会被前端下拉框误用。
func (s *quotationService) UpdateStatus(
	ctx context.Context, id uint, in dto.QuotationStatusInput,
) (*dto.QuotationDetail, error) {
	existing, err := s.quotations.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	next := model.DocumentStatus(in.Status)
	switch next {
	case model.DocumentStatusConfirmed, model.DocumentStatusCancelled:
	default:
		return nil, errs.BadRequest("报价单只能转为「已确认」或「已取消」")
	}
	if err := checkDocumentTransit(existing.Status, next); err != nil {
		return nil, err
	}
	// 过期报价确认不了：确认即承诺，而有效期已过意味着这个价格不再成立。
	if next == model.DocumentStatusConfirmed && existing.IsExpired(s.clock()) {
		return nil, errs.ErrConflict.WithMsg(
			"报价单已过有效期，请另存新版本并更新有效期后再确认")
	}
	// 已转成订单的报价单不能作废：订单已经在跑，报价单再变成"已取消"，
	// 事后就没法解释这张订单是凭什么来的。要停就停订单。
	if next == model.DocumentStatusCancelled && existing.SalesOrderID != nil {
		return nil, errs.ErrConflict.WithMsg(
			"报价单已转为订单 " + existing.SalesOrderDocNo + "，请改为取消该订单")
	}
	if err := s.quotations.Update(ctx, id, map[string]any{"status": next}); err != nil {
		return nil, err
	}
	s.log.Info("报价单状态变更", "id", id, "from", existing.Status, "to", next)
	return s.Get(ctx, id)
}

// NewVersion 把当前版本另存为下一版草稿。
//
// 单号不变、版本加一：客户与供应商邮件里说的是"QT-202610-0001 的第 3 版"，
// 换单号会让对方以为是一张新的报价，之前的谈判记录就对不上了。
// 明细照抄，成本快照也照抄——第一版保存时的成本就是"上一版报出去的依据"，
// 下次编辑保存时会按当时的货源重新取价。
func (s *quotationService) NewVersion(
	ctx context.Context, id uint, in dto.QuotationVersionInput,
) (*dto.QuotationDetail, error) {
	existing, err := s.quotations.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}

	validUntil := existing.ValidUntil
	if strings.TrimSpace(in.ValidUntil) != "" {
		validUntil, err = s.parseValidUntil(in.ValidUntil)
		if err != nil {
			return nil, err
		}
	} else if existing.IsExpired(s.clock()) {
		return nil, errs.BadRequest("该报价单已过有效期，另存新版本时请填写新的有效期")
	}

	rootID := existing.RootID
	if rootID == 0 {
		rootID = existing.ID
	}
	maxVer, err := s.quotations.MaxVersionByDocNo(ctx, existing.DocNo)
	if err != nil {
		return nil, err
	}

	items := make([]model.QuotationItem, 0, len(existing.Items))
	for _, it := range existing.Items {
		items = append(items, model.QuotationItem{
			ProductID:  it.ProductID,
			SKUCode:    it.SKUCode,
			NameCN:     it.NameCN,
			Spec:       it.Spec,
			Unit:       it.Unit,
			Qty:        it.Qty,
			UnitPrice:  it.UnitPrice,
			Amount:     it.Amount,
			AmountBase: it.AmountBase,
			CostPrice:  it.CostPrice,
			CostAmount: it.CostAmount,
			SourceID:   it.SourceID,
			Remark:     it.Remark,
		})
	}

	next := &model.Quotation{
		DocNo:           existing.DocNo,
		Version:         maxVer + 1,
		RootID:          rootID,
		PartnerID:       existing.PartnerID,
		TradeTerm:       existing.TradeTerm,
		Status:          model.DocumentStatusDraft,
		CurrencyCode:    existing.CurrencyCode,
		ExchangeRate:    existing.ExchangeRate,
		ValidUntil:      validUntil,
		AmountTotal:     existing.AmountTotal,
		AmountBaseTotal: existing.AmountBaseTotal,
		CostTotal:       existing.CostTotal,
		GrossProfit:     existing.GrossProfit,
		MarginRate:      existing.MarginRate,
		Remark:          existing.Remark,
	}
	if err := s.quotations.CreateWithItems(ctx, next, items); err != nil {
		return nil, err
	}
	s.log.Info("报价单另存新版本", "from_id", existing.ID, "id", next.ID,
		"doc_no", next.DocNo, "version", next.Version)
	return s.Get(ctx, next.ID)
}

// Convert 把报价单转成销售订单草稿。
//
// 生成的订单是**草稿**而不是已确认：转单是内部动作，客户确认的是报价，
// 订单要不要确认、什么时候确认由业务决定；自动确认会让"已确认订单"这个状态失去意义。
//
// 幂等靠两点：转换前的 `SalesOrderID == nil` 检查挡住顺序重复点击；
// 回填走条件更新（WHERE sales_order_id IS NULL）挡住并发双击——
// 后者抢不到时，把刚建的那张订单删掉再报 409，避免留下一张没人认领的草稿单。
func (s *quotationService) Convert(
	ctx context.Context, id uint, in dto.QuotationConvertInput,
) (*dto.QuotationDetail, error) {
	q, err := s.quotations.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if q.SalesOrderID != nil {
		return nil, errs.ErrConflict.WithMsg("该报价单已转为订单 " + q.SalesOrderDocNo)
	}
	if block := q.ConvertBlocker(s.clock()); block != "" {
		return nil, errs.ErrConflict.WithMsg(block)
	}

	orderDate := model.DateOnly(s.clock())
	if strings.TrimSpace(in.OrderDate) != "" {
		orderDate, err = parseDateOnly(in.OrderDate)
		if err != nil {
			return nil, err
		}
	}
	var deliveryDate *time.Time
	if strings.TrimSpace(in.DeliveryDate) != "" {
		d, err := parseDateOnly(in.DeliveryDate)
		if err != nil {
			return nil, err
		}
		if d.Before(orderDate) {
			return nil, errs.BadRequest("交期不能早于下单日")
		}
		deliveryDate = &d
	}

	lines := make([]QuotationOrderLine, 0, len(q.Items))
	for _, it := range q.Items {
		lines = append(lines, QuotationOrderLine{
			ProductID: it.ProductID,
			Qty:       it.Qty,
			UnitPrice: it.UnitPrice,
			Remark:    it.Remark,
		})
	}

	// 订单沿用报价的案号与序号：QT03 转出必得 SO03。
	// 序号从单号里解析，不另存一列——两处存同一个数字，迟早会有一处没跟上。
	var dealSeq uint
	if _, seq, ok := model.ParseDealDocNo(q.DocNo, model.DocTypeQuotation); ok {
		dealSeq = seq
	}
	order, err := s.orders.CreateFromQuotation(ctx, QuotationOrderInput{
		PartnerID:    q.PartnerID,
		BaseNo:       q.BaseNo,
		DealSeq:      dealSeq,
		TradeTerm:    q.TradeTerm,
		CurrencyCode: q.CurrencyCode,
		ExchangeRate: q.ExchangeRate,
		OrderDate:    orderDate,
		DeliveryDate: deliveryDate,
		Remark:       fmt.Sprintf("由报价单 %s v%d 转入", q.DocNo, q.Version),
		Items:        lines,
	})
	if err != nil {
		return nil, err
	}

	claimed, err := s.quotations.MarkConverted(ctx, q.ID, order.ID, order.DocNo)
	if err != nil || !claimed {
		// 并发里输的那一方：刚建的订单没人认领，删掉再报冲突。
		if delErr := s.orders.Delete(ctx, order.ID); delErr != nil {
			s.log.Error("撤销并发转单产生的订单失败", "order_id", order.ID, "err", delErr)
		}
		if err != nil {
			return nil, err
		}
		return nil, errs.ErrConflict.WithMsg("该报价单已被转为订单，请刷新后查看")
	}

	s.log.Info("报价单转订单", "quotation_id", q.ID, "doc_no", q.DocNo,
		"order_id", order.ID, "order_doc_no", order.DocNo)
	return s.Get(ctx, q.ID)
}

// Preview 算一遍但不落库：报价前先看毛利，别等发出去才发现亏了。
// 与保存路径共用 buildItems，口径不可能分叉。
func (s *quotationService) Preview(
	ctx context.Context, in dto.QuotationPreviewInput,
) (*dto.QuotationPreview, error) {
	code, rate, curDP, baseDP, err := s.buildCurrency(ctx, in.CurrencyCode)
	if err != nil {
		return nil, err
	}
	terms := quotationTerms{
		CurrencyCode: code, ExchangeRate: rate,
		CurrencyDecimals: curDP, BaseDecimals: baseDP,
	}
	items, totals, err := s.buildItems(ctx, in.Items, terms)
	if err != nil {
		return nil, err
	}

	out := &dto.QuotationPreview{
		CurrencyCode:    code,
		ExchangeRate:    rate,
		Items:           make([]dto.QuotationPreviewItem, 0, len(items)),
		AmountTotal:     totals.Amount,
		AmountBaseTotal: totals.AmountBase,
		CostTotal:       totals.Cost,
		GrossProfit:     totals.Gross,
		MarginRate:      totals.Margin,
	}
	for _, it := range items {
		name := ""
		if it.SourceID != nil {
			if src, err := s.sources.FindByID(ctx, *it.SourceID); err == nil && src.Partner != nil {
				name = src.Partner.Name
			}
		}
		out.Items = append(out.Items, dto.QuotationPreviewItem{
			ProductID:   it.ProductID,
			SKUCode:     it.SKUCode,
			NameCN:      it.NameCN,
			Unit:        it.Unit,
			Qty:         it.Qty,
			UnitPrice:   it.UnitPrice,
			Amount:      it.Amount,
			AmountBase:  it.AmountBase,
			CostPrice:   it.CostPrice,
			CostAmount:  it.CostAmount,
			GrossProfit: it.GrossProfit(),
			SourceID:    it.SourceID,
			SourceName:  name,
		})
	}
	return out, nil
}

func (s *quotationService) detail(m *model.Quotation) *dto.QuotationDetail {
	now := s.clock()
	return &dto.QuotationDetail{
		Quotation:    m,
		Expired:      m.IsExpired(now),
		ConvertBlock: m.ConvertBlocker(now),
	}
}

// quotationTerms 是校验并归一化之后的表头字段。
type quotationTerms struct {
	TradeTerm        model.TradeTerm
	CurrencyCode     string
	ExchangeRate     float64
	CurrencyDecimals int
	BaseDecimals     int
	ValidUntil       time.Time
	Remark           string
}

// quotationTotals 是一整单的锁定合计。
type quotationTotals struct {
	Amount     float64
	AmountBase float64
	Cost       float64
	Gross      float64
	Margin     float64
}

func (s *quotationService) buildTerms(
	ctx context.Context, in dto.QuotationInput,
) (quotationTerms, error) {
	var out quotationTerms

	term := model.TradeTerm(strings.ToUpper(strings.TrimSpace(in.TradeTerm)))
	if !term.IsValid() {
		return out, errInvalidTradeTerm
	}
	validUntil, err := s.parseValidUntil(in.ValidUntil)
	if err != nil {
		return out, err
	}

	code, rate, curDP, baseDP, err := s.buildCurrency(ctx, in.CurrencyCode)
	if err != nil {
		return out, err
	}

	out = quotationTerms{
		TradeTerm:        term,
		CurrencyCode:     code,
		ExchangeRate:     rate,
		CurrencyDecimals: curDP,
		BaseDecimals:     baseDP,
		ValidUntil:       validUntil,
		Remark:           in.Remark,
	}
	return out, nil
}

// buildCurrency 定下报价币种、单据汇率与两侧的小数位。
func (s *quotationService) buildCurrency(
	ctx context.Context, raw string,
) (code string, rate float64, curDP, baseDP int, err error) {
	return resolveCurrency(ctx, s.currencies, raw, s.clock())
}

func (s *quotationService) parseValidUntil(raw string) (time.Time, error) {
	d, err := parseDateOnly(raw)
	if err != nil {
		return time.Time{}, err
	}
	if d.Before(model.DateOnly(s.clock())) {
		return time.Time{}, errs.BadRequest("有效期不能早于今天")
	}
	return d, nil
}

// buildItems 逐行取价、取成本并算金额，同时汇总整单合计。
//
// 成本取不到时直接拒绝，不给"手填成本"的余地：手填的成本价就是毛利失真的源头，
// 而"这个产品没有货源"是一句能被当场修好的提示。
func (s *quotationService) buildItems(
	ctx context.Context, in []dto.QuotationItemInput, t quotationTerms,
) ([]model.QuotationItem, quotationTotals, error) {
	var totals quotationTotals
	items := make([]model.QuotationItem, 0, len(in))

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
		src, err := s.pickSource(ctx, product, row.SourceID)
		if err != nil {
			return nil, totals, err
		}

		amount := money.Round(row.UnitPrice*row.Qty, t.CurrencyDecimals)
		amountBase := money.Round(amount*t.ExchangeRate, t.BaseDecimals)
		costAmount := money.Round(src.PurchasePriceBase*row.Qty, t.BaseDecimals)

		items = append(items, model.QuotationItem{
			ProductID:  product.ID,
			SKUCode:    product.SKUCode,
			NameCN:     product.NameCN,
			Spec:       product.Spec,
			Unit:       product.Unit,
			Qty:        row.Qty,
			UnitPrice:  row.UnitPrice,
			Amount:     amount,
			AmountBase: amountBase,
			CostPrice:  src.PurchasePriceBase,
			CostAmount: costAmount,
			SourceID:   &src.ID,
			Remark:     row.Remark,
		})

		totals.Amount += amount
		totals.AmountBase += amountBase
		totals.Cost += costAmount
	}

	totals.Amount = money.Round(totals.Amount, t.CurrencyDecimals)
	totals.AmountBase = money.Round(totals.AmountBase, t.BaseDecimals)
	totals.Cost = money.Round(totals.Cost, t.BaseDecimals)
	totals.Gross = money.Round(totals.AmountBase-totals.Cost, t.BaseDecimals)
	if totals.AmountBase != 0 {
		totals.Margin = money.Round(totals.Gross/totals.AmountBase, 6)
	}
	return items, totals, nil
}

// pickSource 定下这一行的成本来源：指定了就校验归属，没指定就取本币价最低的启用货源。
func (s *quotationService) pickSource(
	ctx context.Context, product *model.Product, sourceID *uint,
) (*model.ProductSource, error) {
	if sourceID != nil && *sourceID > 0 {
		src, err := s.sources.FindByID(ctx, *sourceID)
		if err != nil {
			if errors.Is(err, errs.ErrNotFound) {
				return nil, errs.BadRequest("指定的货源不存在")
			}
			return nil, err
		}
		if src.ProductID != product.ID {
			return nil, errs.BadRequest("指定的货源不属于这个产品：" + product.SKUCode)
		}
		if src.Status != model.EnableStatusOn {
			return nil, errs.BadRequest("指定的货源已停用，不能用于报价成本")
		}
		return src, nil
	}

	src, err := s.sources.FindCheapestByProductID(ctx, product.ID)
	if err != nil {
		if errors.Is(err, errs.ErrNotFound) {
			return nil, errs.BadRequest(fmt.Sprintf(
				"产品 %s 没有启用的货源，请先在产品详情页维护货源再报价", product.SKUCode))
		}
		return nil, err
	}
	return src, nil
}

// ensureCustomerRole 确认主体存在且具备客户角色。
// 报价对象是客户：报给供应商等于把成本单发给对方，没有业务含义。
func ensureCustomerRole(
	ctx context.Context, partners repository.PartnerRepository, id uint,
) error {
	_, err := loadCustomer(ctx, partners, id)
	return err
}

// loadCustomer 取主体并确认它有客户角色。
//
// 返回主体本身而不只是 error：发案号要用它的编码拼 `{编码}{YY}{NN}`，
// 再查一次等于同一次请求里把同一行读两遍。
func loadCustomer(
	ctx context.Context, partners repository.PartnerRepository, id uint,
) (*model.Partner, error) {
	p, err := partners.FindByID(ctx, id)
	if err != nil {
		if errors.Is(err, errs.ErrNotFound) {
			return nil, errs.BadRequest("往来单位不存在")
		}
		return nil, err
	}
	if !p.Role.Has(model.PartnerRoleCustomer) {
		return nil, errs.BadRequest(fmt.Sprintf("所选主体不具备客户角色：%s", p.Name))
	}
	return p, nil
}
