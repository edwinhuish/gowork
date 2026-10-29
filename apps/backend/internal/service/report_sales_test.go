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

type salesFixture struct {
	reports ReportService
	db      *gorm.DB
	now     time.Time
	seq     int
}

// newSalesFixture 直接写库造原料：看板的口径是"怎么把已有的数拼起来"，
// 而订单行与采购行的金额怎么产生，T-10 / T-11 各自已经测过。
func newSalesFixture(t *testing.T, now time.Time) salesFixture {
	t.Helper()

	db := openTestDB(t,
		&model.Partner{}, &model.Product{}, &model.SalesOrder{}, &model.SalesOrderItem{},
		&model.PurchaseOrder{}, &model.PurchaseOrderItem{},
		&model.Shipment{}, &model.ShipmentAllocation{}, &model.ShipmentCharge{})

	return salesFixture{
		reports: NewReportService(
			repository.NewReportRepository(db),
			WithReportClock(func() time.Time { return now })),
		db:  db,
		now: now,
	}
}

func (f *salesFixture) addProduct(t *testing.T, sku, name, category string) uint {
	t.Helper()
	p := &model.Product{SKUCode: sku, NameCN: name, Category: category, Unit: "PCS"}
	if err := f.db.Create(p).Error; err != nil {
		t.Fatalf("建产品失败: %v", err)
	}
	return p.ID
}

func (f *salesFixture) addPartner(t *testing.T, code, name, country string) uint {
	t.Helper()
	p := &model.Partner{
		Code: code, Name: name, Country: country,
		Role: model.PartnerRoleCustomer, Status: model.EnableStatusOn,
	}
	if err := f.db.Create(p).Error; err != nil {
		t.Fatalf("建主体失败: %v", err)
	}
	return p.ID
}

// salesItemSpec 是一行订单明细的量：产品、数量与本币金额。
type salesItemSpec struct {
	productID uint
	sku       string
	name      string
	qty       float64
	amount    float64
}

func (f *salesFixture) addOrder(
	t *testing.T, partnerID uint, orderDate time.Time, items []salesItemSpec,
) []uint {
	t.Helper()
	f.seq++
	order := &model.SalesOrder{
		DocNo: fmt.Sprintf("SO-SALES-%d", f.seq), PartnerID: partnerID,
		TradeTerm: model.TradeTermFOB, Status: model.DocumentStatusConfirmed,
		OrderDate: model.DateOnly(orderDate), CurrencyCode: "USD",
		ExchangeRate: money.MustParse("7.1"),
	}
	if err := f.db.Create(order).Error; err != nil {
		t.Fatalf("建销售订单失败: %v", err)
	}

	ids := make([]uint, 0, len(items))
	for _, it := range items {
		item := &model.SalesOrderItem{
			SalesOrderID: order.ID, ProductID: it.productID,
			SKUCode: it.sku, NameCN: it.name, Unit: "PCS",
			Qty: qtyOf(it.qty), UnitPrice: money.FromFloat(it.amount / it.qty),
			Amount: money.FromFloat(it.amount), AmountBase: money.FromFloat(it.amount),
		}
		if err := f.db.Create(item).Error; err != nil {
			t.Fatalf("建销售明细失败: %v", err)
		}
		ids = append(ids, item.ID)
	}
	return ids
}

func (f *salesFixture) addPurchase(t *testing.T, salesItemID uint, qty, amountBase float64) {
	t.Helper()
	f.seq++
	itemID := salesItemID
	order := &model.PurchaseOrder{
		DocNo: fmt.Sprintf("PO-SALES-%d", f.seq), SupplierID: 2,
		TradeTerm: model.TradeTermEXW, Status: model.DocumentStatusConfirmed,
		OrderDate: model.DateOnly(f.now), CurrencyCode: "CNY", ExchangeRate: money.FromInt(1),
	}
	if err := f.db.Create(order).Error; err != nil {
		t.Fatalf("建采购订单失败: %v", err)
	}
	item := &model.PurchaseOrderItem{
		PurchaseOrderID: order.ID, ProductID: 1, SKUCode: "SKU-1", Unit: "PCS",
		Qty: qtyOf(qty), UnitPrice: money.FromFloat(amountBase / qty),
		Amount: money.FromFloat(amountBase), AmountBase: money.FromFloat(amountBase),
		SalesOrderItemID: &itemID,
	}
	if err := f.db.Create(item).Error; err != nil {
		t.Fatalf("建采购明细失败: %v", err)
	}
}

type allocSpec struct {
	salesItemID uint
	qty         float64
}

func (f *salesFixture) addShipment(t *testing.T, shipDate time.Time, allocs []allocSpec) uint {
	t.Helper()
	f.seq++
	m := &model.Shipment{
		DocNo:  fmt.Sprintf("SH-SALES-%d", f.seq),
		Status: model.DocumentStatusShipped, ShipDate: model.DateOnly(shipDate),
	}
	if err := f.db.Create(m).Error; err != nil {
		t.Fatalf("建出货批次失败: %v", err)
	}
	for _, a := range allocs {
		alloc := &model.ShipmentAllocation{
			ShipmentID: m.ID, SalesOrderItemID: a.salesItemID,
			SKUCode: "SKU-1", NameCN: "测试品", Unit: "PCS", Qty: qtyOf(a.qty),
		}
		if err := f.db.Create(alloc).Error; err != nil {
			t.Fatalf("建分配行失败: %v", err)
		}
	}
	return m.ID
}

func (f *salesFixture) addCharge(t *testing.T, shipmentID uint, amountBase float64) {
	t.Helper()
	c := &model.ShipmentCharge{
		ShipmentID: shipmentID, Type: model.ShipmentChargeFreight,
		CurrencyCode: "CNY", ExchangeRate: money.FromInt(1),
		Amount: money.FromFloat(amountBase), AmountBase: money.FromFloat(amountBase),
	}
	if err := f.db.Create(c).Error; err != nil {
		t.Fatalf("建运杂费失败: %v", err)
	}
}

func (f *salesFixture) dashboard(t *testing.T, q dto.SalesDashboardQuery) *dto.SalesDashboard {
	t.Helper()
	res, err := f.reports.SalesDashboard(context.Background(), q)
	if err != nil {
		t.Fatalf("查询看板失败: %v", err)
	}
	return res
}

// TestSalesDashboardTotalsAgreeAcrossDimensions 是 T-28 的核心验收：
// 同一批数据换维度看，合计必须一模一样，且与逐票毛利相加的结果相等。
// 只要哪个维度偷偷换了一份口径，这里立刻会响。
func TestSalesDashboardTotalsAgreeAcrossDimensions(t *testing.T) {
	now := time.Date(2026, 10, 28, 9, 0, 0, 0, time.Local)
	f := newSalesFixture(t, now)

	cusA := f.addPartner(t, "CUS001", "Ferreteria El Sol", "Ecuador")
	cusB := f.addPartner(t, "CUS002", "宁波华翔五金", "China")

	// 票一：客户 A、产品 1，收入 7100、成本 5000、运费 500 → 毛利 1600。
	a1 := f.addOrder(t, cusA, now, []salesItemSpec{{productID: 1, sku: "BH01-0001", name: "合页", qty: 100, amount: 7100}})[0]
	f.addPurchase(t, a1, 100, 5000)
	s1 := f.addShipment(t, now, []allocSpec{{salesItemID: a1, qty: 100}})
	f.addCharge(t, s1, 500)

	// 票二：客户 B、产品 2，收入 2000、成本 1200、运费 100 → 毛利 700。
	b1 := f.addOrder(t, cusB, now, []salesItemSpec{{productID: 2, sku: "HT02-0007", name: "锁体", qty: 50, amount: 2000}})[0]
	f.addPurchase(t, b1, 50, 1200)
	s2 := f.addShipment(t, now, []allocSpec{{salesItemID: b1, qty: 50}})
	f.addCharge(t, s2, 100)

	// 逐票毛利是口径的源头，看板的总计必须与它逐项相等。
	profits, err := f.reports.ShipmentProfits(context.Background(),
		dto.ShipmentProfitQuery{})
	if err != nil {
		t.Fatalf("查询单票毛利失败: %v", err)
	}
	wantRevenue, wantCost, wantCharge, wantProfit := money.Zero(), money.Zero(), money.Zero(), money.Zero()
	for _, p := range profits.List {
		wantRevenue = wantRevenue.Add(p.Revenue)
		wantCost = wantCost.Add(p.Cost)
		wantCharge = wantCharge.Add(p.Charge)
		wantProfit = wantProfit.Add(p.Profit)
	}

	for _, dim := range []string{"customer", "product", "country", "month"} {
		res := f.dashboard(t, dto.SalesDashboardQuery{Dimension: dim})
		if res.Total.Revenue.Cmp(wantRevenue) != 0 ||
			res.Total.Cost.Cmp(wantCost) != 0 ||
			res.Total.Charge.Cmp(wantCharge) != 0 ||
			res.Total.Profit.Cmp(wantProfit) != 0 {
			t.Errorf("维度 %s 的合计与单票毛利不一致: 收入 %s/%s 成本 %s/%s 运费 %s/%s 毛利 %s/%s",
				dim, res.Total.Revenue, wantRevenue, res.Total.Cost, wantCost,
				res.Total.Charge, wantCharge, res.Total.Profit, wantProfit)
		}
		if res.Total.Shipments != 2 {
			t.Errorf("维度 %s 的票数应为 2，得到 %d", dim, res.Total.Shipments)
		}
	}
}

// 运杂费按票发生、按行归集：摊到各行之和必须精确等于票面总额，
// 否则按产品看的成本会比按客户看的多出几分钱。
func TestSalesDashboardProratesChargeWithoutLosingCents(t *testing.T) {
	now := time.Date(2026, 10, 28, 9, 0, 0, 0, time.Local)
	f := newSalesFixture(t, now)

	cus := f.addPartner(t, "CUS001", "Ferreteria El Sol", "Ecuador")
	items := f.addOrder(t, cus, now, []salesItemSpec{
		{productID: 1, sku: "BH01-0001", name: "合页", qty: 100, amount: 3000},
		{productID: 2, sku: "HT02-0007", name: "锁体", qty: 100, amount: 6000},
	})
	f.addPurchase(t, items[0], 100, 2000)
	f.addPurchase(t, items[1], 100, 4000)
	// 一票两行，收入比 1:2；运费 900 应当摊成 300 / 600。
	s := f.addShipment(t, now, []allocSpec{
		{salesItemID: items[0], qty: 100},
		{salesItemID: items[1], qty: 100},
	})
	f.addCharge(t, s, 900)

	res := f.dashboard(t, dto.SalesDashboardQuery{Dimension: "product"})
	if len(res.Rows) != 2 {
		t.Fatalf("应有两行产品，得到 %d", len(res.Rows))
	}

	byKey := map[string]dto.SalesDashboardRow{}
	for _, row := range res.Rows {
		byKey[row.Key] = row
	}
	// 各行份额按 1:2 摊，但比例只有 6 位小数，单行会差出零点几个 0.0001；
	// 真正不能差的是**合计**与"各行相加等于票面"，那两条下面按精确值断言。
	closeDecimal(t, "合页运费", byKey["1"].Charge, "300", "0.001")
	closeDecimal(t, "锁体运费", byKey["2"].Charge, "600", "0.001")
	wantDecimal(t, "各行运费之和", byKey["1"].Charge.Add(byKey["2"].Charge), "900")
	wantDecimal(t, "运费合计", res.Total.Charge, "900")
	// 毛利各自 = 收入 − 成本 − 摊到的运费：合页 3000−2000−300，锁体 6000−4000−600。
	closeDecimal(t, "合页毛利", byKey["1"].Profit, "700", "0.001")
	closeDecimal(t, "锁体毛利", byKey["2"].Profit, "1400", "0.001")
}

// closeDecimal 断言两者的差不超过 tol：分摊比例有末位截断，单行份额允许极小误差。
func closeDecimal(t *testing.T, label string, got money.Decimal, want, tol string) {
	t.Helper()
	diff := got.Sub(money.MustParse(want)).Abs()
	if diff.Cmp(money.MustParse(tol)) > 0 {
		t.Errorf("%s: 期望 %s ±%s，得到 %s", label, want, tol, got)
	}
}

// 订单额按下单日、毛利按出货日：同一笔生意跨月时，两个月各只出现在自己该出现的那一边。
func TestSalesDashboardSplitsOrderDateAndShipDate(t *testing.T) {
	now := time.Date(2026, 10, 28, 9, 0, 0, 0, time.Local)
	f := newSalesFixture(t, now)

	cus := f.addPartner(t, "CUS001", "Ferreteria El Sol", "Ecuador")
	item := f.addOrder(t, cus, time.Date(2026, 8, 15, 0, 0, 0, 0, time.Local),
		[]salesItemSpec{{productID: 1, sku: "BH01-0001", name: "合页", qty: 100, amount: 7100}})[0]
	f.addPurchase(t, item, 100, 5000)
	f.addShipment(t, time.Date(2026, 9, 5, 0, 0, 0, 0, time.Local),
		[]allocSpec{{salesItemID: item, qty: 100}})

	sep := f.dashboard(t, dto.SalesDashboardQuery{
		Dimension: "month", From: "2026-09-01", To: "2026-09-30",
	})
	if len(sep.Rows) != 1 || sep.Rows[0].Key != "2026-09" {
		t.Fatalf("9 月应只有一行 2026-09，得到 %+v", sep.Rows)
	}
	wantDecimal(t, "9 月收入", sep.Rows[0].Revenue, "7100")
	wantDecimal(t, "9 月订单额（8 月下的单，不该算进来）", sep.Rows[0].OrderAmount, "0")

	aug := f.dashboard(t, dto.SalesDashboardQuery{
		Dimension: "month", From: "2026-08-01", To: "2026-08-31",
	})
	if len(aug.Rows) != 1 || aug.Rows[0].Key != "2026-08" {
		t.Fatalf("8 月应只有一行 2026-08，得到 %+v", aug.Rows)
	}
	wantDecimal(t, "8 月订单额", aug.Rows[0].OrderAmount, "7100")
	wantDecimal(t, "8 月收入（9 月才出货，不该算进来）", aug.Rows[0].Revenue, "0")
}

// limit 截断的是展示，不是口径：合计仍要覆盖全部行，否则"前 20 名加起来"会被当成全公司。
func TestSalesDashboardTotalSurvivesTruncation(t *testing.T) {
	now := time.Date(2026, 10, 28, 9, 0, 0, 0, time.Local)
	f := newSalesFixture(t, now)

	for i := 1; i <= 3; i++ {
		cus := f.addPartner(t, fmt.Sprintf("CUS00%d", i), fmt.Sprintf("客户 %d", i), "Ecuador")
		item := f.addOrder(t, cus, now, []salesItemSpec{
			{productID: uint(i), sku: "SKU-1", name: "测试品", qty: 10, amount: float64(100 * i)},
		})[0]
		f.addShipment(t, now, []allocSpec{{salesItemID: item, qty: 10}})
	}

	one := f.dashboard(t, dto.SalesDashboardQuery{Dimension: "customer", Limit: intPtr(1)})
	if len(one.Rows) != 1 {
		t.Fatalf("limit=1 应只返回 1 行，得到 %d", len(one.Rows))
	}
	if !one.Truncated {
		t.Error("被截断时应标 truncated")
	}
	wantDecimal(t, "截断后的合计", one.Total.Revenue, "600")

	all := f.dashboard(t, dto.SalesDashboardQuery{Dimension: "customer"})
	if len(all.Rows) != 3 || all.Truncated {
		t.Fatalf("默认应返回全部 3 行且不标截断，得到 %d 行 truncated=%v", len(all.Rows), all.Truncated)
	}
	if one.Total.Revenue.Cmp(all.Total.Revenue) != 0 {
		t.Errorf("合计随 limit 变化了：%s vs %s", one.Total.Revenue, all.Total.Revenue)
	}
}

// 没有采购记录的票，成本算不出来：标 cost_missing，而不是把"没录采购"报成"毛利极高"。
func TestSalesDashboardMarksMissingCost(t *testing.T) {
	now := time.Date(2026, 10, 28, 9, 0, 0, 0, time.Local)
	f := newSalesFixture(t, now)

	cus := f.addPartner(t, "CUS001", "Ferreteria El Sol", "Ecuador")
	item := f.addOrder(t, cus, now, []salesItemSpec{
		{productID: 1, sku: "BH01-0001", name: "合页", qty: 100, amount: 7100},
	})[0]
	f.addShipment(t, now, []allocSpec{{salesItemID: item, qty: 100}})

	res := f.dashboard(t, dto.SalesDashboardQuery{})
	if len(res.Rows) != 1 {
		t.Fatalf("应有 1 行，得到 %d", len(res.Rows))
	}
	if !res.Rows[0].CostMissing || !res.Total.CostMissing {
		t.Error("没有采购记录时应标 cost_missing")
	}
	wantDecimal(t, "无采购时成本", res.Rows[0].Cost, "0")
}

func TestSalesDashboardRejects(t *testing.T) {
	now := time.Date(2026, 10, 28, 9, 0, 0, 0, time.Local)
	f := newSalesFixture(t, now)

	cases := []struct {
		name string
		q    dto.SalesDashboardQuery
	}{
		{"业务员维度还没数据源", dto.SalesDashboardQuery{Dimension: "salesperson"}},
		{"未知维度", dto.SalesDashboardQuery{Dimension: "region"}},
		{"起始晚于结束", dto.SalesDashboardQuery{From: "2026-10-31", To: "2026-10-01"}},
		{"日期格式不对", dto.SalesDashboardQuery{From: "2026/10/01"}},
		{"limit 为负", dto.SalesDashboardQuery{Limit: intPtr(-1)}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := f.reports.SalesDashboard(context.Background(), c.q)
			wantCode(t, err, errs.CodeInvalidParam, c.name)
		})
	}
}
