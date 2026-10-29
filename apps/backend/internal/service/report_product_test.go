package service

import (
	"context"
	"testing"
	"time"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/pkg/errs"
)

// 品类是产品的归类，不是成交条件：报表按产品**当前**的归类算，
// 所以没归类的产品必须落在「未分类」里，而不是从表上消失。
func TestSalesDashboardCategoryDimension(t *testing.T) {
	now := time.Date(2026, 10, 28, 9, 0, 0, 0, time.Local)
	f := newSalesFixture(t, now)

	cus := f.addPartner(t, "CUS001", "Ferreteria El Sol", "Ecuador")
	hinge := f.addProduct(t, "BH01-0001", "合页", "BH")
	lock := f.addProduct(t, "HT02-0007", "锁体", "") // 没归类

	itemA := f.addOrder(t, cus, now, []salesItemSpec{
		{productID: hinge, sku: "BH01-0001", name: "合页", qty: 100, amount: 3000},
	})[0]
	f.addPurchase(t, itemA, 100, 2000)
	s1 := f.addShipment(t, now, []allocSpec{{salesItemID: itemA, qty: 100}})
	f.addCharge(t, s1, 100)

	itemB := f.addOrder(t, cus, now, []salesItemSpec{
		{productID: lock, sku: "HT02-0007", name: "锁体", qty: 50, amount: 1000},
	})[0]
	f.addPurchase(t, itemB, 50, 600)
	s2 := f.addShipment(t, now, []allocSpec{{salesItemID: itemB, qty: 50}})
	f.addCharge(t, s2, 50)

	res := f.dashboard(t, dto.SalesDashboardQuery{Dimension: "category"})
	if len(res.Rows) != 2 {
		t.Fatalf("应有两行（BH + 未分类），得到 %+v", res.Rows)
	}
	byKey := map[string]int{}
	for i, row := range res.Rows {
		byKey[row.Label] = i
	}
	if _, ok := byKey["BH"]; !ok {
		t.Errorf("应有 BH 品类，得到 %+v", res.Rows)
	}
	if _, ok := byKey["未分类"]; !ok {
		t.Errorf("没归类的产品应落在「未分类」，得到 %+v", res.Rows)
	}

	// 换维度不改合计：这是看板的根本承诺。
	product := f.dashboard(t, dto.SalesDashboardQuery{Dimension: "product"})
	if res.Total.Revenue.Cmp(product.Total.Revenue) != 0 ||
		res.Total.Profit.Cmp(product.Total.Profit) != 0 {
		t.Errorf("品类与产品的合计不一致: %s/%s 毛利 %s/%s",
			res.Total.Revenue, product.Total.Revenue, res.Total.Profit, product.Total.Profit)
	}
}

// 下钻要能拆到票，且合计与看板上那个产品行对得上——两个页面算出不同毛利，
// 看报表的人只会得出"系统不可信"。
func TestProductProfitDrillDown(t *testing.T) {
	now := time.Date(2026, 10, 28, 9, 0, 0, 0, time.Local)
	f := newSalesFixture(t, now)

	cus := f.addPartner(t, "CUS001", "Ferreteria El Sol", "Ecuador")
	hinge := f.addProduct(t, "BH01-0001", "合页", "BH")

	// 同一个产品分两票出：一张订单两行、两票各出一行。
	items := f.addOrder(t, cus, now, []salesItemSpec{
		{productID: hinge, sku: "BH01-0001", name: "合页", qty: 100, amount: 3000},
		{productID: hinge, sku: "BH01-0001", name: "合页", qty: 100, amount: 3000},
	})
	f.addPurchase(t, items[0], 100, 2000)
	f.addPurchase(t, items[1], 100, 2000)

	s1 := f.addShipment(t, now.AddDate(0, 0, -3), []allocSpec{{salesItemID: items[0], qty: 100}})
	f.addCharge(t, s1, 100)
	s2 := f.addShipment(t, now, []allocSpec{{salesItemID: items[1], qty: 100}})
	f.addCharge(t, s2, 100)

	res, err := f.reports.ProductProfit(context.Background(), dto.ProductProfitQuery{
		ProductID: hinge,
	})
	if err != nil {
		t.Fatalf("查下钻失败: %v", err)
	}
	if len(res.Rows) != 2 {
		t.Fatalf("应有两票，得到 %d", len(res.Rows))
	}
	// 最近出的货排前面。
	if res.Rows[0].ShipmentID != s2 {
		t.Errorf("应按出货日倒序，第一行是 %d，期望 %d", res.Rows[0].ShipmentID, s2)
	}
	wantDecimal(t, "单票收入", res.Rows[0].Revenue, "3000")
	wantDecimal(t, "单票成本", res.Rows[0].Cost, "2000")
	wantDecimal(t, "单票分摊运杂费", res.Rows[0].Charge, "100")
	wantDecimal(t, "单票毛利", res.Rows[0].Profit, "900")

	wantDecimal(t, "合计收入", res.Revenue, "6000")
	wantDecimal(t, "合计毛利", res.Profit, "1800")

	// 与看板的产品行对齐：同一口径下两处必须相等。
	dash := f.dashboard(t, dto.SalesDashboardQuery{Dimension: "product"})
	for _, row := range dash.Rows {
		if row.Key == "1" || row.Label == "BH01-0001 · 合页" {
			if row.Revenue.Cmp(res.Revenue) != 0 || row.Profit.Cmp(res.Profit) != 0 {
				t.Errorf("下钻合计与看板不一致: 收入 %s/%s 毛利 %s/%s",
					res.Revenue, row.Revenue, res.Profit, row.Profit)
			}
		}
	}
}

func TestProductProfitRejects(t *testing.T) {
	now := time.Date(2026, 10, 28, 9, 0, 0, 0, time.Local)
	f := newSalesFixture(t, now)

	_, err := f.reports.ProductProfit(context.Background(), dto.ProductProfitQuery{})
	wantCode(t, err, errs.CodeInvalidParam, "没指定产品")

	_, err = f.reports.ProductProfit(context.Background(),
		dto.ProductProfitQuery{ProductID: 1, From: "2026/10/01"})
	wantCode(t, err, errs.CodeInvalidParam, "日期格式")
}
