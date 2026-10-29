package service

import (
	"context"
	"fmt"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/money"
	"github.com/edwinhuish/gowork/internal/repository"
)

type profitFixture struct {
	reports ReportService
	db      *gorm.DB
	now     time.Time
	seq     int
}

// newProfitFixture 造一套单票毛利的原料：销售行、采购行、出货分配、运杂费。
//
// 全部直接写库：毛利的口径是"怎么把已有的数拼起来"，而销售行与采购行的
// 金额怎么产生，T-10/T-11 已经各自测过。这里要能精确控制数量与金额。
func newProfitFixture(t *testing.T, now time.Time) profitFixture {
	t.Helper()

	db := openTestDB(t,
		&model.Partner{}, &model.SalesOrder{}, &model.SalesOrderItem{},
		&model.PurchaseOrder{}, &model.PurchaseOrderItem{},
		&model.Shipment{}, &model.ShipmentAllocation{}, &model.ShipmentCharge{})

	return profitFixture{
		reports: NewReportService(
			repository.NewReportRepository(db),
			WithReportClock(func() time.Time { return now })),
		db:  db,
		now: now,
	}
}

// addSalesLine 建一张销售订单与它的一行明细，返回明细行 id。
func (f *profitFixture) addSalesLine(
	t *testing.T, qty, unitPrice, amountBase float64,
) uint {
	t.Helper()
	f.seq++
	order := &model.SalesOrder{
		DocNo: fmt.Sprintf("SO-PROFIT-%d", f.seq), PartnerID: 1, TradeTerm: model.TradeTermFOB,
		Status: model.DocumentStatusConfirmed, OrderDate: model.DateOnly(f.now),
		CurrencyCode: "USD", ExchangeRate: money.MustParse("7.1"),
		AmountTotal: money.FromFloat(qty * unitPrice), AmountBaseTotal: money.FromFloat(amountBase),
	}
	if err := f.db.Create(order).Error; err != nil {
		t.Fatalf("建销售订单失败: %v", err)
	}
	item := &model.SalesOrderItem{
		SalesOrderID: order.ID, ProductID: 1,
		SKUCode: "SKU-1", NameCN: "测试品", Unit: "PCS",
		Qty:        qtyOf(qty),
		UnitPrice:  money.FromFloat(unitPrice),
		Amount:     money.FromFloat(qty * unitPrice),
		AmountBase: money.FromFloat(amountBase),
	}
	if err := f.db.Create(item).Error; err != nil {
		t.Fatalf("建销售明细失败: %v", err)
	}
	return item.ID
}

// addPurchase 为某条销售行建一张采购订单与一行明细。
func (f *profitFixture) addPurchase(
	t *testing.T, salesItemID uint, qty, amountBase float64, status model.DocumentStatus,
) {
	t.Helper()
	f.seq++
	order := &model.PurchaseOrder{
		DocNo: fmt.Sprintf("PO-PROFIT-%d", f.seq), SupplierID: 2, TradeTerm: model.TradeTermEXW,
		Status: status, OrderDate: model.DateOnly(f.now),
		CurrencyCode: "CNY", ExchangeRate: money.FromInt(1),
		AmountTotal: money.FromFloat(amountBase), AmountBaseTotal: money.FromFloat(amountBase),
	}
	if err := f.db.Create(order).Error; err != nil {
		t.Fatalf("建采购订单失败: %v", err)
	}
	itemID := salesItemID
	item := &model.PurchaseOrderItem{
		PurchaseOrderID: order.ID, ProductID: 1,
		SKUCode: "SKU-1", NameCN: "测试品", Unit: "PCS",
		Qty:              qtyOf(qty),
		UnitPrice:        money.FromFloat(amountBase / qty),
		Amount:           money.FromFloat(amountBase),
		AmountBase:       money.FromFloat(amountBase),
		SalesOrderItemID: &itemID,
	}
	if err := f.db.Create(item).Error; err != nil {
		t.Fatalf("建采购明细失败: %v", err)
	}
}

// addShipment 建一票货并挂一行分配。
func (f *profitFixture) addShipment(
	t *testing.T, status model.DocumentStatus, shipDate time.Time, salesItemID uint, qty float64,
) uint {
	t.Helper()
	f.seq++
	m := &model.Shipment{
		DocNo: fmt.Sprintf("SH-PROFIT-%d", f.seq), Status: status, ShipDate: model.DateOnly(shipDate),
	}
	if err := f.db.Create(m).Error; err != nil {
		t.Fatalf("建出货批次失败: %v", err)
	}
	alloc := &model.ShipmentAllocation{
		ShipmentID: m.ID, SalesOrderItemID: salesItemID,
		SKUCode: "SKU-1", NameCN: "测试品", Unit: "PCS", Qty: qtyOf(qty),
	}
	if err := f.db.Create(alloc).Error; err != nil {
		t.Fatalf("建分配行失败: %v", err)
	}
	return m.ID
}

func (f *profitFixture) addCharge(t *testing.T, shipmentID uint, amountBase float64) {
	t.Helper()
	m := &model.ShipmentCharge{
		ShipmentID: shipmentID, Type: model.ShipmentChargeFreight,
		CurrencyCode: "CNY", ExchangeRate: money.FromInt(1),
		Amount: money.FromFloat(amountBase), AmountBase: money.FromFloat(amountBase),
	}
	if err := f.db.Create(m).Error; err != nil {
		t.Fatalf("建运费杂费失败: %v", err)
	}
}

func (f *profitFixture) profits(t *testing.T, q dto.ShipmentProfitQuery) []dto.ShipmentProfit {
	t.Helper()
	res, err := f.reports.ShipmentProfits(context.Background(), q)
	if err != nil {
		t.Fatalf("查询单票毛利失败: %v", err)
	}
	return res.List
}

// TestShipmentProfitFullLine 是 T-22 的验收点：毛利 = 收入 − 采购 − 运杂费。
// 收入 100 × 71 = 7100（销售行本币），成本 5000，费用 500，毛利 1600。
func TestShipmentProfitFullLine(t *testing.T) {
	now := time.Date(2026, 10, 28, 9, 0, 0, 0, time.Local)
	f := newProfitFixture(t, now)

	itemID := f.addSalesLine(t, 100, 71, 7100)
	f.addPurchase(t, itemID, 100, 5000, model.DocumentStatusConfirmed)
	shipmentID := f.addShipment(t, model.DocumentStatusShipped, now, itemID, 100)
	f.addCharge(t, shipmentID, 500)

	list := f.profits(t, dto.ShipmentProfitQuery{})
	if len(list) != 1 {
		t.Fatalf("应有 1 票，得到 %d", len(list))
	}
	p := list[0]
	if p.ShipmentID != shipmentID {
		t.Errorf("票 id 应为 %d，得到 %d", shipmentID, p.ShipmentID)
	}
	wantDecimal(t, "收入", p.Revenue, "7100")
	wantDecimal(t, "采购成本", p.Cost, "5000")
	wantDecimal(t, "运杂费", p.Charge, "500")
	wantDecimal(t, "毛利", p.Profit, "1600")
	wantDecimal(t, "毛利率", p.Margin, "0.2254")
	if p.CostMissing {
		t.Error("有采购记录时不该标 cost_missing")
	}
	if p.ShipDate != "2026-10-28" {
		t.Errorf("出货日应为 2026-10-28，得到 %s", p.ShipDate)
	}
}

// TestShipmentProfitPartialShipment：一票只出了 40 件，收入与成本都按 40/100 分摊，
// 两票之和必须回到整行的数——这才是"分批出货不重复计成本"。
func TestShipmentProfitPartialShipment(t *testing.T) {
	now := time.Date(2026, 10, 28, 9, 0, 0, 0, time.Local)
	f := newProfitFixture(t, now)

	itemID := f.addSalesLine(t, 100, 71, 7100)
	f.addPurchase(t, itemID, 100, 5000, model.DocumentStatusConfirmed)
	first := f.addShipment(t, model.DocumentStatusShipped, now, itemID, 40)
	second := f.addShipment(t, model.DocumentStatusShipped, now, itemID, 60)

	list := f.profits(t, dto.ShipmentProfitQuery{})
	if len(list) != 2 {
		t.Fatalf("应有 2 票，得到 %d", len(list))
	}

	byID := map[uint]dto.ShipmentProfit{}
	for _, p := range list {
		byID[p.ShipmentID] = p
	}
	wantDecimal(t, "第一票收入", byID[first].Revenue, "2840")
	wantDecimal(t, "第一票成本", byID[first].Cost, "2000")
	wantDecimal(t, "第二票收入", byID[second].Revenue, "4260")
	wantDecimal(t, "第二票成本", byID[second].Cost, "3000")

	totalProfit := byID[first].Profit.Add(byID[second].Profit)
	wantDecimal(t, "两票毛利之和", totalProfit, "2100")
}

// TestShipmentProfitMissingPurchase：没挂采购的票，成本是"算不出来"而不是 0。
// 标出 cost_missing 才能让人去补采购单，而不是把 7100 全当毛利。
func TestShipmentProfitMissingPurchase(t *testing.T) {
	now := time.Date(2026, 10, 28, 9, 0, 0, 0, time.Local)
	f := newProfitFixture(t, now)

	itemID := f.addSalesLine(t, 100, 71, 7100)
	shipmentID := f.addShipment(t, model.DocumentStatusShipped, now, itemID, 100)
	f.addCharge(t, shipmentID, 500)

	p := f.profits(t, dto.ShipmentProfitQuery{})[0]
	wantDecimal(t, "收入", p.Revenue, "7100")
	wantDecimal(t, "成本", p.Cost, "0")
	if !p.CostMissing {
		t.Error("没有采购记录时应标 cost_missing")
	}
	wantDecimal(t, "毛利（缺成本时只减了费用）", p.Profit, "6600")
}

// TestShipmentProfitSkipsDraftShipmentAndDraftPurchase：
// 草稿出货不进统计（数量还会改），草稿采购不算成本（还只是询价）。
func TestShipmentProfitSkipsDraftShipmentAndDraftPurchase(t *testing.T) {
	now := time.Date(2026, 10, 28, 9, 0, 0, 0, time.Local)
	f := newProfitFixture(t, now)

	itemID := f.addSalesLine(t, 100, 71, 7100)
	f.addShipment(t, model.DocumentStatusDraft, now, itemID, 100)

	if list := f.profits(t, dto.ShipmentProfitQuery{}); len(list) != 0 {
		t.Fatalf("草稿出货不该进统计，得到 %d 票", len(list))
	}

	// 换成已确认的票，但采购单还是草稿：成本算不出来。
	f.addPurchase(t, itemID, 100, 5000, model.DocumentStatusDraft)
	f.addShipment(t, model.DocumentStatusConfirmed, now, itemID, 100)

	p := f.profits(t, dto.ShipmentProfitQuery{})[0]
	wantDecimal(t, "草稿采购不该计入成本", p.Cost, "0")
	if !p.CostMissing {
		t.Error("只有草稿采购时应标 cost_missing")
	}
}

func TestShipmentProfitFilterByShipmentID(t *testing.T) {
	now := time.Date(2026, 10, 28, 9, 0, 0, 0, time.Local)
	f := newProfitFixture(t, now)

	itemA := f.addSalesLine(t, 10, 71, 710)
	itemB := f.addSalesLine(t, 10, 71, 710)
	f.addPurchase(t, itemA, 10, 500, model.DocumentStatusConfirmed)
	f.addPurchase(t, itemB, 10, 500, model.DocumentStatusConfirmed)
	f.addShipment(t, model.DocumentStatusShipped, now, itemA, 10)
	second := f.addShipment(t, model.DocumentStatusShipped, now.AddDate(0, 0, 1), itemB, 10)

	id := second
	res, err := f.reports.ShipmentProfits(context.Background(), dto.ShipmentProfitQuery{
		ShipmentID: &id,
	})
	if err != nil {
		t.Fatalf("按票筛选失败: %v", err)
	}
	if res.Total != 1 || len(res.List) != 1 || res.List[0].ShipmentID != second {
		t.Fatalf("按票筛选应只回一票，得到 total=%d list=%+v", res.Total, res.List)
	}
}
