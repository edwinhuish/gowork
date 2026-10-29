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

type salesOrderFixture struct {
	svc        SalesOrderService
	now        *time.Time
	customerID uint
	otherID    uint
	supplierID uint
	productID  uint
	product2ID uint
}

func newSalesOrderFixture(t *testing.T, now *time.Time) salesOrderFixture {
	t.Helper()
	ctx := context.Background()

	db := openTestDB(t,
		&model.Currency{}, &model.ExchangeRate{}, &model.Partner{}, &model.Product{},
		&model.SalesOrder{}, &model.SalesOrderItem{}, &model.DocSeq{},
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
	customer := &model.Partner{
		Code: "C001", Name: "Ferreteria El Sol", Role: model.PartnerRoleCustomer,
		EntityType: model.PartnerEntityCompany, Status: model.EnableStatusOn,
	}
	other := &model.Partner{
		Code: "C002", Name: "另一家客户", Role: model.PartnerRoleCustomer,
		EntityType: model.PartnerEntityCompany, Status: model.EnableStatusOn,
	}
	supplier := &model.Partner{
		Code: "S001", Name: "永康五金厂", Role: model.PartnerRoleSupplier,
		EntityType: model.PartnerEntityCompany, Status: model.EnableStatusOn,
	}
	for _, p := range []*model.Partner{customer, other, supplier} {
		if err := partnerRepo.Create(ctx, p); err != nil {
			t.Fatalf("建主体失败: %v", err)
		}
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

	return salesOrderFixture{
		svc: NewSalesOrderService(
			repository.NewSalesOrderRepository(db), partnerRepo, productRepo,
			repository.NewShipmentRepository(db), currencySvc,
			NewDocNoService(repository.NewDocSeqRepository(db), WithDocNoClock(clock)),
			NewCreditService(repository.NewCreditRepository(db), partnerRepo),
			WithSalesOrderClock(clock),
		),
		now:        now,
		customerID: customer.ID,
		otherID:    other.ID,
		supplierID: supplier.ID,
		productID:  p1.ID,
		product2ID: p2.ID,
	}
}

func TestSalesOrderRules(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 10, 28, 10, 0, 0, 0, time.Local)
	fx := newSalesOrderFixture(t, &now)

	input := dto.SalesOrderInput{
		PartnerID:    fx.customerID,
		TradeTerm:    "fob",
		OrderDate:    "2026-10-28",
		CurrencyCode: "usd",
		Items: []dto.SalesOrderItemInput{
			{ProductID: fx.productID, Qty: 100, UnitPrice: money.MustParse("10")},
			{ProductID: fx.product2ID, Qty: 20, UnitPrice: money.MustParse("5")},
		},
	}

	var created *model.SalesOrder
	t.Run("总额只由明细汇总", func(t *testing.T) {
		o, err := fx.svc.Create(ctx, input)
		if err != nil {
			t.Fatalf("新建订单失败: %v", err)
		}
		created = o

		// 手建订单没有报价可继承，算一笔新生意：{案号}-SO{NN}。
		if o.BaseNo == "" || o.DocNo != o.BaseNo+"-SO01" {
			t.Errorf("doc_no/base_no = %q / %q，期望 {案号}-SO01", o.DocNo, o.BaseNo)
		}
		if o.TradeTerm != "FOB" {
			t.Errorf("trade_term = %q，期望归一化为 FOB", o.TradeTerm)
		}
		if o.Status != model.DocumentStatusDraft {
			t.Errorf("新单状态 = %d，期望草稿", o.Status)
		}
		// 1000 + 100 = 1100 USD；× 7.1 = 7810 CNY
		if o.AmountTotal.Cmp(money.MustParse("1100")) != 0 ||
			o.AmountBaseTotal.Cmp(money.MustParse("7810")) != 0 {
			t.Errorf("合计 = %s / %s，期望 1100 / 7810", o.AmountTotal, o.AmountBaseTotal)
		}
		if len(o.Items) != 2 {
			t.Fatalf("明细行数 = %d，期望 2", len(o.Items))
		}
		if o.Items[1].SKUCode != "BH01-0002" || o.Items[1].NameCN != "合页（彩盒装）" {
			t.Errorf("明细未存产品快照: %+v", o.Items[1])
		}
	})

	t.Run("跳步的状态跳转被拒", func(t *testing.T) {
		for _, next := range []model.DocumentStatus{
			model.DocumentStatusPreparing,
			model.DocumentStatusShipped,
			model.DocumentStatusCompleted,
		} {
			// 非法跳转给 409 而不是 400：请求本身没写错，是单据当前状态不允许。
			_, err := fx.svc.UpdateStatus(ctx, created.ID, dto.SalesOrderStatusInput{Status: int8(next)})
			if e, ok := errs.From(err); !ok || e.Code != errs.CodeConflict {
				t.Fatalf("草稿直接跳到 %d 应被拒绝，实际 %v", next, err)
			}
		}
	})

	t.Run("正向流转与取消边界", func(t *testing.T) {
		steps := []model.DocumentStatus{
			model.DocumentStatusConfirmed,
			model.DocumentStatusPreparing,
			model.DocumentStatusShipped,
			model.DocumentStatusCompleted,
		}
		for _, next := range steps {
			o, err := fx.svc.UpdateStatus(ctx, created.ID, dto.SalesOrderStatusInput{Status: int8(next)})
			if err != nil {
				t.Fatalf("转到 %d 失败: %v", next, err)
			}
			if o.Status != next {
				t.Errorf("状态 = %d，期望 %d", o.Status, next)
			}
		}
		// 已完成后不能再取消，也不能回到备货中。
		if _, err := fx.svc.UpdateStatus(ctx, created.ID, dto.SalesOrderStatusInput{
			Status: int8(model.DocumentStatusCancelled),
		}); err == nil {
			t.Fatal("已完成的订单不该允许取消")
		}
	})

	t.Run("已确认的订单不可改也不可删", func(t *testing.T) {
		o, err := fx.svc.Create(ctx, input)
		if err != nil {
			t.Fatalf("新建订单失败: %v", err)
		}
		if _, err := fx.svc.UpdateStatus(ctx, o.ID, dto.SalesOrderStatusInput{
			Status: int8(model.DocumentStatusConfirmed),
		}); err != nil {
			t.Fatalf("确认失败: %v", err)
		}
		if _, err := fx.svc.Update(ctx, o.ID, input); err == nil {
			t.Fatal("已确认的订单不该允许修改")
		} else if e, ok := errs.From(err); !ok || e.Code != errs.CodeConflict {
			t.Fatalf("期望 40901，实际 %v", err)
		}
		if err := fx.svc.Delete(ctx, o.ID); err == nil {
			t.Fatal("已确认的订单不该允许删除")
		}
	})

	t.Run("改客户被拒", func(t *testing.T) {
		draft, err := fx.svc.Create(ctx, input)
		if err != nil {
			t.Fatalf("新建订单失败: %v", err)
		}
		changed := input
		changed.PartnerID = fx.otherID
		_, err = fx.svc.Update(ctx, draft.ID, changed)
		if e, ok := errs.From(err); !ok || e.Code != errs.CodeInvalidParam {
			t.Fatalf("期望 40001，实际 %v", err)
		}
	})

	t.Run("报给供应商被拒", func(t *testing.T) {
		bad := input
		bad.PartnerID = fx.supplierID
		_, err := fx.svc.Create(ctx, bad)
		if e, ok := errs.From(err); !ok || e.Code != errs.CodeInvalidParam {
			t.Fatalf("期望 40001，实际 %v", err)
		}
	})

	t.Run("交期早于下单日被拒", func(t *testing.T) {
		bad := input
		bad.DeliveryDate = "2026-10-01"
		_, err := fx.svc.Create(ctx, bad)
		if e, ok := errs.From(err); !ok || e.Code != errs.CodeInvalidParam {
			t.Fatalf("期望 40001，实际 %v", err)
		}
	})

	t.Run("下单日留空取今天", func(t *testing.T) {
		in := input
		in.OrderDate = ""
		o, err := fx.svc.Create(ctx, in)
		if err != nil {
			t.Fatalf("新建订单失败: %v", err)
		}
		if !o.OrderDate.Equal(model.DateOnly(now)) {
			t.Errorf("order_date = %v，期望 %v", o.OrderDate, model.DateOnly(now))
		}
	})
}
