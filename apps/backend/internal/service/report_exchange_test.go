package service

import (
	"context"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/repository"
)

type exchangeFixture struct {
	reports ReportService
	db      *gorm.DB
	seq     int
}

// newExchangeFixture 造汇兑损益的原料：应收/应付单（锁定汇率）与收付款单（到账汇率）。
//
// 全部直接写库：损耗的口径是"两个汇率相减"，而这两张单各自的汇率怎么定的，
// T-20（开单锁汇率）与 T-21（到账取当日汇率）已经各自测过。
func newExchangeFixture(t *testing.T) exchangeFixture {
	t.Helper()
	db := openTestDB(t,
		&model.Partner{}, &model.Receivable{}, &model.Payable{},
		&model.Payment{}, &model.PaymentAllocation{})
	return exchangeFixture{
		reports: NewReportService(repository.NewReportRepository(db)),
		db:      db,
	}
}

func (f *exchangeFixture) addReceivable(t *testing.T, docNo string, rate, amount float64) uint {
	t.Helper()
	return f.addReceivableIn(t, docNo, "USD", rate, amount)
}

func (f *exchangeFixture) addReceivableIn(
	t *testing.T, docNo, currency string, rate, amount float64,
) uint {
	t.Helper()
	m := &model.Receivable{
		Settlement: model.Settlement{
			DocNo: docNo, PartnerID: 1,
			SourceType: model.SettlementSourceSalesOrder, SourceID: 1, SourceDocNo: "SO-X",
			CurrencyCode: currency, ExchangeRate: rate,
			Amount: amount, AmountBase: amount * rate,
			DueDate: model.DateOnly(time.Now()), Status: model.SettlementStatusSettled,
		},
	}
	if err := f.db.Create(m).Error; err != nil {
		t.Fatalf("建应收失败: %v", err)
	}
	return m.ID
}

func (f *exchangeFixture) addPayable(t *testing.T, docNo string, rate, amount float64) uint {
	t.Helper()
	m := &model.Payable{
		Settlement: model.Settlement{
			DocNo: docNo, PartnerID: 2,
			SourceType: model.SettlementSourcePurchaseOrder, SourceID: 1, SourceDocNo: "PO-X",
			CurrencyCode: "USD", ExchangeRate: rate,
			Amount: amount, AmountBase: amount * rate,
			DueDate: model.DateOnly(time.Now()), Status: model.SettlementStatusSettled,
		},
	}
	if err := f.db.Create(m).Error; err != nil {
		t.Fatalf("建应付失败: %v", err)
	}
	return m.ID
}

func (f *exchangeFixture) addPayment(
	t *testing.T, docNo string, dir model.PaymentDirection, currency string,
	rate, amount, amountBase float64, payDate time.Time,
) uint {
	t.Helper()
	partnerID := uint(1)
	if dir == model.PaymentDirectionOut {
		partnerID = 2
	}
	m := &model.Payment{
		DocNo: docNo, Direction: dir, PartnerID: partnerID,
		PayDate:      model.DateOnly(payDate),
		CurrencyCode: currency, ExchangeRate: rate,
		Amount: amount, AmountBase: amountBase,
	}
	if err := f.db.Create(m).Error; err != nil {
		t.Fatalf("建收付款失败: %v", err)
	}
	return m.ID
}

func (f *exchangeFixture) allocate(
	t *testing.T, paymentID uint, target model.SettlementTarget, targetID uint,
	targetDocNo string, amount, amountBase float64,
) {
	t.Helper()
	m := &model.PaymentAllocation{
		PaymentID: paymentID, TargetType: target, TargetID: targetID,
		TargetDocNo: targetDocNo, Amount: amount, AmountBase: amountBase,
	}
	if err := f.db.Create(m).Error; err != nil {
		t.Fatalf("建核销行失败: %v", err)
	}
}

func (f exchangeFixture) report(t *testing.T, q dto.ExchangeGainQuery) *dto.ExchangeGainReport {
	t.Helper()
	res, err := f.reports.ExchangeGains(context.Background(), q)
	if err != nil {
		t.Fatalf("查询汇兑损益失败: %v", err)
	}
	return res
}

// TestExchangeGainReceivableIsGain：应收 1000 美元开票 7.1、到账 7.2，
// 同一笔钱多换回 100 元，是收益（正）。
func TestExchangeGainReceivableIsGain(t *testing.T) {
	f := newExchangeFixture(t)
	day := time.Date(2026, 10, 20, 0, 0, 0, 0, time.Local)

	rc := f.addReceivable(t, "RC-EX-1", 7.1, 1000)
	pay := f.addPayment(t, "PM-EX-1", model.PaymentDirectionIn, "USD", 7.2, 1000, 7200, day)
	f.allocate(t, pay, model.SettlementTargetReceivable, rc, "RC-EX-1", 1000, 7200)

	rep := f.report(t, dto.ExchangeGainQuery{})
	if len(rep.Lines) != 1 {
		t.Fatalf("应有 1 行，得到 %d", len(rep.Lines))
	}
	line := rep.Lines[0]
	wantMoney(t, "单据本币", line.DocumentBase, 7100)
	wantMoney(t, "到账本币", line.AmountBase, 7200)
	wantMoney(t, "汇兑损益", line.GainBase, 100)
	if line.PaymentDocNo != "PM-EX-1" || line.TargetDocNo != "RC-EX-1" {
		t.Errorf("应能追到收付款单与目标单，得到 %s / %s", line.PaymentDocNo, line.TargetDocNo)
	}
	if line.TargetType != int8(model.SettlementTargetReceivable) {
		t.Errorf("目标类型应为应收（10），得到 %d", line.TargetType)
	}
	if line.PayDate != "2026-10-20" {
		t.Errorf("到账日应为 2026-10-20，得到 %s", line.PayDate)
	}
	wantMoney(t, "合计", rep.TotalGainBase, 100)
}

// TestExchangeGainPayableIsLoss：应付 1000 美元账面 7.0、实付 7.2，
// 多付出去 200，是损失（负）。这条与上一条只差一个符号，写反了整张表都会变号。
func TestExchangeGainPayableIsLoss(t *testing.T) {
	f := newExchangeFixture(t)
	day := time.Date(2026, 10, 21, 0, 0, 0, 0, time.Local)

	py := f.addPayable(t, "PY-EX-1", 7.0, 1000)
	pay := f.addPayment(t, "PM-EX-2", model.PaymentDirectionOut, "USD", 7.2, 1000, 7200, day)
	f.allocate(t, pay, model.SettlementTargetPayable, py, "PY-EX-1", 1000, 7200)

	rep := f.report(t, dto.ExchangeGainQuery{})
	if len(rep.Lines) != 1 {
		t.Fatalf("应有 1 行，得到 %d", len(rep.Lines))
	}
	wantMoney(t, "汇兑损益", rep.Lines[0].GainBase, -200)
	wantMoney(t, "合计", rep.TotalGainBase, -200)
}

// TestExchangeGainPartialAndPrepay：部分核销按核销金额算；
// 预收款没有核销行，不该凭空产生损益。
func TestExchangeGainPartialAndPrepay(t *testing.T) {
	f := newExchangeFixture(t)
	day := time.Date(2026, 10, 22, 0, 0, 0, 0, time.Local)

	rc := f.addReceivable(t, "RC-EX-2", 7.1, 1000)
	pay := f.addPayment(t, "PM-EX-3", model.PaymentDirectionIn, "USD", 7.2, 400, 2880, day)
	f.allocate(t, pay, model.SettlementTargetReceivable, rc, "RC-EX-2", 400, 2880)
	// 另一笔预收款：还没有核销对象，谈不上差额。
	f.addPayment(t, "PM-EX-PREPAY", model.PaymentDirectionIn, "USD", 7.2, 500, 3600, day)

	rep := f.report(t, dto.ExchangeGainQuery{})
	if len(rep.Lines) != 1 {
		t.Fatalf("预收款不该出行，得到 %d 行", len(rep.Lines))
	}
	wantMoney(t, "部分核销的损益", rep.Lines[0].GainBase, 40)
	wantMoney(t, "合计", rep.TotalGainBase, 40)
}

// TestExchangeGainFiltersAndSubtotals：日期区间闭区间、主体与币种筛选，
// 以及按币种小计 + 跨币种本币合计。
func TestExchangeGainFiltersAndSubtotals(t *testing.T) {
	f := newExchangeFixture(t)
	old := time.Date(2026, 9, 1, 0, 0, 0, 0, time.Local)
	mid := time.Date(2026, 10, 15, 0, 0, 0, 0, time.Local)
	late := time.Date(2026, 10, 25, 0, 0, 0, 0, time.Local)

	rc := f.addReceivable(t, "RC-EX-3", 7.0, 1000)
	rc2 := f.addReceivable(t, "RC-EX-4", 7.0, 200)

	p1 := f.addPayment(t, "PM-EX-4", model.PaymentDirectionIn, "USD", 7.2, 1000, 7200, mid)
	f.allocate(t, p1, model.SettlementTargetReceivable, rc, "RC-EX-3", 1000, 7200)

	p2 := f.addPayment(t, "PM-EX-5", model.PaymentDirectionIn, "USD", 7.1, 200, 1420, late)
	f.allocate(t, p2, model.SettlementTargetReceivable, rc2, "RC-EX-4", 200, 1420)

	// 人民币收款挂在人民币应收上：跨币种核销在 T-21 就被拒了，这里造不出那种数据。
	rcCNY := f.addReceivableIn(t, "RC-EX-CNY", "CNY", 1, 500)
	p3 := f.addPayment(t, "PM-EX-6", model.PaymentDirectionIn, "CNY", 1, 500, 500, old)
	f.allocate(t, p3, model.SettlementTargetReceivable, rcCNY, "RC-EX-CNY", 500, 500)

	all := f.report(t, dto.ExchangeGainQuery{})
	if len(all.Lines) != 3 {
		t.Fatalf("应有 3 行，得到 %d", len(all.Lines))
	}
	if len(all.Subtotals) != 2 || all.Subtotals[0].CurrencyCode != "CNY" || all.Subtotals[1].CurrencyCode != "USD" {
		t.Fatalf("小计应按币种排序（CNY 在前），得到 %+v", all.Subtotals)
	}
	wantMoney(t, "USD 小计", all.Subtotals[1].GainBase, 220)
	wantMoney(t, "CNY 小计", all.Subtotals[0].GainBase, 0)
	wantMoney(t, "本币合计", all.TotalGainBase, 220)

	// 闭区间：mid 与 late 都在区间内，old 被排除。
	inRange := f.report(t, dto.ExchangeGainQuery{
		From: mid.Format("2006-01-02"),
		To:   late.Format("2006-01-02"),
	})
	if len(inRange.Lines) != 2 {
		t.Fatalf("区间内应有 2 行，得到 %d", len(inRange.Lines))
	}

	// 币种筛选。
	usdOnly := f.report(t, dto.ExchangeGainQuery{CurrencyCode: "USD"})
	if len(usdOnly.Lines) != 2 {
		t.Fatalf("按 USD 筛选应有 2 行，得到 %d", len(usdOnly.Lines))
	}

	// 主体筛选：应收都挂在客户 1 上。
	customer := uint(1)
	byPartner := f.report(t, dto.ExchangeGainQuery{PartnerID: &customer})
	if len(byPartner.Lines) != 3 {
		t.Errorf("客户 1 应有 3 行，得到 %d", len(byPartner.Lines))
	}
	supplier := uint(2)
	if got := f.report(t, dto.ExchangeGainQuery{PartnerID: &supplier}); len(got.Lines) != 0 {
		t.Errorf("供应商 2 不该有损益行，得到 %d", len(got.Lines))
	}
}

func TestExchangeGainRejectsReversedRange(t *testing.T) {
	f := newExchangeFixture(t)
	if _, err := f.reports.ExchangeGains(context.Background(), dto.ExchangeGainQuery{
		From: "2026-10-20",
		To:   "2026-10-01",
	}); err == nil {
		t.Error("起始日晚于截止日应被拒绝")
	}
}
