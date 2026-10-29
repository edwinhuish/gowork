package service

import (
	"context"
	"testing"
	"time"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/errs"
	"github.com/edwinhuish/gowork/internal/pkg/money"
	"github.com/edwinhuish/gowork/internal/repository"
)

type purchaseFixture struct {
	svc        PurchaseOrderService
	sales      SalesOrderService
	now        *time.Time
	supplierID uint
	customerID uint
	productID  uint
	product2ID uint
	sourceID   uint
}

func newPurchaseFixture(t *testing.T, now *time.Time) purchaseFixture {
	t.Helper()
	ctx := context.Background()

	db := openTestDB(t,
		&model.Currency{}, &model.ExchangeRate{}, &model.Partner{}, &model.Product{},
		&model.ProductSource{}, &model.SalesOrder{}, &model.SalesOrderItem{},
		&model.PurchaseOrder{}, &model.PurchaseOrderItem{}, &model.DocSeq{},
		&model.Shipment{}, &model.ShipmentAllocation{})

	currencyRepo := repository.NewCurrencyRepository(db)
	rateRepo := repository.NewExchangeRateRepository(db)
	currencySvc := NewCurrencyService(currencyRepo, rateRepo)
	clock := func() time.Time { return *now }

	if _, err := currencySvc.Create(ctx, dto.CurrencyInput{
		Code: "CNY", Name: "人民币", IsBase: boolPtr(true),
	}); err != nil {
		t.Fatalf("建本币失败: %v", err)
	}
	if _, err := currencySvc.Create(ctx, dto.CurrencyInput{Code: "USD", Name: "美元"}); err != nil {
		t.Fatalf("建外币失败: %v", err)
	}
	if _, err := NewExchangeRateService(rateRepo, currencyRepo).Create(ctx, dto.ExchangeRateInput{
		CurrencyCode: "USD", Rate: money.MustParse("7.1"), EffectiveDate: now.Format(rateDateLayout),
	}); err != nil {
		t.Fatalf("登记汇率失败: %v", err)
	}

	partnerRepo := repository.NewPartnerRepository(db)
	supplier := &model.Partner{
		Code: "S001", Name: "永康五金厂", Role: model.PartnerRoleSupplier,
		EntityType: model.PartnerEntityCompany, Status: model.EnableStatusOn,
	}
	customer := &model.Partner{
		Code: "C001", Name: "Ferreteria El Sol", Role: model.PartnerRoleCustomer,
		EntityType: model.PartnerEntityCompany, Status: model.EnableStatusOn,
	}
	for _, p := range []*model.Partner{supplier, customer} {
		if err := partnerRepo.Create(ctx, p); err != nil {
			t.Fatalf("建主体失败: %v", err)
		}
	}

	productRepo := repository.NewProductRepository(db)
	p1 := &model.Product{
		SKUCode: "BH01-0001", NameCN: "不锈钢合页", Unit: "PCS", Status: model.EnableStatusOn,
	}
	p2 := &model.Product{
		SKUCode: "BH01-0002", NameCN: "合页（彩盒装）", Unit: "PCS", Status: model.EnableStatusOn,
	}
	for _, p := range []*model.Product{p1, p2} {
		if err := productRepo.Create(ctx, p); err != nil {
			t.Fatalf("建产品失败: %v", err)
		}
	}

	sourceRepo := repository.NewProductSourceRepository(db)
	src, err := NewProductSourceService(
		sourceRepo, productRepo, partnerRepo, currencySvc, WithProductSourceClock(clock),
	).Create(ctx, dto.ProductSourceInput{
		ProductID: p1.ID, PartnerID: supplier.ID, SupplierSKU: "A-100",
		PurchasePrice: money.MustParse("8"), MOQ: 500, DeliveryDays: 15,
	})
	if err != nil {
		t.Fatalf("建货源失败: %v", err)
	}

	docNoSvc := NewDocNoService(repository.NewDocSeqRepository(db), WithDocNoClock(clock))
	orderRepo := repository.NewSalesOrderRepository(db)

	return purchaseFixture{
		svc: NewPurchaseOrderService(
			repository.NewPurchaseOrderRepository(db), partnerRepo, productRepo, sourceRepo,
			orderRepo, currencySvc, docNoSvc, WithPurchaseOrderClock(clock),
		),
		sales: NewSalesOrderService(
			orderRepo, partnerRepo, productRepo, repository.NewShipmentRepository(db),
			currencySvc, docNoSvc,
			NewCreditService(repository.NewCreditRepository(db), partnerRepo),
			WithSalesOrderClock(clock),
		),
		now:        now,
		supplierID: supplier.ID,
		customerID: customer.ID,
		productID:  p1.ID,
		product2ID: p2.ID,
		sourceID:   src.ID,
	}
}

func TestPurchaseOrderRules(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 10, 28, 10, 0, 0, 0, time.Local)
	fx := newPurchaseFixture(t, &now)

	input := dto.PurchaseOrderInput{
		SupplierID:   fx.supplierID,
		TradeTerm:    "FOB",
		CurrencyCode: "usd",
		OrderDate:    "2026-10-28",
		Items: []dto.PurchaseOrderItemInput{
			{ProductID: fx.productID, Qty: qtyOf(100), UnitPrice: money.MustParse("2")},
		},
	}

	var created *model.PurchaseOrder
	t.Run("跨币种成本折本币", func(t *testing.T) {
		o, err := fx.svc.Create(ctx, input)
		if err != nil {
			t.Fatalf("新建采购单失败: %v", err)
		}
		created = o

		if o.DocNo != "PO-2610-001" {
			t.Errorf("doc_no = %q，期望 PO-2610-001", o.DocNo)
		}
		if o.Status != model.DocumentStatusDraft {
			t.Errorf("状态 = %d，期望草稿", o.Status)
		}
		// 100 × 2 USD = 200 USD；× 7.1 = 1420 CNY
		if o.AmountTotal.Cmp(money.MustParse("200")) != 0 ||
			o.AmountBaseTotal.Cmp(money.MustParse("1420")) != 0 {
			t.Errorf("合计 = %s / %s，期望 200 / 1420", o.AmountTotal, o.AmountBaseTotal)
		}
	})

	t.Run("按销售订单行采购", func(t *testing.T) {
		so, err := fx.sales.Create(ctx, dto.SalesOrderInput{
			PartnerID: fx.customerID, TradeTerm: "FOB", CurrencyCode: "USD",
			OrderDate: "2026-10-28",
			Items:     []dto.SalesOrderItemInput{{ProductID: fx.productID, Qty: qtyOf(100), UnitPrice: money.MustParse("10")}},
		})
		if err != nil {
			t.Fatalf("新建销售订单失败: %v", err)
		}
		soItemID := so.Items[0].ID

		in := input
		in.Items = []dto.PurchaseOrderItemInput{{
			ProductID: fx.productID, Qty: qtyOf(100), UnitPrice: money.MustParse("2"), SalesOrderItemID: &soItemID,
		}}
		o, err := fx.svc.Create(ctx, in)
		if err != nil {
			t.Fatalf("以销定采失败: %v", err)
		}
		if o.Items[0].SalesOrderItemID == nil || *o.Items[0].SalesOrderItemID != soItemID {
			t.Errorf("未记录关联的销售订单行: %v", o.Items[0].SalesOrderItemID)
		}

		// 挂到另一张已取消的销售单上应被拒。
		if _, err := fx.sales.UpdateStatus(ctx, so.ID, dto.SalesOrderStatusInput{
			Status: int8(model.DocumentStatusCancelled),
		}); err != nil {
			t.Fatalf("取消失败: %v", err)
		}
		if _, err := fx.svc.Create(ctx, in); err == nil {
			t.Fatal("已取消的销售订单不该还能据此采购")
		} else if e, ok := errs.From(err); !ok || e.Code != errs.CodeConflict {
			t.Fatalf("期望 40901，实际 %v", err)
		}
	})

	t.Run("挂到另一个产品的销售单行被拒", func(t *testing.T) {
		so, err := fx.sales.Create(ctx, dto.SalesOrderInput{
			PartnerID: fx.customerID, TradeTerm: "FOB", CurrencyCode: "USD",
			Items: []dto.SalesOrderItemInput{{ProductID: fx.product2ID, Qty: qtyOf(10), UnitPrice: money.MustParse("5")}},
		})
		if err != nil {
			t.Fatalf("新建销售订单失败: %v", err)
		}
		soItemID := so.Items[0].ID

		in := input
		in.Items = []dto.PurchaseOrderItemInput{{
			ProductID: fx.productID, Qty: qtyOf(10), UnitPrice: money.MustParse("2"), SalesOrderItemID: &soItemID,
		}}
		_, err = fx.svc.Create(ctx, in)
		if e, ok := errs.From(err); !ok || e.Code != errs.CodeInvalidParam {
			t.Fatalf("期望 40001，实际 %v", err)
		}
	})

	t.Run("货源不属于这个产品被拒", func(t *testing.T) {
		in := input
		src := fx.sourceID
		in.Items = []dto.PurchaseOrderItemInput{{
			ProductID: fx.product2ID, Qty: qtyOf(1), UnitPrice: money.MustParse("1"), SourceID: &src,
		}}
		_, err := fx.svc.Create(ctx, in)
		if e, ok := errs.From(err); !ok || e.Code != errs.CodeInvalidParam {
			t.Fatalf("期望 40001，实际 %v", err)
		}
	})

	t.Run("客户当供应商被拒", func(t *testing.T) {
		in := input
		in.SupplierID = fx.customerID
		_, err := fx.svc.Create(ctx, in)
		if e, ok := errs.From(err); !ok || e.Code != errs.CodeInvalidParam {
			t.Fatalf("期望 40001，实际 %v", err)
		}
	})

	t.Run("跳步的状态跳转被拒", func(t *testing.T) {
		_, err := fx.svc.UpdateStatus(ctx, created.ID, dto.PurchaseOrderStatusInput{
			Status: int8(model.DocumentStatusShipped),
		})
		if e, ok := errs.From(err); !ok || e.Code != errs.CodeConflict {
			t.Fatalf("期望 40901，实际 %v", err)
		}
	})

	t.Run("确认后不可改也不可删", func(t *testing.T) {
		if _, err := fx.svc.UpdateStatus(ctx, created.ID, dto.PurchaseOrderStatusInput{
			Status: int8(model.DocumentStatusConfirmed),
		}); err != nil {
			t.Fatalf("确认失败: %v", err)
		}
		if _, err := fx.svc.Update(ctx, created.ID, input); err == nil {
			t.Fatal("已确认的采购单不该允许修改")
		} else if e, ok := errs.From(err); !ok || e.Code != errs.CodeConflict {
			t.Fatalf("期望 40901，实际 %v", err)
		}
		if err := fx.svc.Delete(ctx, created.ID); err == nil {
			t.Fatal("已确认的采购单不该允许删除")
		}
	})

	t.Run("到货日早于下单日被拒", func(t *testing.T) {
		in := input
		in.DeliveryDate = "2026-10-01"
		_, err := fx.svc.Create(ctx, in)
		if e, ok := errs.From(err); !ok || e.Code != errs.CodeInvalidParam {
			t.Fatalf("期望 40001，实际 %v", err)
		}
	})
}
