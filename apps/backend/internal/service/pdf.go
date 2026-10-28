package service

import (
	"context"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/errs"
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
	currencies CurrencyService,
	renderer *pdf.Renderer,
	company CompanyInfo,
	opts ...PDFOption,
) PDFService {
	s := &pdfService{
		quotations: quotations,
		orders:     orders,
		contracts:  contracts,
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
			"unit_price":  moneyText(it.UnitPrice, curDP),
			"amount":      moneyText(it.Amount, curDP),
		})
	}

	values := s.baseValues(baseCode)
	values["doc_no"] = q.DocNo
	values["date"] = q.CreatedAt.Format("2006-01-02")
	values["valid_until"] = q.ValidUntil.Format("2006-01-02")
	values["trade_term"] = string(q.TradeTerm)
	values["version"] = strconv.Itoa(q.Version)
	values["currency_code"] = q.CurrencyCode
	values["exchange_rate"] = rateText(q.ExchangeRate)
	values["amount_total"] = moneyText(q.AmountTotal, curDP)
	values["amount_base_total"] = moneyText(q.AmountBaseTotal, baseDP)
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
	termsTotal := 0.0
	for i, t := range c.Terms {
		terms = append(terms, map[string]string{
			"no":       strconv.Itoa(i + 1),
			"name":     t.Name,
			"due_date": dateText(t.DueDate),
			"trigger":  orDash(t.Trigger),
			"amount":   moneyText(t.Amount, curDP),
		})
		termsTotal += t.Amount
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
	values["exchange_rate"] = rateText(c.ExchangeRate)
	values["amount_total"] = moneyText(c.AmountTotal, curDP)
	values["amount_base_total"] = moneyText(c.AmountBaseTotal, baseDP)
	values["terms_total"] = moneyText(termsTotal, curDP)
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

// sourceItems 取来源单据的明细行。
//
// 报价单与订单的明细字段同名，抽成同一组渲染键，模板就不必为两种来源分叉。
func (s *pdfService) sourceItems(
	ctx context.Context, c *model.Contract, curDP int,
) ([]map[string]string, string, string, error) {
	// 报价单与订单的明细字段同名，收成同一组渲染键，模板不必为两种来源分叉。
	row := func(i int, sku, name, spec, unit string, qty, unitPrice, amount float64) map[string]string {
		return map[string]string{
			"no":          strconv.Itoa(i + 1),
			"description": itemDescription(sku, name, spec),
			"qty":         qtyText(qty),
			"unit":        unit,
			"unit_price":  moneyText(unitPrice, curDP),
			"amount":      moneyText(amount, curDP),
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
			rows = append(rows, row(i, it.SKUCode, it.NameCN, it.Spec, it.Unit, it.Qty, it.UnitPrice, it.Amount))
		}
		return rows, model.ContractSourceQuotation.Label(), string(q.TradeTerm), nil

	case model.ContractSourceSalesOrder:
		o, err := s.orders.FindByID(ctx, c.SourceID)
		if err != nil {
			return nil, "", "", err
		}
		rows := make([]map[string]string, 0, len(o.Items))
		for i, it := range o.Items {
			rows = append(rows, row(i, it.SKUCode, it.NameCN, it.Spec, it.Unit, it.Qty, it.UnitPrice, it.Amount))
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

func (s *pdfService) baseValues(baseCode string) map[string]string {
	return map[string]string{
		"company_name":    s.company.Name,
		"company_address": s.company.Address,
		"company_contact": s.company.Contact,
		"base_currency":   baseCode,
	}
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

// moneyText 按币种小数位印金额并加千分位：单据是给人核对的，
// 1234567.00 和 1,234,567.00 差的不只是好看。
func moneyText(v float64, dp int) string {
	return groupThousands(strconv.FormatFloat(v, 'f', dp, 64))
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

// rateText 印汇率，去掉尾零：7.1000000 会让人以为汇率精确到小数点后七位。
func rateText(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

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
