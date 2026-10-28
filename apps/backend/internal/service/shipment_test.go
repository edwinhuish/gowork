package service

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/errs"
	"github.com/edwinhuish/gowork/internal/repository"
)

type shipmentFixture struct {
	shipments ShipmentService
	orders    SalesOrderService
	now       *time.Time
	// orderA 一行 100 件，orderB 一行 20 件，两张单都已确认（草稿不能出货）。
	orderA *model.SalesOrder
	orderB *model.SalesOrder
	// draftOrder 停在草稿状态，用来验证"草稿订单不能出货"。
	draftOrder *model.SalesOrder
}

func newShipmentFixture(t *testing.T, now *time.Time) shipmentFixture {
	t.Helper()
	ctx := context.Background()

	db := openTestDB(t,
		&model.Currency{}, &model.ExchangeRate{}, &model.Partner{}, &model.Product{},
		&model.SalesOrder{}, &model.SalesOrderItem{}, &model.DocSeq{},
		&model.Shipment{}, &model.ShipmentAllocation{}, &model.ShipmentCharge{})

	clock := func() time.Time { return *now }
	currencyRepo := repository.NewCurrencyRepository(db)
	currencySvc := NewCurrencyService(currencyRepo, repository.NewExchangeRateRepository(db))
	if _, err := currencySvc.Create(ctx, dto.CurrencyInput{
		Code: "CNY", Name: "人民币", IsBase: boolPtr(true),
	}); err != nil {
		t.Fatalf("建本币失败: %v", err)
	}

	partnerRepo := repository.NewPartnerRepository(db)
	customer := &model.Partner{
		Code: "C001", Name: "Ferreteria El Sol", Role: model.PartnerRoleCustomer,
		EntityType: model.PartnerEntityCompany, Status: model.EnableStatusOn,
	}
	if err := partnerRepo.Create(ctx, customer); err != nil {
		t.Fatalf("建主体失败: %v", err)
	}

	productRepo := repository.NewProductRepository(db)
	p1 := &model.Product{
		SKUCode: "BH01-0001", NameCN: "不锈钢合页", Spec: "100×75×3", Unit: "PCS",
		Status: model.EnableStatusOn,
	}
	p2 := &model.Product{
		SKUCode: "BH01-0002", NameCN: "合页（彩盒装）", Unit: "PCS", Status: model.EnableStatusOn,
	}
	for _, p := range []*model.Product{p1, p2} {
		if err := productRepo.Create(ctx, p); err != nil {
			t.Fatalf("建产品失败: %v", err)
		}
	}

	orderRepo := repository.NewSalesOrderRepository(db)
	shipmentRepo := repository.NewShipmentRepository(db)
	docNoSvc := NewDocNoService(repository.NewDocSeqRepository(db), WithDocNoClock(clock))
	orderSvc := NewSalesOrderService(
		orderRepo, partnerRepo, productRepo, shipmentRepo, currencySvc, docNoSvc,
		WithSalesOrderClock(clock),
	)

	createOrder := func(productID uint, qty float64) *model.SalesOrder {
		o, err := orderSvc.Create(ctx, dto.SalesOrderInput{
			PartnerID:    customer.ID,
			TradeTerm:    "FOB",
			OrderDate:    now.Format("2006-01-02"),
			CurrencyCode: "CNY",
			Items: []dto.SalesOrderItemInput{
				{ProductID: productID, Qty: qty, UnitPrice: 10},
			},
		})
		if err != nil {
			t.Fatalf("建订单失败: %v", err)
		}
		return o
	}
	confirm := func(o *model.SalesOrder) *model.SalesOrder {
		confirmed, err := orderSvc.UpdateStatus(ctx, o.ID, dto.SalesOrderStatusInput{
			Status: int8(model.DocumentStatusConfirmed),
		})
		if err != nil {
			t.Fatalf("确认订单失败: %v", err)
		}
		return confirmed
	}

	orderA := confirm(createOrder(p1.ID, 100))
	orderB := confirm(createOrder(p2.ID, 20))
	draftOrder := createOrder(p1.ID, 50)

	return shipmentFixture{
		shipments: NewShipmentService(
			shipmentRepo, orderRepo, repository.NewShipmentChargeRepository(db), docNoSvc,
			WithShipmentClock(clock),
			WithShipmentLogger(slog.New(slog.DiscardHandler)),
		),
		orders:     orderSvc,
		now:        now,
		orderA:     orderA,
		orderB:     orderB,
		draftOrder: draftOrder,
	}
}

func TestShipmentCandidates(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 10, 28, 10, 0, 0, 0, time.Local)
	fx := newShipmentFixture(t, &now)

	itemA := fx.orderA.Items[0]
	findA := func(t *testing.T) (dto.ShipmentCandidate, bool) {
		t.Helper()
		list, err := fx.shipments.Candidates(ctx, "", 0)
		if err != nil {
			t.Fatalf("查可出货行失败: %v", err)
		}
		for _, c := range list {
			if c.SalesOrderItemID == itemA.ID {
				return c, true
			}
		}
		return dto.ShipmentCandidate{}, false
	}

	t.Run("只列已确认且还有未出数量的行", func(t *testing.T) {
		list, err := fx.shipments.Candidates(ctx, "", 0)
		if err != nil {
			t.Fatalf("查可出货行失败: %v", err)
		}
		// orderA 100 + orderB 20；draftOrder 还在草稿，不该出现。
		if len(list) != 2 {
			t.Fatalf("可出货行数 = %d，期望 2（草稿订单不进候选）", len(list))
		}
		c, ok := findA(t)
		if !ok {
			t.Fatal("未列到 orderA 的行")
		}
		if !approx(c.UnshippedQty, 100) || !approx(c.ShippedQty, 0) {
			t.Errorf("未出 / 已出 = %v / %v，期望 100 / 0", c.UnshippedQty, c.ShippedQty)
		}
		if c.SalesOrderDocNo != fx.orderA.DocNo || c.SKUCode != "BH01-0001" {
			t.Errorf("候选行缺订单号或 SKU 快照: %+v", c)
		}
		if c.DeliveryDate != "" {
			t.Errorf("没约定交期时不该编一个日期，实际 %q", c.DeliveryDate)
		}
	})

	t.Run("已出数量随出货单变化", func(t *testing.T) {
		s, err := fx.shipments.Create(ctx, dto.ShipmentInput{
			Allocations: []dto.ShipmentAllocationInput{{SalesOrderItemID: itemA.ID, Qty: 60}},
		})
		if err != nil {
			t.Fatalf("建出货批次失败: %v", err)
		}
		c, ok := findA(t)
		if !ok {
			t.Fatal("出了一部分的行仍应留在候选里")
		}
		if !approx(c.ShippedQty, 60) || !approx(c.UnshippedQty, 40) {
			t.Errorf("已出 / 未出 = %v / %v，期望 60 / 40", c.ShippedQty, c.UnshippedQty)
		}

		// 出完就该从候选里消失，而不是留一个"还能出 0"的选项。
		if _, err := fx.shipments.Create(ctx, dto.ShipmentInput{
			Allocations: []dto.ShipmentAllocationInput{{SalesOrderItemID: itemA.ID, Qty: 40}},
		}); err != nil {
			t.Fatalf("建出货批次失败: %v", err)
		}
		if _, ok := findA(t); ok {
			t.Error("已出完的行不该还出现在候选里")
		}

		// 取消一票后数量归还，候选里应重新看到这一行。
		if _, err := fx.shipments.UpdateStatus(ctx, s.ID, dto.ShipmentStatusInput{
			Status: int8(model.DocumentStatusCancelled),
		}); err != nil {
			t.Fatalf("取消出货单失败: %v", err)
		}
		c, ok = findA(t)
		if !ok {
			t.Fatal("取消后这一行应重新可出")
		}
		if !approx(c.UnshippedQty, 60) {
			t.Errorf("取消后未出 = %v，期望 60", c.UnshippedQty)
		}
	})

	t.Run("关键字可按订单号或 SKU 搜", func(t *testing.T) {
		list, err := fx.shipments.Candidates(ctx, fx.orderA.DocNo, 0)
		if err != nil {
			t.Fatalf("查可出货行失败: %v", err)
		}
		if len(list) != 1 || list[0].SalesOrderItemID != itemA.ID {
			t.Fatalf("按订单号搜到的候选 = %+v", list)
		}
		list, err = fx.shipments.Candidates(ctx, "BH01-0002", 0)
		if err != nil {
			t.Fatalf("查可出货行失败: %v", err)
		}
		if len(list) != 1 || list[0].SKUCode != "BH01-0002" {
			t.Fatalf("按 SKU 搜到的候选 = %+v", list)
		}
	})
}

func TestShipmentRules(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 10, 28, 10, 0, 0, 0, time.Local)
	fx := newShipmentFixture(t, &now)

	itemA := fx.orderA.Items[0]
	itemB := fx.orderB.Items[0]

	// 一票装两张订单的行：外贸拼柜是常态，所以第一步就按合并出货建。
	var first *model.Shipment
	t.Run("一票可装多张订单的行", func(t *testing.T) {
		s, err := fx.shipments.Create(ctx, dto.ShipmentInput{
			Remark: "拼柜第一票",
			Allocations: []dto.ShipmentAllocationInput{
				{SalesOrderItemID: itemA.ID, Qty: 60},
				{SalesOrderItemID: itemB.ID, Qty: 20},
			},
		})
		if err != nil {
			t.Fatalf("建出货批次失败: %v", err)
		}
		first = s

		if s.DocNo != "SH-202610-0001" {
			t.Errorf("doc_no = %q，期望 SH-202610-0001", s.DocNo)
		}
		if s.Status != model.DocumentStatusDraft {
			t.Errorf("新出货单状态 = %d，期望草稿", s.Status)
		}
		// 出货日留空取今天，且只取日期：它决定这票落在哪个统计月份。
		if !s.ShipDate.Equal(model.DateOnly(now)) {
			t.Errorf("ship_date = %v，期望 %v", s.ShipDate, model.DateOnly(now))
		}
		if len(s.Allocations) != 2 {
			t.Fatalf("分配行数 = %d，期望 2", len(s.Allocations))
		}
		// 产品快照：出货单要发给仓库与货代，产品改名后单上仍应是当时那个说法。
		if s.Allocations[0].SKUCode != "BH01-0001" || s.Allocations[0].NameCN != "不锈钢合页" {
			t.Errorf("分配行未存产品快照: %+v", s.Allocations[0])
		}
		if s.Allocations[0].SalesOrderItem == nil ||
			s.Allocations[0].SalesOrderItem.SalesOrder == nil ||
			s.Allocations[0].SalesOrderItem.SalesOrder.DocNo != fx.orderA.DocNo {
			t.Errorf("分配行未带出来源订单: %+v", s.Allocations[0].SalesOrderItem)
		}
	})

	t.Run("超过未出数量被拒", func(t *testing.T) {
		// A 已出 60，剩 40；要 50 必须给 409 并把剩余量说清楚。
		_, err := fx.shipments.Create(ctx, dto.ShipmentInput{
			Allocations: []dto.ShipmentAllocationInput{{SalesOrderItemID: itemA.ID, Qty: 50}},
		})
		if e, ok := errs.From(err); !ok || e.Code != errs.CodeConflict {
			t.Fatalf("超出未出数量应返回 409，实际 %v", err)
		}
		if msg := err.Error(); !strings.Contains(msg, "最多还能出 40") {
			t.Errorf("提示未说明剩余量: %q", msg)
		}
	})

	t.Run("同一次请求拆两行也绕不过校验", func(t *testing.T) {
		// 40 是剩余量，两行各 25 合计 50：逐行看都没超，合起来就超了。
		_, err := fx.shipments.Create(ctx, dto.ShipmentInput{
			Allocations: []dto.ShipmentAllocationInput{
				{SalesOrderItemID: itemA.ID, Qty: 25},
				{SalesOrderItemID: itemA.ID, Qty: 25},
			},
		})
		if e, ok := errs.From(err); !ok || e.Code != errs.CodeConflict {
			t.Fatalf("同一行拆成两行应仍被拒，实际 %v", err)
		}
	})

	t.Run("恰好出完不算超量", func(t *testing.T) {
		s, err := fx.shipments.Create(ctx, dto.ShipmentInput{
			Allocations: []dto.ShipmentAllocationInput{{SalesOrderItemID: itemA.ID, Qty: 40}},
		})
		if err != nil {
			t.Fatalf("恰好等于剩余量应允许，实际 %v", err)
		}
		if s.DocNo != "SH-202610-0002" {
			t.Errorf("doc_no = %q，期望 SH-202610-0002", s.DocNo)
		}
		if _, err := fx.shipments.Create(ctx, dto.ShipmentInput{
			Allocations: []dto.ShipmentAllocationInput{{SalesOrderItemID: itemA.ID, Qty: 1}},
		}); err == nil {
			t.Fatal("已出完的行不该还能再出货")
		}
	})

	t.Run("编辑时排除自己已占的数量", func(t *testing.T) {
		// 第一票占 60、第二票占 40。把第一票改成 50 是可行的（合计 90），
		// 但这要求统计时先把第一票自己的 60 扣掉；否则永远判超量。
		s, err := fx.shipments.Update(ctx, first.ID, dto.ShipmentInput{
			Allocations: []dto.ShipmentAllocationInput{{SalesOrderItemID: itemA.ID, Qty: 50}},
		})
		if err != nil {
			t.Fatalf("编辑应排除自身已占数量，实际 %v", err)
		}
		if len(s.Allocations) != 1 || !approx(s.Allocations[0].Qty, 50) {
			t.Errorf("编辑后分配行 = %+v，期望一行 50", s.Allocations)
		}
		// 40（第二票）+ 70 > 100，仍然要拒。
		if _, err := fx.shipments.Update(ctx, first.ID, dto.ShipmentInput{
			Allocations: []dto.ShipmentAllocationInput{{SalesOrderItemID: itemA.ID, Qty: 70}},
		}); err == nil {
			t.Fatal("把数量改到超过总量应被拒")
		}
	})

	t.Run("已确认的出货单不可改不可删", func(t *testing.T) {
		if _, err := fx.shipments.UpdateStatus(ctx, first.ID, dto.ShipmentStatusInput{
			Status: int8(model.DocumentStatusConfirmed),
		}); err != nil {
			t.Fatalf("确认出货单失败: %v", err)
		}
		if _, err := fx.shipments.Update(ctx, first.ID, dto.ShipmentInput{
			Allocations: []dto.ShipmentAllocationInput{{SalesOrderItemID: itemA.ID, Qty: 10}},
		}); err == nil {
			t.Fatal("已确认的出货单不该允许修改")
		}
		if err := fx.shipments.Delete(ctx, first.ID); err == nil {
			t.Fatal("已确认的出货单不该允许删除")
		}
	})

	t.Run("取消的出货单把数量还回去", func(t *testing.T) {
		// 取消前 A 已出满 90（50 + 40）；取消第一票后应剩 60 可出。
		if _, err := fx.shipments.UpdateStatus(ctx, first.ID, dto.ShipmentStatusInput{
			Status: int8(model.DocumentStatusCancelled),
		}); err != nil {
			t.Fatalf("取消出货单失败: %v", err)
		}
		if _, err := fx.shipments.Create(ctx, dto.ShipmentInput{
			Allocations: []dto.ShipmentAllocationInput{{SalesOrderItemID: itemA.ID, Qty: 60}},
		}); err != nil {
			t.Fatalf("已取消的出货不该继续占着数量，实际 %v", err)
		}
		if _, err := fx.shipments.Create(ctx, dto.ShipmentInput{
			Allocations: []dto.ShipmentAllocationInput{{SalesOrderItemID: itemA.ID, Qty: 1}},
		}); err == nil {
			t.Fatal("还回来的 60 已被新票占用，不该再能出货")
		}
	})

	t.Run("草稿订单不能出货", func(t *testing.T) {
		_, err := fx.shipments.Create(ctx, dto.ShipmentInput{
			Allocations: []dto.ShipmentAllocationInput{
				{SalesOrderItemID: fx.draftOrder.Items[0].ID, Qty: 1},
			},
		})
		if e, ok := errs.From(err); !ok || e.Code != errs.CodeConflict {
			t.Fatalf("草稿订单出货应返回 409，实际 %v", err)
		}
		if msg := err.Error(); !strings.Contains(msg, "还是草稿") {
			t.Errorf("提示未指向订单状态: %q", msg)
		}
	})

	t.Run("订单详情带出已出与未出数量", func(t *testing.T) {
		o, err := fx.orders.Get(ctx, fx.orderA.ID)
		if err != nil {
			t.Fatalf("查订单详情失败: %v", err)
		}
		// A 的 100 件：第一票已取消（不计），第二票 40，第三票 60 → 已出 100。
		if got := o.Items[0].ShippedQty; !approx(got, 100) {
			t.Errorf("shipped_qty = %v，期望 100", got)
		}
		if got := o.Items[0].UnshippedQty; !approx(got, 0) {
			t.Errorf("unshipped_qty = %v，期望 0", got)
		}
	})

	t.Run("数量为 0 的分配行被拒", func(t *testing.T) {
		if _, err := fx.shipments.Create(ctx, dto.ShipmentInput{
			Allocations: []dto.ShipmentAllocationInput{{SalesOrderItemID: itemB.ID, Qty: 0}},
		}); err == nil {
			t.Fatal("数量为 0 应被拒")
		}
	})

	t.Run("草稿可删，单号不回收", func(t *testing.T) {
		s, err := fx.shipments.Create(ctx, dto.ShipmentInput{
			Allocations: []dto.ShipmentAllocationInput{{SalesOrderItemID: itemB.ID, Qty: 5}},
		})
		if err != nil {
			t.Fatalf("建出货批次失败: %v", err)
		}
		if err := fx.shipments.Delete(ctx, s.ID); err != nil {
			t.Fatalf("草稿应可删除，实际 %v", err)
		}
		if _, err := fx.shipments.Get(ctx, s.ID); !errors.Is(err, errs.ErrNotFound) {
			t.Errorf("删除后再查应 404，实际 %v", err)
		}
		// 单号不回收：删掉的 SH-XXXX 留在审计与聊天记录里，
		// 把它发给另一票货，等于两份不同的事实共用一个号。
		next, err := fx.shipments.Create(ctx, dto.ShipmentInput{
			Allocations: []dto.ShipmentAllocationInput{{SalesOrderItemID: itemB.ID, Qty: 5}},
		})
		if err != nil {
			t.Fatalf("建出货批次失败: %v", err)
		}
		if next.DocNo == s.DocNo {
			t.Errorf("单号被复用: %q", next.DocNo)
		}
	})
}
