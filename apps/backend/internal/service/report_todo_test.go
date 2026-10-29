package service

import (
	"context"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/errs"
	"github.com/edwinhuish/gowork/internal/repository"
)

type todoFixture struct {
	reports ReportService
	db      *gorm.DB
	now     time.Time
}

func newTodoFixture(t *testing.T, now time.Time) todoFixture {
	t.Helper()
	db := openTestDB(t,
		&model.Partner{}, &model.Activity{}, &model.PurchaseMilestone{}, &model.PurchaseOrder{}, &model.PurchaseOrderItem{}, &model.Receivable{}, &model.CreditNote{}, &model.DebitNote{},
		&model.Currency{}, &model.ExchangeRate{})
	return todoFixture{
		reports: NewReportService(
			repository.NewReportRepository(db),
			WithReportClock(func() time.Time { return now })),
		db:  db,
		now: now,
	}
}

func (f todoFixture) addPartner(t *testing.T, code string, ownerID uint) uint {
	t.Helper()
	p := &model.Partner{
		Code: code, Name: code, Role: model.PartnerRoleCustomer,
		EntityType: model.PartnerEntityCompany, Status: model.EnableStatusOn, OwnerID: ownerID,
	}
	if err := f.db.Create(p).Error; err != nil {
		t.Fatalf("建主体失败: %v", err)
	}
	return p.ID
}

func (f todoFixture) addFollowup(t *testing.T, partnerID uint, subject string, next time.Time) {
	t.Helper()
	m := &model.Activity{
		PartnerID: partnerID, Type: model.ActivityTypeCall, Subject: subject,
		OccurredAt: model.DateOnly(f.now), NextFollowAt: &[]time.Time{model.DateOnly(next)}[0],
	}
	if err := f.db.Create(m).Error; err != nil {
		t.Fatalf("建跟进失败: %v", err)
	}
}

func (f todoFixture) addReceivable(
	t *testing.T, docNo string, partnerID uint, amount, settled float64,
	due time.Time, status model.SettlementStatus,
) {
	t.Helper()
	m := &model.Receivable{Settlement: model.Settlement{
		DocNo: docNo, PartnerID: partnerID,
		SourceType: model.SettlementSourceSalesOrder, SourceID: 1, SourceDocNo: "SO-T",
		CurrencyCode: "CNY", ExchangeRate: dec(1),
		Amount: dec(amount), AmountBase: dec(amount),
		SettledAmount: dec(settled), SettledAmountBase: dec(settled),
		DueDate: model.DateOnly(due), Status: status,
	}}
	if err := f.db.Create(m).Error; err != nil {
		t.Fatalf("建应收失败: %v", err)
	}
}

func (f todoFixture) todos(t *testing.T, q dto.TodoQuery) *dto.TodoReport {
	t.Helper()
	res, err := f.reports.Todos(context.Background(), q)
	if err != nil {
		t.Fatalf("查待办失败: %v", err)
	}
	return res
}

// TestTodoFollowupBoundary：到期当天不算——早上还没开始联系就先标红，
// 提醒就成了噪音，人会学会无视它。
func TestTodoFollowupBoundary(t *testing.T) {
	now := time.Date(2026, 10, 29, 9, 0, 0, 0, time.Local)
	f := newTodoFixture(t, now)
	cus := f.addPartner(t, "CUS001", 0)

	f.addFollowup(t, cus, "昨天该跟", now.AddDate(0, 0, -1))
	f.addFollowup(t, cus, "今天该跟", now)
	f.addFollowup(t, cus, "明天该跟", now.AddDate(0, 0, 1))

	res := f.todos(t, dto.TodoQuery{})
	if res.FollowupCount != 1 || len(res.Followups) != 1 {
		t.Fatalf("只有昨天该跟的那条算到期，得到 count=%d rows=%d",
			res.FollowupCount, len(res.Followups))
	}
	if res.Followups[0].Subject != "昨天该跟" || res.Followups[0].OverdueDays != 1 {
		t.Errorf("记录或逾期天数不对: %+v", res.Followups[0])
	}
}

// TestTodoReceivableBoundary：到期当天就要提醒——催款是当天的事，等到明天就晚了一天。
func TestTodoReceivableBoundary(t *testing.T) {
	now := time.Date(2026, 10, 29, 9, 0, 0, 0, time.Local)
	f := newTodoFixture(t, now)
	cus := f.addPartner(t, "CUS001", 0)

	f.addReceivable(t, "RC-OLD", cus, 1000, 0, now.AddDate(0, 0, -5), model.SettlementStatusOpen)
	f.addReceivable(t, "RC-TODAY", cus, 500, 0, now, model.SettlementStatusOpen)
	f.addReceivable(t, "RC-NEXT", cus, 800, 0, now.AddDate(0, 0, 3), model.SettlementStatusOpen)
	f.addReceivable(t, "RC-PAID", cus, 300, 300, now.AddDate(0, 0, -2), model.SettlementStatusSettled)
	f.addReceivable(t, "RC-VOID", cus, 400, 0, now.AddDate(0, 0, -2), model.SettlementStatusVoid)

	res := f.todos(t, dto.TodoQuery{})
	if res.ReceivableCount != 2 {
		t.Fatalf("应只剩逾期与到期的两张，得到 %d", res.ReceivableCount)
	}
	// 最早的排最前。
	if res.Receivables[0].DocNo != "RC-OLD" || res.Receivables[0].OverdueDays != 5 {
		t.Errorf("应按到期日升序且带逾期天数，得到 %+v", res.Receivables[0])
	}
	if res.Receivables[1].DocNo != "RC-TODAY" || res.Receivables[1].OverdueDays != 0 {
		t.Errorf("到期当天应算在清单里，逾期天数为 0，得到 %+v", res.Receivables[1])
	}
}

// 缺汇率按"今天没有这条记录"算：有昨天的汇率不算有今天的。
func TestTodoMissingRates(t *testing.T) {
	now := time.Date(2026, 10, 29, 9, 0, 0, 0, time.Local)
	f := newTodoFixture(t, now)

	for _, c := range []*model.Currency{
		{Code: "CNY", Name: "人民币", IsBase: true, Status: model.EnableStatusOn},
		{Code: "USD", Name: "美元", Status: model.EnableStatusOn},
		{Code: "EUR", Name: "欧元", Status: model.EnableStatusOn},
		{Code: "JPY", Name: "日元", Status: model.EnableStatusOff},
	} {
		if err := f.db.Create(c).Error; err != nil {
			t.Fatalf("建币种失败: %v", err)
		}
	}
	// USD 有今天的；EUR 只有昨天的。
	for _, r := range []*model.ExchangeRate{
		{CurrencyCode: "USD", BaseCode: "CNY", Rate: dec(7.1), EffectiveDate: model.DateOnly(now)},
		{CurrencyCode: "EUR", BaseCode: "CNY", Rate: dec(7.8), EffectiveDate: model.DateOnly(now.AddDate(0, 0, -1))},
	} {
		if err := f.db.Create(r).Error; err != nil {
			t.Fatalf("建汇率失败: %v", err)
		}
	}

	res := f.todos(t, dto.TodoQuery{})
	if len(res.MissingRates) != 1 || res.MissingRates[0].Code != "EUR" {
		t.Fatalf("只该提示 EUR（停用的 JPY 与本币都不算），得到 %+v", res.MissingRates)
	}
}

// 提醒也不该把人叫去看别人的客户；汇率是主数据，业务员那里整块不出现。
func TestTodoScopeForStaff(t *testing.T) {
	now := time.Date(2026, 10, 29, 9, 0, 0, 0, time.Local)
	f := newTodoFixture(t, now)
	alice := f.addPartner(t, "CUS001", 1)
	bob := f.addPartner(t, "CUS002", 2)

	f.addFollowup(t, alice, "我的跟进", now.AddDate(0, 0, -1))
	f.addFollowup(t, bob, "别人的跟进", now.AddDate(0, 0, -1))
	f.addReceivable(t, "RC-A", alice, 100, 0, now.AddDate(0, 0, -1), model.SettlementStatusOpen)
	f.addReceivable(t, "RC-B", bob, 200, 0, now.AddDate(0, 0, -1), model.SettlementStatusOpen)

	ctx := WithActor(context.Background(), scopeActor(1, model.RoleStaff))
	res, err := f.reports.Todos(ctx, dto.TodoQuery{})
	if err != nil {
		t.Fatalf("查待办失败: %v", err)
	}
	if res.FollowupCount != 1 || res.Receivables != nil && len(res.Receivables) != 1 {
		t.Errorf("业务员只该看到自己客户的待办，得到 followups=%d receivables=%d",
			res.FollowupCount, len(res.Receivables))
	}
	if len(res.MissingRates) != 0 {
		t.Errorf("汇率维护不属于业务员，不该提示，得到 %+v", res.MissingRates)
	}

	// 主管看全部。
	bossCtx := WithActor(context.Background(), scopeActor(9, model.RoleManager))
	boss, err := f.reports.Todos(bossCtx, dto.TodoQuery{})
	if err != nil {
		t.Fatalf("查待办失败: %v", err)
	}
	if boss.FollowupCount != 2 || boss.ReceivableCount != 2 {
		t.Errorf("主管应看到全部 2 + 2，得到 %d + %d", boss.FollowupCount, boss.ReceivableCount)
	}
}

// 截断的是清单，不是条数：条数回答"还剩多少事"，清单回答"先做哪几件"。
func TestTodoLimitKeepsCount(t *testing.T) {
	now := time.Date(2026, 10, 29, 9, 0, 0, 0, time.Local)
	f := newTodoFixture(t, now)
	cus := f.addPartner(t, "CUS001", 0)
	for i := 1; i <= 5; i++ {
		f.addFollowup(t, cus, "跟进", now.AddDate(0, 0, -i))
	}

	res := f.todos(t, dto.TodoQuery{Limit: intPtr(2)})
	if len(res.Followups) != 2 {
		t.Fatalf("limit=2 应只返回 2 条，得到 %d", len(res.Followups))
	}
	if res.FollowupCount != 5 {
		t.Errorf("条数应是全量 5，得到 %d", res.FollowupCount)
	}
}

func TestTodoRejects(t *testing.T) {
	now := time.Date(2026, 10, 29, 9, 0, 0, 0, time.Local)
	f := newTodoFixture(t, now)

	_, err := f.reports.Todos(context.Background(), dto.TodoQuery{Limit: intPtr(-1)})
	wantCode(t, err, errs.CodeInvalidParam, "limit 为负")

	_, err = f.reports.Todos(context.Background(), dto.TodoQuery{AsOf: "2026/10/29"})
	wantCode(t, err, errs.CodeInvalidParam, "日期格式")
}
