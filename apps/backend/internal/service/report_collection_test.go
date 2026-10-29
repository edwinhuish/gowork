package service

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/errs"
	"github.com/edwinhuish/gowork/internal/pkg/money"
	"github.com/edwinhuish/gowork/internal/repository"
)

// collectionFixture 是账龄 fixture 的扩展：多出收付款与核销两张表。
type collectionFixture struct {
	reportFixture
	seq int
}

func newCollectionFixture(t *testing.T, now time.Time) collectionFixture {
	t.Helper()
	db := openTestDB(t,
		&model.Partner{}, &model.Receivable{}, &model.Payable{},
		&model.Payment{}, &model.PaymentAllocation{}, &model.CreditNote{})
	partnerRepo := repository.NewPartnerRepository(db)

	customer := &model.Partner{
		Code: "C001", Name: "Ferreteria El Sol", Role: model.PartnerRoleCustomer,
		EntityType: model.PartnerEntityCompany, Status: model.EnableStatusOn,
	}
	if err := partnerRepo.Create(context.Background(), customer); err != nil {
		t.Fatalf("建主体失败: %v", err)
	}
	supplier := &model.Partner{
		Code: "S001", Name: "宁波五金", Role: model.PartnerRoleSupplier,
		EntityType: model.PartnerEntityCompany, Status: model.EnableStatusOn,
	}
	if err := partnerRepo.Create(context.Background(), supplier); err != nil {
		t.Fatalf("建主体失败: %v", err)
	}

	return collectionFixture{
		reportFixture: reportFixture{
			reports: NewReportService(
				repository.NewReportRepository(db),
				WithReportClock(func() time.Time { return now })),
			db:         db,
			now:        now,
			customerID: customer.ID,
			supplierID: supplier.ID,
		},
	}
}

// addPayment 建一张收付款单并挂一条核销行。方向与目标类型成对出现：
// 收款核应收、付款核应付，与 service 的校验保持一致。
func (f *collectionFixture) addPayment(
	t *testing.T, direction model.PaymentDirection, target model.SettlementTarget,
	payDate time.Time, amountBase float64,
) {
	t.Helper()
	f.seq++
	docNo := fmt.Sprintf("PM-%03d", f.seq)
	targetDocNo := fmt.Sprintf("RS-%03d", f.seq)
	partnerID := f.customerID
	if direction == model.PaymentDirectionOut {
		partnerID = f.supplierID
	}

	payment := &model.Payment{
		DocNo: docNo, Direction: direction, PartnerID: partnerID,
		PayDate: model.DateOnly(payDate), CurrencyCode: "CNY", ExchangeRate: dec(1),
		Amount: dec(amountBase), AmountBase: dec(amountBase),
	}
	if err := f.db.Create(payment).Error; err != nil {
		t.Fatalf("建收付款失败: %v", err)
	}
	alloc := &model.PaymentAllocation{
		PaymentID: payment.ID, TargetType: target, TargetID: 1, TargetDocNo: targetDocNo,
		Amount: dec(amountBase), AmountBase: dec(amountBase),
	}
	if err := f.db.Create(alloc).Error; err != nil {
		t.Fatalf("建核销行失败: %v", err)
	}
}

func (f collectionFixture) report(t *testing.T, q dto.CollectionQuery) *dto.CollectionReport {
	t.Helper()
	res, err := f.reports.Collection(context.Background(), q)
	if err != nil {
		t.Fatalf("查询回款报表失败: %v", err)
	}
	return res
}

// TestCollectionRate 是 T-29 的第一个验收点：回款率 = 期间到账并核销 ÷ 期间到期。
// 分母按到期日、分子按到账日，两批单据可以不同批——收的钱可能是在还上个月的账。
func TestCollectionRate(t *testing.T) {
	now := time.Date(2026, 10, 29, 9, 0, 0, 0, time.Local)
	f := newCollectionFixture(t, now)

	// 期间内到期：两张，共 1000 + 500 = 1500。
	f.addReceivable(t, "RS-001", f.customerID, "CNY", 1, 1000, 0,
		time.Date(2026, 10, 10, 0, 0, 0, 0, time.Local), model.SettlementStatusOpen)
	f.addReceivable(t, "RS-002", f.customerID, "CNY", 1, 500, 0,
		time.Date(2026, 10, 20, 0, 0, 0, 0, time.Local), model.SettlementStatusOpen)
	// 期间外到期：不该进分母。
	f.addReceivable(t, "RS-003", f.customerID, "CNY", 1, 3000, 0,
		time.Date(2026, 11, 20, 0, 0, 0, 0, time.Local), model.SettlementStatusOpen)

	// 期间内到账 900（0.6 × 1500）。
	f.addPayment(t, model.PaymentDirectionIn, model.SettlementTargetReceivable,
		time.Date(2026, 10, 25, 0, 0, 0, 0, time.Local), 900)
	// 期间外的到账不算：9 月收的钱不该进 10 月的分子。
	f.addPayment(t, model.PaymentDirectionIn, model.SettlementTargetReceivable,
		time.Date(2026, 9, 25, 0, 0, 0, 0, time.Local), 200)
	// 付给供应商的钱与应收无关。
	f.addPayment(t, model.PaymentDirectionOut, model.SettlementTargetPayable,
		time.Date(2026, 10, 25, 0, 0, 0, 0, time.Local), 400)

	res := f.report(t, dto.CollectionQuery{From: "2026-10-01", To: "2026-10-31"})
	wantDecimal(t, "期间到期", res.DueAmountBase, "1500")
	wantDecimal(t, "期间回款", res.ReceivedBase, "900")
	wantDecimal(t, "回款率", res.CollectionRate, "0.6")
	if res.AsOf != "2026-10-29" {
		t.Errorf("截止日应取今天，得到 %q", res.AsOf)
	}
}

// TestCollectionOverdueRows：只有**已逾期且未结清**的单据进明细，按逾期天数倒序。
// 未到期的计入未结清总额但不进催款清单。
func TestCollectionOverdueRows(t *testing.T) {
	now := time.Date(2026, 10, 29, 9, 0, 0, 0, time.Local)
	f := newCollectionFixture(t, now)

	// 逾期 10 天，未收。
	f.addReceivable(t, "RS-OLD10", f.customerID, "CNY", 1, 1000, 0,
		time.Date(2026, 10, 19, 0, 0, 0, 0, time.Local), model.SettlementStatusOpen)
	// 逾期 40 天，部分收了 400：未结清 600。
	f.addReceivable(t, "RS-OLD40", f.customerID, "CNY", 1, 1000, 400,
		time.Date(2026, 9, 19, 0, 0, 0, 0, time.Local), model.SettlementStatusPartial)
	// 未到期：不进明细，但计入未结清总额。
	f.addReceivable(t, "RS-NEXT", f.customerID, "CNY", 1, 2000, 0,
		time.Date(2026, 11, 30, 0, 0, 0, 0, time.Local), model.SettlementStatusOpen)
	// 已作废：哪边都不算。
	f.addReceivable(t, "RS-VOID", f.customerID, "CNY", 1, 5000, 0,
		time.Date(2026, 9, 1, 0, 0, 0, 0, time.Local), model.SettlementStatusVoid)

	res := f.report(t, dto.CollectionQuery{AsOf: "2026-10-29"})

	if len(res.OverdueRows) != 2 {
		t.Fatalf("应有 2 条超期明细，得到 %d: %+v", len(res.OverdueRows), res.OverdueRows)
	}
	first := res.OverdueRows[0]
	if first.DocNo != "RS-OLD40" || first.OverdueDays != 40 {
		t.Errorf("最该催的是逾期 40 天的 RS-OLD40，得到 %s（%d 天）",
			first.DocNo, first.OverdueDays)
	}
	wantDecimal(t, "部分核销后的未结清", first.Outstanding, "600")
	wantDecimal(t, "第二条未结清", res.OverdueRows[1].Outstanding, "1000")
	wantDecimal(t, "超期合计", res.OverdueBase, "1600")
	// 未结清总额含未到期的 2000：1600 + 2000，作废的 5000 不算。
	wantDecimal(t, "未结清总额", res.OutstandingBase, "3600")

	// 分桶与账龄报表同一套规则：10 天落 1-30，40 天落 31-60。
	wantDecimal(t, "1-30 桶", res.OverdueBuckets.Days1To30, "1000")
	wantDecimal(t, "31-60 桶", res.OverdueBuckets.Days31To60, "600")
	wantDecimal(t, "未到期桶（明细里不该有）", res.OverdueBuckets.NotDue, "0")
}

// limit 截断的是清单，不是合计：拿前几行自己加，会低估该催的钱。
func TestCollectionLimitKeepsTotals(t *testing.T) {
	now := time.Date(2026, 10, 29, 9, 0, 0, 0, time.Local)
	f := newCollectionFixture(t, now)

	for i, amount := range []float64{100, 200, 300} {
		f.addReceivable(t, fmt.Sprintf("RS-%03d", i), f.customerID, "CNY", 1, amount, 0,
			time.Date(2026, 9, 1+i, 0, 0, 0, 0, time.Local), model.SettlementStatusOpen)
	}

	res := f.report(t, dto.CollectionQuery{AsOf: "2026-10-29", Limit: intPtr(1)})
	if len(res.OverdueRows) != 1 || !res.Truncated {
		t.Fatalf("limit=1 应只返回 1 行并标截断，得到 %d 行 truncated=%v",
			len(res.OverdueRows), res.Truncated)
	}
	wantDecimal(t, "截断后的超期合计", res.OverdueBase, "600")
}

// 应付方向：看的是我们欠供应商的钱与我们付出去的钱，两张表都要换过来。
func TestCollectionPayableDirection(t *testing.T) {
	now := time.Date(2026, 10, 29, 9, 0, 0, 0, time.Local)
	f := newCollectionFixture(t, now)

	if err := f.db.Create(&model.Payable{Settlement: model.Settlement{
		DocNo: "PY-001", PartnerID: f.supplierID,
		SourceType: model.SettlementSourcePurchaseOrder, SourceID: 1, SourceDocNo: "PO-TEST",
		CurrencyCode: "CNY", ExchangeRate: dec(1),
		Amount: dec(800), AmountBase: dec(800),
		DueDate: model.DateOnly(time.Date(2026, 9, 1, 0, 0, 0, 0, time.Local)),
		Status:  model.SettlementStatusOpen,
	}}).Error; err != nil {
		t.Fatalf("建应付失败: %v", err)
	}
	f.addPayment(t, model.PaymentDirectionOut, model.SettlementTargetPayable,
		time.Date(2026, 9, 5, 0, 0, 0, 0, time.Local), 300)

	res := f.report(t, dto.CollectionQuery{
		Direction: intPtr8(20), From: "2026-09-01", To: "2026-09-30", AsOf: "2026-10-29",
	})
	if res.Direction != 20 {
		t.Errorf("方向应为应付 20，得到 %d", res.Direction)
	}
	wantDecimal(t, "期间应付到期", res.DueAmountBase, "800")
	wantDecimal(t, "期间付款", res.ReceivedBase, "300")
	if len(res.OverdueRows) != 1 || res.OverdueRows[0].DocNo != "PY-001" {
		t.Fatalf("应付超期明细不对: %+v", res.OverdueRows)
	}
}

func TestCollectionRejects(t *testing.T) {
	now := time.Date(2026, 10, 29, 9, 0, 0, 0, time.Local)
	f := newCollectionFixture(t, now)

	cases := []struct {
		name string
		q    dto.CollectionQuery
	}{
		{"方向非法", dto.CollectionQuery{Direction: intPtr8(30)}},
		{"起始晚于结束", dto.CollectionQuery{From: "2026-10-31", To: "2026-10-01"}},
		{"截止日格式不对", dto.CollectionQuery{AsOf: "2026/10/29"}},
		{"limit 为负", dto.CollectionQuery{Limit: intPtr(-1)}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := f.reports.Collection(context.Background(), c.q)
			wantCode(t, err, errs.CodeInvalidParam, c.name)
		})
	}
}

// 回款率的分母为 0 时给 0 而不是 NaN：分母为 0 的那一期没有"比率"可言。
func TestCollectionRateWithoutDue(t *testing.T) {
	now := time.Date(2026, 10, 29, 9, 0, 0, 0, time.Local)
	f := newCollectionFixture(t, now)

	res := f.report(t, dto.CollectionQuery{From: "2020-01-01", To: "2020-12-31"})
	wantDecimal(t, "无到期时的回款率", res.CollectionRate, "0")
	if res.CollectionRate.Cmp(money.Zero()) != 0 {
		t.Errorf("回款率应为 0，得到 %s", res.CollectionRate)
	}
}

func intPtr8(v int8) *int8 { return &v }
