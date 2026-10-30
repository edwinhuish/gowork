package service

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/errs"
	"github.com/edwinhuish/gowork/internal/pkg/pdf"
)

// WithPDFStatements 注入对账单服务（T-53 的对账单 PDF）。
//
// 用 Option 而不是加构造参数：PDF 服务的构造参数已经有七个，
// 而这里只是把"算账"这一步借过来用，账目口径仍然只有 StatementService 一份。
func WithPDFStatements(s StatementService) PDFOption {
	return func(p *pdfService) { p.statements = s }
}

// ReceivableStatement 出客户对账单 PDF。
//
// **算账不在这里**：期初 / 本期 / 期末由 StatementService 给出，
// 这里只负责排版。同一份数字在界面上与纸面上必须是同一个算法，
// 在 PDF 里再累加一遍，两边迟早会出现一分钱的差。
//
// 明细每行都带余额：对账单是拿来"照着验算"的，
// 只给借贷两列的话，客户得自己拿计算器从期初一路加下来。
func (s *pdfService) ReceivableStatement(
	ctx context.Context, q dto.StatementQuery,
) (*RenderedFile, error) {
	if s.statements == nil {
		return nil, errs.New(errs.CodeInternal, "对账单打印未接入")
	}
	st, err := s.statements.Statement(ctx, q)
	if err != nil {
		return nil, err
	}

	// 金额按该币种的小数位印：日元 0 位、第纳尔 3 位。
	// 一律按 2 位印，客户在纸上核出来的数会与系统里的差几分。
	dp, err := s.currencies.Decimals(ctx, st.Currency)
	if err != nil {
		return nil, err
	}

	running := st.Opening
	items := make([]map[string]string, 0, len(st.Lines))
	for i, line := range st.Lines {
		running = running.Add(line.Signed)
		debit, credit := "", ""
		switch {
		case line.Signed.IsNegative():
			credit = moneyTextD(line.Signed.Abs(), dp)
		case !line.Signed.IsZero():
			debit = moneyTextD(line.Signed, dp)
		}
		items = append(items, map[string]string{
			"no":      strconv.Itoa(i + 1),
			"date":    line.DocDate,
			"kind":    statementKindText(line.Kind),
			"doc_no":  line.DocNo,
			"debit":   debit,
			"credit":  credit,
			"balance": moneyTextD(running, dp),
			"remark":  orDash(line.Remark),
		})
	}

	issued := time.Now().Format("2006-01-02")
	values := map[string]string{
		"company_name":    s.company.Name,
		"company_address": s.company.Address,
		"company_contact": s.company.Contact,
		"partner_name":    st.PartnerName,
		"currency_code":   st.Currency,
		"period_text":     statementPeriod(st.From, st.To),
		"issued_at":       issued,
		"opening":         moneyTextD(st.Opening, dp),
		"debit":           moneyTextD(st.Debit, dp),
		"credit":          moneyTextD(st.Credit, dp),
		"closing":         moneyTextD(st.Closing, dp),
	}
	// 地址取「发票抬头」那一条，与 CI / PI 同一口径。
	// 取不到就整行不印（模板会跳过空行），而不是印一个空标题让人以为漏打。
	if addr, ok := s.defaultAddress(ctx, st.PartnerID, model.PartnerAddressInvoice); ok {
		values["partner_address"] = addressText(addr)
		values["partner_contact"] = strings.TrimSpace(addr.Contact + " " + addr.Phone)
	}

	data, err := s.render("receivable-statement", pdf.Data{Values: values, Items: items})
	if err != nil {
		return nil, err
	}

	// 文件名只用 ASCII：对账单没有单号，用「客户 id + 币种 + 出具日」定位，
	// 而客户名可能是中文——文件名里的中文在部分邮件网关会被改写。
	filename := fmt.Sprintf("STATEMENT-%06d-%s-%s.pdf",
		st.PartnerID, orDash(st.Currency), time.Now().Format("20060102"))
	s.log.Info("生成客户对账单 PDF", "partner_id", st.PartnerID, "currency", st.Currency,
		"lines", len(st.Lines), "closing", st.Closing, "bytes", len(data))
	return &RenderedFile{Filename: filename, Data: data}, nil
}

// statementKindText 是印在纸上的发生类型。
//
// 不直接印 kind 的原值：对账单是发给客户的，receivable / credit_note 这两个词
// 只有我们内部认，客户看到的是 invoice 与 credit note。
func statementKindText(kind string) string {
	switch kind {
	case "receivable":
		return "Invoice"
	case "credit_note":
		return "Credit Note"
	case "payment":
		return "Payment"
	default:
		return orDash(kind)
	}
}

// statementPeriod 把账期区间写成一行。两端都留空是不限账期，
// 这时写 "All transactions" 而不是留白——留白看起来像没填参数。
func statementPeriod(from, to string) string {
	switch {
	case from == "" && to == "":
		return "All transactions to date"
	case from == "":
		return "Up to " + to
	case to == "":
		return "From " + from
	default:
		return from + " ~ " + to
	}
}
