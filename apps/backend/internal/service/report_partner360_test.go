package service

import (
	"context"
	"fmt"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/errs"
	"github.com/edwinhuish/gowork/internal/pkg/money"
	"github.com/edwinhuish/gowork/internal/repository"
)

type partner360Fixture struct {
	reports ReportService
	db      *gorm.DB
	now     time.Time
	seq     int
	// lastItemID 是最近一次 addOrder 建的明细行 id，供 addShipment 挂分配用。
	lastItemID uint
}

func newPartner360Fixture(t *testing.T, now time.Time) partner360Fixture {
	t.Helper()
	db := openTestDB(t,
		&model.Partner{}, &model.Activity{}, &model.Opportunity{},
		&model.SalesOrder{}, &model.SalesOrderItem{},
		&model.Shipment{}, &model.ShipmentAllocation{}, &model.Receivable{})

	return partner360Fixture{
		reports: NewReportService(
			repository.NewReportRepository(db),
			WithReportClock(func() time.Time { return now })),
		db:  db,
		now: now,
	}
}

func (f *partner360Fixture) addPartner(t *testing.T, code, name, country string) uint {
	t.Helper()
	p := &model.Partner{
		Code: code, Name: name, Country: country,
		Role: model.PartnerRoleCustomer, EntityType: model.PartnerEntityCompany,
		Status: model.EnableStatusOn,
	}
	if err := f.db.Create(p).Error; err != nil {
		t.Fatalf("建主体失败: %v", err)
	}
	return p.ID
}

func (f *partner360Fixture) addOrder(
	t *testing.T, partnerID uint, docNo string, status model.DocumentStatus,
	orderDate time.Time, amountBase float64,
) uint {
	t.Helper()
	f.seq++
	order := &model.SalesOrder{
		DocNo: docNo, PartnerID: partnerID, TradeTerm: model.TradeTermFOB,
		Status: status, OrderDate: model.DateOnly(orderDate),
		CurrencyCode: "USD", ExchangeRate: money.MustParse("7.1"),
		AmountTotal: dec(amountBase / 7.1), AmountBaseTotal: dec(amountBase),
	}
	if err := f.db.Create(order).Error; err != nil {
		t.Fatalf("建订单 %s 失败: %v", docNo, err)
	}
	item := &model.SalesOrderItem{
		SalesOrderID: order.ID, ProductID: 1, SKUCode: "SKU-1", NameCN: "测试品", Unit: "PCS",
		Qty: dec(100), UnitPrice: dec(amountBase / 100),
		Amount: dec(amountBase), AmountBase: dec(amountBase),
	}
	if err := f.db.Create(item).Error; err != nil {
		t.Fatalf("建订单明细失败: %v", err)
	}
	f.lastItemID = item.ID
	return order.ID
}

func (f *partner360Fixture) addShipment(
	t *testing.T, docNo string, shipDate time.Time, salesItemIDs ...uint,
) uint {
	t.Helper()
	m := &model.Shipment{
		DocNo: docNo, Status: model.DocumentStatusShipped, ShipDate: model.DateOnly(shipDate),
	}
	if err := f.db.Create(m).Error; err != nil {
		t.Fatalf("建出货 %s 失败: %v", docNo, err)
	}
	for _, itemID := range salesItemIDs {
		alloc := &model.ShipmentAllocation{
			ShipmentID: m.ID, SalesOrderItemID: itemID,
			SKUCode: "SKU-1", NameCN: "测试品", Unit: "PCS", Qty: dec(50),
		}
		if err := f.db.Create(alloc).Error; err != nil {
			t.Fatalf("建分配行失败: %v", err)
		}
	}
	return m.ID
}

func (f *partner360Fixture) addReceivable(
	t *testing.T, docNo string, partnerID uint, currency string,
	rate, amount, settled float64, due time.Time, status model.SettlementStatus,
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
			DueDate:           model.DateOnly(due),
			Status:            status,
		},
	}
	if err := f.db.Create(m).Error; err != nil {
		t.Fatalf("建应收 %s 失败: %v", docNo, err)
	}
}

func (f *partner360Fixture) view(t *testing.T, q dto.Partner360Query) *dto.Partner360 {
	t.Helper()
	res, err := f.reports.Partner360(context.Background(), q)
	if err != nil {
		t.Fatalf("查询客户 360 失败: %v", err)
	}
	return res
}

// TestPartner360Overview 是 T-30 的验收点：一屏看到这家客户的全貌，
// 五个口径各自正确，且每块都能追到源单据（带 id 与单号）。
func TestPartner360Overview(t *testing.T) {
	now := time.Date(2026, 10, 29, 9, 0, 0, 0, time.Local)
	f := newPartner360Fixture(t, now)
	cus := f.addPartner(t, "CUS001", "Ferreteria El Sol", "Ecuador")

	// 两张有效订单 + 一张草稿（草稿不计入）。
	f.addOrder(t, cus, "SO-001", model.DocumentStatusConfirmed,
		time.Date(2026, 9, 1, 0, 0, 0, 0, time.Local), 7100)
	item1 := f.lastItemID
	f.addOrder(t, cus, "SO-002", model.DocumentStatusPreparing,
		time.Date(2026, 10, 1, 0, 0, 0, 0, time.Local), 2000)
	f.addOrder(t, cus, "SO-DRAFT", model.DocumentStatusDraft,
		time.Date(2026, 10, 2, 0, 0, 0, 0, time.Local), 9999)

	// 一票出货装 SO-001 的货。
	shipment := f.addShipment(t, "SH-001", time.Date(2026, 9, 20, 0, 0, 0, 0, time.Local), item1)

	// 应收：一张逾期（9/1 到期）、一张未到期（11/30）。
	f.addReceivable(t, "RC-001", cus, "CNY", 1, 1000, 300,
		time.Date(2026, 9, 1, 0, 0, 0, 0, time.Local), model.SettlementStatusPartial)
	f.addReceivable(t, "RC-002", cus, "CNY", 1, 800, 0,
		time.Date(2026, 11, 30, 0, 0, 0, 0, time.Local), model.SettlementStatusOpen)

	// 跟进两条、商机两条（一条已赢，不该占预测）。
	for i, subject := range []string{"第一次询盘", "寄样确认"} {
		act := &model.Activity{
			PartnerID: cus, Type: model.ActivityTypeCall, Subject: subject,
			OccurredAt: time.Date(2026, 9, 10+i, 0, 0, 0, 0, time.Local),
		}
		if err := f.db.Create(act).Error; err != nil {
			t.Fatalf("建跟进失败: %v", err)
		}
	}
	for _, o := range []*model.Opportunity{
		{
			PartnerID: cus, Subject: "2027 春季补单", Stage: model.OpportunityStageNegotiating,
			Amount: dec(5000), CurrencyCode: "CNY", AmountBase: dec(5000),
			ExchangeRate: dec(1), Probability: 50,
			ExpectedCloseDate: model.DateOnly(time.Date(2026, 12, 15, 0, 0, 0, 0, time.Local)),
		},
		{
			PartnerID: cus, Subject: "去年的单", Stage: model.OpportunityStageWon,
			Amount: dec(9000), CurrencyCode: "CNY", AmountBase: dec(9000),
			ExchangeRate: dec(1), Probability: 100,
			ExpectedCloseDate: model.DateOnly(time.Date(2026, 6, 1, 0, 0, 0, 0, time.Local)),
		},
	} {
		if err := f.db.Create(o).Error; err != nil {
			t.Fatalf("建商机失败: %v", err)
		}
	}

	v := f.view(t, dto.Partner360Query{PartnerID: cus})

	if v.PartnerCode != "CUS001" || v.PartnerName != "Ferreteria El Sol" || v.Country != "Ecuador" {
		t.Errorf("抬头不对: %+v", v)
	}
	if v.AsOf != "2026-10-29" {
		t.Errorf("截止日应取今天，得到 %q", v.AsOf)
	}

	t.Run("订单口径", func(t *testing.T) {
		if v.Totals.OrderCount != 2 {
			t.Errorf("草稿不该计入，期望 2 张，得到 %d", v.Totals.OrderCount)
		}
		wantDecimal(t, "订单额合计", v.Totals.OrderAmountBase, "9100")
		if v.Totals.LastOrderDate != "2026-10-01" {
			t.Errorf("最近下单日应为 2026-10-01，得到 %q", v.Totals.LastOrderDate)
		}
		if len(v.Orders) != 2 || v.Orders[0].DocNo != "SO-002" {
			t.Errorf("订单应按日期倒序且不含草稿，得到 %+v", v.Orders)
		}
		if v.Orders[0].ID == 0 {
			t.Error("订单行必须带 id，否则点不进源单据")
		}
	})

	t.Run("出货口径按订单行归属", func(t *testing.T) {
		if v.Totals.ShipmentCount != 1 {
			t.Errorf("应有 1 票出货，得到 %d", v.Totals.ShipmentCount)
		}
		if len(v.Shipments) != 1 || v.Shipments[0].ID != shipment {
			t.Fatalf("出货明细不对: %+v", v.Shipments)
		}
	})

	t.Run("应收与超期", func(t *testing.T) {
		// 1000−300 + 800 = 1500，其中逾期的 700（9/1 到期）。
		wantDecimal(t, "未结清合计", v.Totals.ReceivableOutstandingBase, "1500")
		wantDecimal(t, "超期合计", v.Totals.ReceivableOverdueBase, "700")
		if len(v.Receivables) != 2 {
			t.Fatalf("应有 2 张未结清应收，得到 %d", len(v.Receivables))
		}
		// 最早到期排前面：逾期那张。
		first := v.Receivables[0]
		if first.DocNo != "RC-001" || first.OverdueDays <= 0 {
			t.Errorf("最早到期的应排前面且带逾期天数，得到 %+v", first)
		}
		wantDecimal(t, "部分核销后的未结清", first.Outstanding, "700")
		if v.Receivables[1].OverdueDays != 0 {
			t.Errorf("未到期的单据逾期天数应为 0，得到 %d", v.Receivables[1].OverdueDays)
		}
	})

	t.Run("跟进与商机", func(t *testing.T) {
		if v.Totals.ActivityCount != 2 || len(v.Activities) != 2 {
			t.Errorf("跟进条数不对: total=%d rows=%d", v.Totals.ActivityCount, len(v.Activities))
		}
		if v.Totals.LastActivityAt != "2026-09-11" {
			t.Errorf("最近跟进日应为 2026-09-11，得到 %q", v.Totals.LastActivityAt)
		}
		if v.Totals.OpenOpportunityCount != 1 {
			t.Errorf("已赢单不该占预测，期望 1 条在谈，得到 %d", v.Totals.OpenOpportunityCount)
		}
		wantDecimal(t, "在谈商机金额", v.Totals.OpenOpportunityAmountBase, "5000")
		if len(v.Opportunities) != 1 || v.Opportunities[0].ID == 0 {
			t.Errorf("商机明细不对: %+v", v.Opportunities)
		}
	})
}

// 拼柜：一票里装了两家的货，这票要算给两家各一次，而不是只算给票头能查到的那一家。
func TestPartner360ShipmentSharedAcrossCustomers(t *testing.T) {
	now := time.Date(2026, 10, 29, 9, 0, 0, 0, time.Local)
	f := newPartner360Fixture(t, now)
	cusA := f.addPartner(t, "CUS001", "甲方", "Ecuador")
	cusB := f.addPartner(t, "CUS002", "乙方", "China")

	f.addOrder(t, cusA, "SO-A", model.DocumentStatusConfirmed, now, 1000)
	itemA := f.lastItemID
	f.addOrder(t, cusB, "SO-B", model.DocumentStatusConfirmed, now, 2000)
	itemB := f.lastItemID

	f.addShipment(t, "SH-MIX", now, itemA, itemB)

	for _, cus := range []uint{cusA, cusB} {
		v := f.view(t, dto.Partner360Query{PartnerID: cus})
		if v.Totals.ShipmentCount != 1 {
			t.Errorf("主体 %d 的出货票数应为 1（去重），得到 %d", cus, v.Totals.ShipmentCount)
		}
		if len(v.Shipments) != 1 {
			t.Errorf("主体 %d 的出货明细应有 1 行，得到 %d", cus, len(v.Shipments))
		}
	}
}

// 截断只影响展示：抬头的数量与金额必须是全量。
func TestPartner360LimitKeepsTotals(t *testing.T) {
	now := time.Date(2026, 10, 29, 9, 0, 0, 0, time.Local)
	f := newPartner360Fixture(t, now)
	cus := f.addPartner(t, "CUS001", "甲方", "Ecuador")

	for i := 1; i <= 5; i++ {
		f.addOrder(t, cus, fmt.Sprintf("SO-%03d", i), model.DocumentStatusConfirmed,
			now.AddDate(0, 0, -i), 1000)
	}

	v := f.view(t, dto.Partner360Query{PartnerID: cus, Limit: intPtr(2)})
	if len(v.Orders) != 2 {
		t.Fatalf("limit=2 应只返回 2 张，得到 %d", len(v.Orders))
	}
	if v.Totals.OrderCount != 5 {
		t.Errorf("合计应为全量的 5 张，得到 %d", v.Totals.OrderCount)
	}
	wantDecimal(t, "全量订单额", v.Totals.OrderAmountBase, "5000")
}

func TestPartner360Rejects(t *testing.T) {
	now := time.Date(2026, 10, 29, 9, 0, 0, 0, time.Local)
	f := newPartner360Fixture(t, now)
	cus := f.addPartner(t, "CUS001", "甲方", "Ecuador")

	t.Run("主体不存在", func(t *testing.T) {
		_, err := f.reports.Partner360(context.Background(), dto.Partner360Query{PartnerID: 9999})
		wantCode(t, err, errs.CodeNotFound, "主体不存在")
	})
	t.Run("没指定主体", func(t *testing.T) {
		_, err := f.reports.Partner360(context.Background(), dto.Partner360Query{})
		wantCode(t, err, errs.CodeInvalidParam, "没指定主体")
	})
	t.Run("limit 为负", func(t *testing.T) {
		_, err := f.reports.Partner360(context.Background(),
			dto.Partner360Query{PartnerID: cus, Limit: intPtr(-1)})
		wantCode(t, err, errs.CodeInvalidParam, "limit 为负")
	})
	t.Run("截止日格式不对", func(t *testing.T) {
		_, err := f.reports.Partner360(context.Background(),
			dto.Partner360Query{PartnerID: cus, AsOf: "2026/10/29"})
		wantCode(t, err, errs.CodeInvalidParam, "截止日格式")
	})
}
