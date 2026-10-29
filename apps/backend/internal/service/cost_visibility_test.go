package service

import (
	"context"
	"testing"
	"time"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/repository"
)

// T-36 的验收点是"隐藏发生在服务端响应里"：所以这里断言的是**返回体**，
// 而不是前端渲染结果。业务员拿到的成本与毛利必须是 0，且带 cost_hidden 标记——
// 没有标记的话，被清成 0 与"真的 0"在界面上分不开。
func TestCostHiddenForStaff(t *testing.T) {
	now := time.Date(2026, 10, 28, 9, 0, 0, 0, time.Local)
	f := newSalesFixture(t, now)

	cus := f.addPartner(t, "CUS001", "Ferreteria El Sol", "Ecuador")
	hinge := f.addProduct(t, "BH01-0001", "合页", "BH")
	// 产品成本价：采购 100 件 2000，单位成本 20。
	if err := f.db.Model(&model.Product{}).Where("id = ?", hinge).
		Update("cost_price", dec(20)).Error; err != nil {
		t.Fatalf("写成本价失败: %v", err)
	}

	item := f.addOrder(t, cus, now, []salesItemSpec{
		{productID: hinge, sku: "BH01-0001", name: "合页", qty: 100, amount: 3000},
	})[0]
	f.addPurchase(t, item, 100, 2000)
	s1 := f.addShipment(t, now, []allocSpec{{salesItemID: item, qty: 100}})
	f.addCharge(t, s1, 100)

	staffCtx := WithActor(context.Background(), scopeActor(1, model.RoleStaff))
	bossCtx := WithActor(context.Background(), scopeActor(2, model.RoleManager))

	t.Run("产品详情", func(t *testing.T) {
		products := NewProductService(
			repository.NewProductRepository(f.db),
			repository.NewPartnerRepository(f.db),
			repository.NewProductSourceRepository(f.db),
			repository.NewProductCodeRepository(f.db),
			repository.NewTxManager(f.db),
		)

		visible, err := products.Get(bossCtx, hinge)
		if err != nil {
			t.Fatalf("主管查产品失败: %v", err)
		}
		wantDecimal(t, "主管看得到成本价", visible.CostPrice, "20")
		if visible.CostHidden {
			t.Error("主管不该带 cost_hidden")
		}

		hidden, err := products.Get(staffCtx, hinge)
		if err != nil {
			t.Fatalf("业务员查产品失败: %v", err)
		}
		wantDecimal(t, "业务员看到的成本价", hidden.CostPrice, "0")
		if !hidden.CostHidden {
			t.Error("业务员的响应必须带 cost_hidden，否则 0 与真的 0 分不开")
		}
	})

	t.Run("单票毛利", func(t *testing.T) {
		res, err := f.reports.ShipmentProfits(staffCtx, dto.ShipmentProfitQuery{})
		if err != nil {
			t.Fatalf("查单票毛利失败: %v", err)
		}
		if len(res.List) != 1 {
			t.Fatalf("应有 1 票，得到 %d", len(res.List))
		}
		row := res.List[0]
		// 收入照给：业务员要知道自己卖了多少。
		wantDecimal(t, "收入仍可见", row.Revenue, "3000")
		wantDecimal(t, "成本被清", row.Cost, "0")
		wantDecimal(t, "运杂费被清", row.Charge, "0")
		wantDecimal(t, "毛利被清", row.Profit, "0")
		wantDecimal(t, "毛利率被清", row.Margin, "0")
		if !row.CostHidden {
			t.Error("业务员的响应必须带 cost_hidden")
		}

		bossRes, err := f.reports.ShipmentProfits(bossCtx, dto.ShipmentProfitQuery{})
		if err != nil {
			t.Fatalf("主管查单票毛利失败: %v", err)
		}
		wantDecimal(t, "主管看到的成本", bossRes.List[0].Cost, "2000")
		wantDecimal(t, "主管看到的毛利", bossRes.List[0].Profit, "900")
	})

	t.Run("销售看板", func(t *testing.T) {
		res, err := f.reports.SalesDashboard(staffCtx, dto.SalesDashboardQuery{})
		if err != nil {
			t.Fatalf("查看板失败: %v", err)
		}
		if len(res.Rows) != 1 {
			t.Fatalf("应有 1 行，得到 %d", len(res.Rows))
		}
		row := res.Rows[0]
		wantDecimal(t, "订单额仍可见", row.OrderAmount, "3000")
		wantDecimal(t, "收入仍可见", row.Revenue, "3000")
		wantDecimal(t, "看板毛利被清", row.Profit, "0")
		if !row.CostHidden || !res.Total.CostHidden {
			t.Error("行与合计都要带 cost_hidden")
		}
	})

	t.Run("产品毛利下钻", func(t *testing.T) {
		res, err := f.reports.ProductProfit(staffCtx, dto.ProductProfitQuery{ProductID: hinge})
		if err != nil {
			t.Fatalf("查下钻失败: %v", err)
		}
		wantDecimal(t, "下钻收入仍可见", res.Revenue, "3000")
		wantDecimal(t, "下钻毛利被清", res.Profit, "0")
		wantDecimal(t, "下钻单票成本被清", res.Rows[0].Cost, "0")
		if !res.CostHidden {
			t.Error("下钻结果要带 cost_hidden")
		}
	})
}

// 没有身份的内部调用不做隐藏：那些调用是系统自己在算，
// 把它们当成业务员会让内部计算拿到一堆 0。
func TestCostVisibleWithoutActor(t *testing.T) {
	if !canSeeCost(context.Background()) {
		t.Fatal("无身份的调用应当放行")
	}
	staffCtx := WithActor(context.Background(), scopeActor(1, model.RoleStaff))
	if canSeeCost(staffCtx) {
		t.Fatal("业务员不该看到成本")
	}
	if !canSeeCost(WithActor(context.Background(), scopeActor(1, model.RoleManager))) {
		t.Fatal("主管应当看到成本")
	}
	// 认不出的角色按业务员处理：与前缀写错不该提权是同一条规则。
	if canSeeCost(WithActor(context.Background(), scopeActor(1, "mgr"))) {
		t.Fatal("未知角色不该看到成本")
	}
}
