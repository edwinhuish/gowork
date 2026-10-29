package service

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/errs"
	"github.com/edwinhuish/gowork/internal/pkg/money"
	"github.com/edwinhuish/gowork/internal/pkg/pdf"
	"github.com/edwinhuish/gowork/internal/repository"
)

// RenderedFile 是可下载的文件：文件名由后端定，前端只管存。
//
// 文件名带单号而不是用业务标题：客户收一堆 "quotation.pdf" 之后，
// 找"那张 QT-202610-0001" 只能逐个打开看。
type RenderedFile struct {
	Filename string
	Data     []byte
}

// PDFService 把单据渲染成 PDF。
//
// 数据装配放在 service 而不是 handler：一份 PDF 要取表头、明细、付款分期
// 与来源单据，handler 拿到这些就叫 service 了。
type PDFService interface {
	Quotation(ctx context.Context, id uint) (*RenderedFile, error)
	Contract(ctx context.Context, id uint) (*RenderedFile, error)
	// Invoice 出商业发票（CI），PackingList 出装箱单（PL）。
	//
	// 两者与送货单共用套号（送货单号），不单独发号：报关、提单与客户账上认的是同一个号，
	// 一套资料三个文件只在文件名后缀上区分（-CI / -PL）。
	Invoice(ctx context.Context, id uint) (*RenderedFile, error)
	PackingList(ctx context.Context, id uint) (*RenderedFile, error)
	// CartonLabels 出箱唛（shipping mark）：一箱一张，主标识是销售订单号。
	CartonLabels(ctx context.Context, id uint) (*RenderedFile, error)
}

// CompanyInfo 是打印抬头，来自配置。
type CompanyInfo struct {
	Name    string
	Address string
	Contact string
}

type pdfService struct {
	quotations repository.QuotationRepository
	orders     repository.SalesOrderRepository
	contracts  repository.ContractRepository
	// shipments 供商业发票与装箱单用：这两份文件印的是"这一票出了什么"，
	// 明细只在分配行上，不回订单取。
	shipments  repository.ShipmentRepository
	currencies CurrencyService
	renderer   *pdf.Renderer
	company    CompanyInfo
	log        *slog.Logger
}

type PDFOption func(*pdfService)

func WithPDFLogger(l *slog.Logger) PDFOption {
	return func(s *pdfService) { s.log = l }
}

func NewPDFService(
	quotations repository.QuotationRepository,
	orders repository.SalesOrderRepository,
	contracts repository.ContractRepository,
	shipments repository.ShipmentRepository,
	currencies CurrencyService,
	renderer *pdf.Renderer,
	company CompanyInfo,
	opts ...PDFOption,
) PDFService {
	s := &pdfService{
		quotations: quotations,
		orders:     orders,
		contracts:  contracts,
		shipments:  shipments,
		currencies: currencies,
		renderer:   renderer,
		company:    company,
		log:        slog.Default(),
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *pdfService) Quotation(ctx context.Context, id uint) (*RenderedFile, error) {
	q, err := s.quotations.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}

	curDP, baseDP, baseCode, err := s.decimals(ctx, q.CurrencyCode)
	if err != nil {
		return nil, err
	}

	items := make([]map[string]string, 0, len(q.Items))
	for i, it := range q.Items {
		items = append(items, map[string]string{
			"no":          strconv.Itoa(i + 1),
			"description": itemDescription(it.SKUCode, it.NameCN, it.Spec),
			"qty":         qtyText(it.Qty),
			"unit":        it.Unit,
			"unit_price":  moneyTextD(it.UnitPrice, curDP),
			"amount":      moneyTextD(it.Amount, curDP),
		})
	}

	values := s.baseValues(baseCode)
	values["doc_no"] = q.DocNo
	values["date"] = q.CreatedAt.Format("2006-01-02")
	values["valid_until"] = q.ValidUntil.Format("2006-01-02")
	values["trade_term"] = string(q.TradeTerm)
	values["version"] = strconv.Itoa(q.Version)
	values["currency_code"] = q.CurrencyCode
	values["exchange_rate"] = rateTextD(q.ExchangeRate)
	values["amount_total"] = moneyTextD(q.AmountTotal, curDP)
	values["amount_base_total"] = moneyTextD(q.AmountBaseTotal, baseDP)
	values["remark"] = orDash(q.Remark)
	if q.Partner != nil {
		// 结算方式是快照式的当下值：报价单没有存一份，改客户资料后重打的 PDF
		// 会跟着变。这一点与金额不同——金额是承诺，结算方式是联系方式。
		values["settlement_method"] = orDash(string(q.Partner.SettlementMethod))
		s.partnerValues(values, q.Partner)
	} else {
		values["settlement_method"] = "—"
	}

	data, err := s.render("quotation", pdf.Data{Values: values, Items: items})
	if err != nil {
		return nil, err
	}
	s.log.Info("生成报价单 PDF", "id", q.ID, "doc_no", q.DocNo, "bytes", len(data))
	return &RenderedFile{Filename: q.DocNo + ".pdf", Data: data}, nil
}

func (s *pdfService) Contract(ctx context.Context, id uint) (*RenderedFile, error) {
	c, err := s.contracts.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}

	curDP, baseDP, baseCode, err := s.decimals(ctx, c.CurrencyCode)
	if err != nil {
		return nil, err
	}

	// 合同不存货物明细：明细回来源单据取。
	// 来源必须是已确认且未取消的单据才能生成合同，而确认后不可改，
	// 所以这里拿到的与生成合同时看到的是同一份。
	sourceRows, sourceLabel, tradeTerm, err := s.sourceItems(ctx, c, curDP)
	if err != nil {
		return nil, err
	}

	terms := make([]map[string]string, 0, len(c.Terms))
	termsTotal := money.Zero()
	for i, t := range c.Terms {
		terms = append(terms, map[string]string{
			"no":       strconv.Itoa(i + 1),
			"name":     t.Name,
			"due_date": dateText(t.DueDate),
			"trigger":  orDash(t.Trigger),
			"amount":   moneyTextD(t.Amount, curDP),
		})
		termsTotal = termsTotal.Add(t.Amount)
	}

	values := s.baseValues(baseCode)
	values["doc_title"] = contractTitle(c.Type)
	values["doc_subtitle"] = c.Type.Label()
	values["doc_no"] = c.DocNo
	values["sign_date"] = c.SignDate.Format("2006-01-02")
	values["delivery_date"] = dateText(c.DeliveryDate)
	values["type_label"] = c.Type.Label()
	values["source_label"] = sourceLabel
	values["source_doc_no"] = c.SourceDocNo
	// 贸易术语与货物明细一样回来源单据取：合同的表结构里没有这一项，
	// 而 PI 上缺了 FOB / CIF 就没法判断谁付运费、风险在哪一刻转移。
	values["trade_term"] = orDash(tradeTerm)
	values["currency_code"] = c.CurrencyCode
	values["exchange_rate"] = rateTextD(c.ExchangeRate)
	values["amount_total"] = moneyTextD(c.AmountTotal, curDP)
	values["amount_base_total"] = moneyTextD(c.AmountBaseTotal, baseDP)
	values["terms_total"] = moneyTextD(termsTotal, curDP)
	values["remark"] = orDash(c.Remark)
	s.partnerValues(values, c.Partner)

	data, err := s.render("contract", pdf.Data{
		Values: values,
		Items:  sourceRows,
		Tables: map[string][]map[string]string{"terms": terms},
	})
	if err != nil {
		return nil, err
	}
	s.log.Info("生成合同 PDF", "id", c.ID, "doc_no", c.DocNo, "bytes", len(data))
	return &RenderedFile{Filename: c.DocNo + ".pdf", Data: data}, nil
}

// Invoice 出商业发票。
//
// 金额一律按**出货数量**算，不抄订单行的 Amount：分批出货时订单行是一整批的量，
// 抄过来就等于把没发走的货也开在发票上。单价沿用订单，发票与订单必须对得上。
func (s *pdfService) Invoice(ctx context.Context, id uint) (*RenderedFile, error) {
	sd, order, err := s.shipmentSource(ctx, id)
	if err != nil {
		return nil, err
	}

	curDP, baseDP, baseCode, err := s.decimals(ctx, order.CurrencyCode)
	if err != nil {
		return nil, err
	}

	items := make([]map[string]string, 0, len(sd.Allocations))
	orderNos := make([]string, 0, len(sd.Allocations))
	total := money.Zero()
	for i, a := range sd.Allocations {
		price := money.Zero()
		if a.SalesOrderItem != nil {
			price = a.SalesOrderItem.UnitPrice
		}
		amount := price.Mul(money.FromFloat(a.Qty))
		total = total.Add(amount)
		orderNo := allocationOrderNo(a)
		orderNos = appendUnique(orderNos, orderNo)

		items = append(items, map[string]string{
			"no":          strconv.Itoa(i + 1),
			"description": itemDescription(a.SKUCode, a.NameCN, a.Spec),
			"order_no":    orderNo,
			"qty":         qtyText(a.Qty),
			"unit":        a.Unit,
			"unit_price":  moneyTextD(price, curDP),
			"amount":      moneyTextD(amount, curDP),
		})
	}

	setNo := setNoText(sd)
	values := s.baseValues(baseCode)
	values["doc_no"] = setNo
	values["set_no"] = setNo
	values["ship_date"] = sd.ShipDate.Format("2006-01-02")
	values["trade_term"] = orDash(string(order.TradeTerm))
	values["order_nos"] = orDash(strings.Join(orderNos, ", "))
	values["vessel"] = orDash(sd.Vessel)
	values["bl_no"] = orDash(sd.BLNo)
	values["etd"] = dateText(sd.ETD)
	values["currency_code"] = order.CurrencyCode
	values["exchange_rate"] = rateTextD(order.ExchangeRate)
	values["amount_total"] = moneyTextD(total.Round(curDP), curDP)
	values["amount_base_total"] = moneyTextD(total.Mul(order.ExchangeRate).Round(baseDP), baseDP)
	values["substitution_note"] = substitutionNote(sd)
	s.partnerValues(values, order.Partner)

	data, err := s.render("invoice", pdf.Data{Values: values, Items: items})
	if err != nil {
		return nil, err
	}
	s.log.Info("生成商业发票 PDF", "id", sd.ID, "doc_no", setNo, "bytes", len(data))
	return &RenderedFile{Filename: setNo + "-CI.pdf", Data: data}, nil
}

// PackingList 出装箱单。
//
// 装箱数据允许还没填就打印：草稿阶段常要一份先给货代看柜型与体积。
// 空的那几格印「—」而不是 0——0 会被读成"这行确实没有纸箱"。
func (s *pdfService) PackingList(ctx context.Context, id uint) (*RenderedFile, error) {
	sd, order, err := s.shipmentSource(ctx, id)
	if err != nil {
		return nil, err
	}

	items := make([]map[string]string, 0, len(sd.Allocations))
	orderNos := make([]string, 0, len(sd.Allocations))
	for i, a := range sd.Allocations {
		orderNo := allocationOrderNo(a)
		orderNos = appendUnique(orderNos, orderNo)

		items = append(items, map[string]string{
			"no":          strconv.Itoa(i + 1),
			"description": itemDescription(a.SKUCode, a.NameCN, a.Spec),
			"order_no":    orderNo,
			"qty":         qtyText(a.Qty),
			"unit":        a.Unit,
			"cartons":     packedInt(a.Cartons),
			"gross":       packedWeight(a.GrossWeight),
			"net":         packedWeight(a.NetWeight),
			"measurement": orDash(a.Measurement),
		})
	}

	cartons, gross, net := sd.PackingTotals()
	setNo := setNoText(sd)
	values := s.headerValues()
	values["doc_no"] = setNo
	values["set_no"] = setNo
	values["ship_date"] = sd.ShipDate.Format("2006-01-02")
	values["order_nos"] = orDash(strings.Join(orderNos, ", "))
	values["vessel"] = orDash(sd.Vessel)
	values["bl_no"] = orDash(sd.BLNo)
	values["etd"] = dateText(sd.ETD)
	// 合计与明细同一口径：都是逐行相加得来，票头上不另存一份。
	values["cartons_total"] = packedInt(cartons)
	values["gross_total"] = packedWeight(gross)
	values["net_total"] = packedWeight(net)
	s.partnerValues(values, order.Partner)

	data, err := s.render("packing-list", pdf.Data{Values: values, Items: items})
	if err != nil {
		return nil, err
	}
	s.log.Info("生成装箱单 PDF", "id", sd.ID, "doc_no", setNo, "bytes", len(data))
	return &RenderedFile{Filename: setNo + "-PL.pdf", Data: data}, nil
}

// maxCartonLabels 是一次最多打几张箱唛。
//
// 装箱数据的箱数是人工填的，一位多打的 0 会让一次点击变成几万页 PDF，
// 服务端跟着卡死。上限给得足够宽（2000 箱），只拦明显的笔误。
const maxCartonLabels = 2000

// CartonLabels 出箱唛：一箱一张。
//
// 唛上只印两样由系统生成的东西——销售订单号（主标识）与套号（发票号），
// 箱号、毛净重、尺寸留空给人工填：分箱方案要到实际装箱时才定，系统先排一遍是重复劳动，
// 排错了还要跟着改。
//
// 张数按装箱数据的箱数走；某行还没填箱数时先给一张——样箱上也要贴。
// 混装箱（一箱装多张订单的货）系统不生成唛，那属于人工排版。
func (s *pdfService) CartonLabels(ctx context.Context, id uint) (*RenderedFile, error) {
	sd, err := s.shipments.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if len(sd.Allocations) == 0 {
		return nil, errs.ErrConflict.WithMsg("这一票还没有出货明细，打不出箱唛")
	}

	labels := make([]map[string]string, 0, len(sd.Allocations))
	for _, a := range sd.Allocations {
		n := a.Cartons
		if n <= 0 {
			n = 1
		}
		for i := 0; i < n; i++ {
			labels = append(labels, map[string]string{
				// 印订单号而不是送货单号：客户仓库按订单分货，而订单号在整票生命周期里不变。
				"order_no": allocationOrderNo(a),
				// 发票号随改证变（-R1），所以它是贴纸位，改证只换贴纸、不重贴整张唛。
				"invoice_no": setNoText(sd),
			})
		}
	}
	if len(labels) > maxCartonLabels {
		return nil, errs.BadRequest(fmt.Sprintf(
			"按装箱数据要打 %d 张箱唛，超过单次上限 %d 张；请先确认箱数是否填错",
			len(labels), maxCartonLabels))
	}

	data, err := s.renderer.RenderLabels("carton-label", labels)
	if err != nil {
		return nil, err
	}
	setNo := setNoText(sd)
	s.log.Info("生成箱唛 PDF", "id", sd.ID, "doc_no", setNo, "labels", len(labels), "bytes", len(data))
	return &RenderedFile{Filename: setNo + "-MARK.pdf", Data: data}, nil
}

// shipmentSource 取一票出货及其开票用的商务条件。
//
// 客户、币种、汇率、贸易术语都不在票头上：一票货的商务条件来自它装的订单。
// 而"一票只装同一个客户、同一个币种的订单"在写入时就已校验过，所以取第一张订单代表全票。
// 这里仍重新核一遍币种：历史数据可能早于那条校验，宁可不打，也不在一张发票上印两个币种。
func (s *pdfService) shipmentSource(
	ctx context.Context, id uint,
) (*model.Shipment, *model.SalesOrder, error) {
	sd, err := s.shipments.FindByID(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	if len(sd.Allocations) == 0 {
		return nil, nil, errs.ErrConflict.WithMsg("这一票还没有出货明细，打不出商业发票与装箱单")
	}

	head := sd.Allocations[0].SalesOrderItem
	if head == nil || head.SalesOrder == nil {
		return nil, nil, errs.ErrInternal.WithMsg("出货明细缺少来源订单，无法打印")
	}
	// 回仓储取整单：分配行里嵌的那一层订单不一定会带客户。
	order, err := s.orders.FindByID(ctx, head.SalesOrder.ID)
	if err != nil {
		return nil, nil, err
	}

	for _, a := range sd.Allocations {
		it := a.SalesOrderItem
		if it == nil || it.SalesOrder == nil {
			continue
		}
		if it.SalesOrder.CurrencyCode != order.CurrencyCode {
			return nil, nil, errs.ErrConflict.WithMsg(
				"这一票的明细来自不同币种的订单，打不出一张发票；请分票再打")
		}
	}
	return sd, order, nil
}

// sourceItems 取来源单据的明细行。
//
// 报价单与订单的明细字段同名，抽成同一组渲染键，模板就不必为两种来源分叉。
func (s *pdfService) sourceItems(
	ctx context.Context, c *model.Contract, curDP int,
) ([]map[string]string, string, string, error) {
	// 报价单与订单的明细字段同名，收成同一组渲染键，模板不必为两种来源分叉。
	// rowD 与 row 同形，只是单价与金额来自 money.Decimal：
	// 报价明细已改定点，打印时不必先转回 float64。
	rowD := func(i int, sku, name, spec, unit string, qty float64, unitPrice, amount money.Decimal) map[string]string {
		return map[string]string{
			"no":          strconv.Itoa(i + 1),
			"description": itemDescription(sku, name, spec),
			"qty":         qtyText(qty),
			"unit":        unit,
			"unit_price":  moneyTextD(unitPrice, curDP),
			"amount":      moneyTextD(amount, curDP),
		}
	}

	switch c.SourceType {
	case model.ContractSourceQuotation:
		q, err := s.quotations.FindByID(ctx, c.SourceID)
		if err != nil {
			return nil, "", "", err
		}
		rows := make([]map[string]string, 0, len(q.Items))
		for i, it := range q.Items {
			rows = append(rows, rowD(i, it.SKUCode, it.NameCN, it.Spec, it.Unit, it.Qty, it.UnitPrice, it.Amount))
		}
		return rows, model.ContractSourceQuotation.Label(), string(q.TradeTerm), nil

	case model.ContractSourceSalesOrder:
		o, err := s.orders.FindByID(ctx, c.SourceID)
		if err != nil {
			return nil, "", "", err
		}
		rows := make([]map[string]string, 0, len(o.Items))
		for i, it := range o.Items {
			rows = append(rows, rowD(i, it.SKUCode, it.NameCN, it.Spec, it.Unit, it.Qty, it.UnitPrice, it.Amount))
		}
		return rows, model.ContractSourceSalesOrder.Label(), string(o.TradeTerm), nil
	}
	return nil, "", "", errs.ErrInternal.WithMsg("合同来源类型未知，无法打印明细")
}

// decimals 取原币与本币的小数位。
//
// 金额按各自币种的小数位印：日元 0 位、第纳尔 3 位，统一按 2 位印，
// 客户会以为金额被改过，或者对账时差出几分钱。
func (s *pdfService) decimals(ctx context.Context, currencyCode string) (int, int, string, error) {
	baseCode, err := s.currencies.BaseCode(ctx)
	if err != nil {
		return 0, 0, "", err
	}
	curDP, err := s.currencies.Decimals(ctx, currencyCode)
	if err != nil {
		return 0, 0, "", err
	}
	baseDP, err := s.currencies.Decimals(ctx, baseCode)
	if err != nil {
		return 0, 0, "", err
	}
	return curDP, baseDP, baseCode, nil
}

// headerValues 是每份单据都有的抬头。
func (s *pdfService) headerValues() map[string]string {
	return map[string]string{
		"company_name":    s.company.Name,
		"company_address": s.company.Address,
		"company_contact": s.company.Contact,
	}
}

func (s *pdfService) baseValues(baseCode string) map[string]string {
	values := s.headerValues()
	values["base_currency"] = baseCode
	return values
}

// setNoText 印套号：送货单 / 商业发票 / 装箱单共用这一个号，改证只加 -R{n}，不换号。
//
// 换号等于换了一票货——报关单、提单与客户账上都认原号。
func setNoText(sd *model.Shipment) string {
	if sd.InvoiceRevision > 0 {
		return fmt.Sprintf("%s-R%d", sd.DocNo, sd.InvoiceRevision)
	}
	return sd.DocNo
}

// substitutionNote 是改证重开时的声明。
//
// 只写"替代哪个号"，不写原开单日期：系统没存下上一版的开单日，
// 在发票上编一个日期比不印更糟。
func substitutionNote(sd *model.Shipment) string {
	if sd.InvoiceRevision == 0 {
		return ""
	}
	return fmt.Sprintf("Issued in substitution of %s (revision %d).", sd.DocNo, sd.InvoiceRevision)
}

// allocationOrderNo 取分配行背后的订单号。
//
// 优先走预加载的整单，取不到时退回服务层摊平好的字段：
// 只读投影只在接口路径上填，PDF 走的是仓储。
func allocationOrderNo(a model.ShipmentAllocation) string {
	if a.SalesOrderItem != nil && a.SalesOrderItem.SalesOrder != nil {
		return a.SalesOrderItem.SalesOrder.DocNo
	}
	return a.SalesOrderDocNo
}

// appendUnique 去重后追加，保持出现顺序：一张订单的多行并进一票时，
// 发票上的 Your Order No. 只该出现一次。
func appendUnique(list []string, s string) []string {
	if s == "" {
		return list
	}
	for _, v := range list {
		if v == s {
			return list
		}
	}
	return append(list, s)
}

// packedInt 印箱数：0 的意思是"还没装箱"，与"装了 0 箱"无关，所以印「—」。
func packedInt(v int) string {
	if v <= 0 {
		return "—"
	}
	return strconv.Itoa(v)
}

// packedWeight 印毛净重：三位小数去掉尾零，0 同上印「—」。
func packedWeight(v float64) string {
	if v <= 0 {
		return "—"
	}
	return qtyText(v)
}

// partnerValues 填客户区块。空字段留空串，模板里那一行会被跳过——
// 与其印一行 "Address: " 让客户猜是不是漏打了，不如什么都不印。
func (s *pdfService) partnerValues(values map[string]string, p *model.Partner) {
	if p == nil {
		return
	}
	values["partner_name"] = p.Name
	values["partner_address"] = p.Address
	values["partner_country"] = p.Country
	values["partner_contact"] = strings.TrimSpace(p.ContactName + " " + p.Phone)
}

func (s *pdfService) render(name string, d pdf.Data) ([]byte, error) {
	b, err := s.renderer.Render(name, d)
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, "生成 PDF 失败")
	}
	return b, nil
}

// contractTitle 是印在纸上的标题：两种合同版式相同、只是抬头不同，
// 所以模板里只写一个占位，标题由类型决定。
func contractTitle(t model.ContractType) string {
	if t == model.ContractTypeSales {
		return "SALES CONTRACT"
	}
	return "PROFORMA INVOICE"
}

// itemDescription 把编码、品名与规格拼成一行。
//
// 模板的明细只有一列描述：分成三列之后，中文品名一长就挤成两行，
// 行高一变，整张表的横线全错位。
func itemDescription(sku, name, spec string) string {
	parts := make([]string, 0, 3)
	for _, s := range []string{strings.TrimSpace(sku), strings.TrimSpace(name)} {
		if s != "" {
			parts = append(parts, s)
		}
	}
	if spec = strings.TrimSpace(spec); spec != "" {
		parts = append(parts, "("+spec+")")
	}
	return strings.Join(parts, " ")
}

// moneyTextD 按币种小数位印金额并加千分位：单据是给人核对的，
// 1234567.00 和 1,234,567.00 差的不只是好看。
//
// 只留定点版本：改造后全仓的金额都是 money.Decimal，
// 再摆一个 float 版就会有人顺手把 Decimal 转回去打。
func moneyTextD(v money.Decimal, dp int) string {
	return groupThousands(v.StringFixed(dp))
}

// qtyText 印数量：最多三位小数，去掉无意义的尾零。
// 1000 印成 1,000.000 会让人以为数量有精度要求。
func qtyText(v float64) string {
	s := strconv.FormatFloat(v, 'f', 3, 64)
	s = strings.TrimRight(s, "0")
	s = strings.TrimSuffix(s, ".")
	return groupThousands(s)
}

func groupThousands(s string) string {
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	intPart, frac, hasFrac := strings.Cut(s, ".")

	var b strings.Builder
	for i := 0; i < len(intPart); i++ {
		if i > 0 && (len(intPart)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteByte(intPart[i])
	}
	out := b.String()
	if hasFrac {
		out += "." + frac
	}
	if neg {
		out = "-" + out
	}
	return out
}

// rateTextD 印汇率，去掉尾零：7.1000000 会让人以为汇率精确到小数点后七位。
func rateTextD(v money.Decimal) string { return v.String() }

func dateText(t *time.Time) string {
	if t == nil || t.IsZero() {
		return "—"
	}
	return t.Format("2006-01-02")
}

func orDash(s string) string {
	if s = strings.TrimSpace(s); s == "" {
		return "—"
	}
	return s
}
