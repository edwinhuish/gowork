package service

import (
	"context"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/money"
	"github.com/edwinhuish/gowork/internal/repository"
)

type reportFixture struct {
	reports    ReportService
	db         *gorm.DB
	now        time.Time
	customerID uint
	supplierID uint
}

// newReportFixture 造一套账龄报表的最小环境：一个客户、一个供应商。
//
// 单据是一条条直接写进去的（不走应收/应付服务）：账龄要的是"未核销余额与
// 到期日"，而这两样在核销路径里怎么产生，T-20/T-21 已经各自测过。
// 这里手工造数才能把"逾期 15 天""逾期 100 天"精确摆进各自那一桶。
func newReportFixture(t *testing.T, now time.Time) reportFixture {
	t.Helper()
	ctx := context.Background()

	db := openTestDB(t, &model.Partner{}, &model.Receivable{}, &model.CreditNote{}, &model.DebitNote{}, &model.Payable{})
	partnerRepo := repository.NewPartnerRepository(db)

	customer := &model.Partner{
		Code: "C001", Name: "Ferreteria El Sol", Role: model.PartnerRoleCustomer,
		EntityType: model.PartnerEntityCompany, Status: model.EnableStatusOn,
	}
	supplier := &model.Partner{
		Code: "S001", Name: "宁波五金", Role: model.PartnerRoleSupplier,
		EntityType: model.PartnerEntityCompany, Status: model.EnableStatusOn,
	}
	for _, p := range []*model.Partner{customer, supplier} {
		if err := partnerRepo.Create(ctx, p); err != nil {
			t.Fatalf("建主体失败: %v", err)
		}
	}

	return reportFixture{
		reports: NewReportService(
			repository.NewReportRepository(db),
			WithReportClock(func() time.Time { return now })),
		db:         db,
		now:        now,
		customerID: customer.ID,
		supplierID: supplier.ID,
	}
}

// addReceivable 写一张应收。settled>0 用来造"部分核销"。
func (f reportFixture) addReceivable(
	t *testing.T, docNo string, partnerID uint, currency string, rate, amount, settled float64,
	due time.Time, status model.SettlementStatus,
) {
	t.Helper()
	m := &model.Receivable{
		Settlement: model.Settlement{
			DocNo: docNo, PartnerID: partnerID,
			SourceType: model.SettlementSourceSalesOrder, SourceID: 1, SourceDocNo: "SO-TEST",
			CurrencyCode: currency, ExchangeRate: dec(rate),
			Amount: dec(amount), AmountBase: dec(amount).Mul(dec(rate)).Round(4),
			SettledAmount:     dec(settled),
			SettledAmountBase: dec(settled).Mul(dec(rate)).Round(4),
			DueDate:           model.DateOnly(due), Status: status,
		},
	}
	if err := f.db.Create(m).Error; err != nil {
		t.Fatalf("建应收 %s 失败: %v", docNo, err)
	}
}

func (f reportFixture) addPayable(
	t *testing.T, docNo string, partnerID uint, amount float64, due time.Time,
) {
	t.Helper()
	m := &model.Payable{
		Settlement: model.Settlement{
			DocNo: docNo, PartnerID: partnerID,
			SourceType: model.SettlementSourcePurchaseOrder, SourceID: 1, SourceDocNo: "PO-TEST",
			CurrencyCode: "CNY", ExchangeRate: money.FromInt(1),
			Amount: dec(amount), AmountBase: dec(amount),
			DueDate: model.DateOnly(due), Status: model.SettlementStatusOpen,
		},
	}
	if err := f.db.Create(m).Error; err != nil {
		t.Fatalf("建应付 %s 失败: %v", docNo, err)
	}
}

func findAgingRow(t *testing.T, rows []dto.AgingRow, partnerID uint, currency string) dto.AgingRow {
	t.Helper()
	for _, r := range rows {
		if r.PartnerID == partnerID && r.CurrencyCode == currency {
			return r
		}
	}
	t.Fatalf("报表里没有主体 %d / 币种 %s 的行，共 %d 行", partnerID, currency, len(rows))
	return dto.AgingRow{}
}

// TestAgingBucketsByPartner 是 T-22 的验收点：未到期与逾期 1-30 / 31-60 / 90+
// 各自落桶，作废单不计入，合计等于明细之和。
func TestAgingBucketsByPartner(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 10, 28, 9, 0, 0, 0, time.Local)
	f := newReportFixture(t, now)

	f.addReceivable(t, "RC-A1", f.customerID, "USD", 7, 100, 0, now.AddDate(0, 0, 10), model.SettlementStatusOpen)
	f.addReceivable(t, "RC-A2", f.customerID, "USD", 7, 200, 50, now.AddDate(0, 0, -15), model.SettlementStatusPartial)
	f.addReceivable(t, "RC-A3", f.customerID, "USD", 7, 400, 0, now.AddDate(0, 0, -100), model.SettlementStatusOpen)
	// 第二个主体：验证分行与排序，且不同币种不混加。
	f.addReceivable(t, "RC-B1", f.supplierID, "CNY", 1, 300, 0, now.AddDate(0, 0, -40), model.SettlementStatusOpen)
	// 作废单不占账龄：开错了的票据不该出现在催款清单里。
	f.addReceivable(t, "RC-VOID", f.customerID, "USD", 7, 999, 0, now.AddDate(0, 0, -5), model.SettlementStatusVoid)
	// 已结清的单也不该出现（settled == amount）。
	f.addReceivable(t, "RC-DONE", f.customerID, "USD", 7, 500, 500, now.AddDate(0, 0, -8), model.SettlementStatusSettled)

	report, err := f.reports.Aging(ctx, dto.AgingQuery{})
	if err != nil {
		t.Fatalf("账龄查询失败: %v", err)
	}
	if report.Direction != int8(model.SettlementTargetReceivable) {
		t.Errorf("留空方向应按应收，得到 %d", report.Direction)
	}
	if report.AsOf != "2026-10-28" {
		t.Errorf("留空 as_of 应取今天，得到 %s", report.AsOf)
	}
	if len(report.Rows) != 2 {
		t.Fatalf("应有 2 行（两个主体各一种币种），得到 %d", len(report.Rows))
	}

	a := findAgingRow(t, report.Rows, f.customerID, "USD")
	if a.Amount.NotDue.Cmp(money.MustParse("100")) != 0 {
		t.Errorf("未到期应为 100，得到 %s", a.Amount.NotDue)
	}
	if a.Amount.Days1To30.Cmp(money.MustParse("150")) != 0 {
		t.Errorf("1-30 天应为 200-50=150，得到 %s", a.Amount.Days1To30)
	}
	if !a.Amount.Days31To60.IsZero() || !a.Amount.Days61To90.IsZero() {
		t.Errorf("不该有 31-60 / 61-90 的金额，得到 %s / %s",
			a.Amount.Days31To60, a.Amount.Days61To90)
	}
	if a.Amount.Over90.Cmp(money.MustParse("400")) != 0 {
		t.Errorf("90+ 应为 400，得到 %s", a.Amount.Over90)
	}
	if a.Total.Cmp(money.MustParse("650")) != 0 {
		t.Errorf("合计应为 650，得到 %s", a.Total)
	}
	if a.TotalBase.Cmp(money.MustParse("4550")) != 0 {
		t.Errorf("本币合计应为 650*7=4550，得到 %s", a.TotalBase)
	}
	if a.EarliestDueDate != now.AddDate(0, 0, -100).Format("2006-01-02") {
		t.Errorf("最早到期日应取 100 天前那一张，得到 %s", a.EarliestDueDate)
	}

	b := findAgingRow(t, report.Rows, f.supplierID, "CNY")
	if b.Amount.Days31To60.Cmp(money.MustParse("300")) != 0 ||
		b.TotalBase.Cmp(money.MustParse("300")) != 0 {
		t.Errorf("人民币 300 逾期 40 天应落 31-60 桶，得到 %s（本币 %s）",
			b.Amount.Days31To60, b.TotalBase)
	}

	// 合计只给本币：100*7 + 150*7 + 400*7 + 300 = 4850。
	if report.TotalBase.NotDue.Cmp(money.MustParse("700")) != 0 {
		t.Errorf("本币未到期合计应为 700，得到 %s", report.TotalBase.NotDue)
	}
	if report.GrandTotalBase.Cmp(money.MustParse("4850")) != 0 {
		t.Errorf("本币合计应为 4850，得到 %s", report.GrandTotalBase)
	}
}

// TestAgingAsOfShiftsBuckets：同一批数据换个截止日，逾期天数跟着变，
// 桶也应该跟着换——这正是"账龄"而不是"固定标签"的含义。
func TestAgingAsOfShiftsBuckets(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 10, 28, 9, 0, 0, 0, time.Local)
	f := newReportFixture(t, now)

	f.addReceivable(t, "RC-A2", f.customerID, "USD", 7, 200, 0, now.AddDate(0, 0, -15), model.SettlementStatusOpen)

	report, err := f.reports.Aging(ctx, dto.AgingQuery{AsOf: now.AddDate(0, 0, 20).Format("2006-01-02")})
	if err != nil {
		t.Fatalf("账龄查询失败: %v", err)
	}
	row := findAgingRow(t, report.Rows, f.customerID, "USD")
	if row.Amount.Days31To60.Cmp(money.MustParse("200")) != 0 {
		t.Errorf("截止日后移 20 天应落进 31-60 桶（35 天），得到未到期 %s / 1-30 %s / 31-60 %s",
			row.Amount.NotDue, row.Amount.Days1To30, row.Amount.Days31To60)
	}
}

// TestAgingDueTodayIsNotDue：到期当天不算逾期。账期 30 天说的是第 30 天付，
// 第 30 天还在承诺期内，第 31 天才叫逾期。
func TestAgingDueTodayIsNotDue(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 10, 28, 9, 0, 0, 0, time.Local)
	f := newReportFixture(t, now)

	f.addReceivable(t, "RC-TODAY", f.customerID, "USD", 7, 100, 0, now, model.SettlementStatusOpen)
	f.addReceivable(t, "RC-Y1", f.customerID, "USD", 7, 60, 0, now.AddDate(0, 0, -1), model.SettlementStatusOpen)

	report, err := f.reports.Aging(ctx, dto.AgingQuery{})
	if err != nil {
		t.Fatalf("账龄查询失败: %v", err)
	}
	row := findAgingRow(t, report.Rows, f.customerID, "USD")
	if row.Amount.NotDue.Cmp(money.MustParse("100")) != 0 {
		t.Errorf("到期当天的 100 应算未到期，得到 %s", row.Amount.NotDue)
	}
	if row.Amount.Days1To30.Cmp(money.MustParse("60")) != 0 {
		t.Errorf("逾期 1 天的 60 应算 1-30 桶，得到 %s", row.Amount.Days1To30)
	}
}

func TestAgingRejectsBadDirection(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 10, 28, 9, 0, 0, 0, time.Local)
	f := newReportFixture(t, now)

	bad := int8(99)
	if _, err := f.reports.Aging(ctx, dto.AgingQuery{Direction: &bad}); err == nil {
		t.Error("非法方向应被拒绝")
	}
}

// TestAgingPayableSide：应付是应收的镜像，但两侧不能互相串台。
func TestAgingPayableSide(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 10, 28, 9, 0, 0, 0, time.Local)
	f := newReportFixture(t, now)

	f.addPayable(t, "PY-S1", f.supplierID, 500, now.AddDate(0, 0, -5))
	f.addReceivable(t, "RC-A1", f.customerID, "USD", 7, 100, 0, now.AddDate(0, 0, -5), model.SettlementStatusOpen)

	payableDir := int8(model.SettlementTargetPayable)
	report, err := f.reports.Aging(ctx, dto.AgingQuery{Direction: &payableDir})
	if err != nil {
		t.Fatalf("应付账龄查询失败: %v", err)
	}
	if len(report.Rows) != 1 || report.Rows[0].PartnerID != f.supplierID {
		t.Fatalf("应付账龄只应包含供应商那一行，得到 %+v", report.Rows)
	}
	if report.Rows[0].Amount.Days1To30.Cmp(money.MustParse("500")) != 0 {
		t.Errorf("应付 500 逾期 5 天应落 1-30 桶，得到 %s", report.Rows[0].Amount.Days1To30)
	}

	receivableReport, err := f.reports.Aging(ctx, dto.AgingQuery{})
	if err != nil {
		t.Fatalf("应收账龄查询失败: %v", err)
	}
	if len(receivableReport.Rows) != 1 || receivableReport.Rows[0].PartnerID != f.customerID {
		t.Errorf("应收账龄不该混入应付，得到 %+v", receivableReport.Rows)
	}
}

func TestAgingPartnerFilter(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 10, 28, 9, 0, 0, 0, time.Local)
	f := newReportFixture(t, now)

	f.addReceivable(t, "RC-A1", f.customerID, "USD", 7, 100, 0, now.AddDate(0, 0, -5), model.SettlementStatusOpen)
	f.addReceivable(t, "RC-B1", f.supplierID, "CNY", 1, 300, 0, now.AddDate(0, 0, -5), model.SettlementStatusOpen)

	pid := f.supplierID
	report, err := f.reports.Aging(ctx, dto.AgingQuery{PartnerID: &pid})
	if err != nil {
		t.Fatalf("按主体筛选失败: %v", err)
	}
	if len(report.Rows) != 1 || report.Rows[0].PartnerID != f.supplierID {
		t.Fatalf("筛选后只应剩一个主体，得到 %+v", report.Rows)
	}
	if report.GrandTotalBase.Cmp(money.MustParse("300")) != 0 {
		t.Errorf("筛选后的合计应为 300，得到 %s", report.GrandTotalBase)
	}
}
