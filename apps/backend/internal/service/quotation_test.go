package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/errs"
	"github.com/edwinhuish/gowork/internal/pkg/money"
	"github.com/edwinhuish/gowork/internal/repository"
)

type quotationFixture struct {
	svc         QuotationService
	orders      SalesOrderService
	now         *time.Time
	customerID  uint
	supplierID  uint
	productID   uint
	product2ID  uint
	cheapSrcID  uint
	priceySrcID uint
}

func newQuotationFixture(t *testing.T, now *time.Time) quotationFixture {
	t.Helper()
	ctx := context.Background()

	db := openTestDB(t,
		&model.Currency{}, &model.ExchangeRate{}, &model.Partner{}, &model.Product{},
		&model.ProductSource{}, &model.Quotation{}, &model.QuotationItem{},
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
	if _, err := NewExchangeRateService(rateRepo, currencyRepo).Create(ctx,
		dto.ExchangeRateInput{
			CurrencyCode:  "USD",
			Rate:          money.MustParse("7.1"),
			EffectiveDate: now.Format(rateDateLayout),
		}); err != nil {
		t.Fatalf("登记汇率失败: %v", err)
	}

	partnerRepo := repository.NewPartnerRepository(db)
	customer := &model.Partner{
		Code: "C001", Name: "Ferreteria El Sol", Role: model.PartnerRoleCustomer,
		EntityType: model.PartnerEntityCompany, Status: model.EnableStatusOn,
	}
	supplier := &model.Partner{
		Code: "S001", Name: "永康五金厂", Role: model.PartnerRoleSupplier,
		EntityType: model.PartnerEntityCompany, Status: model.EnableStatusOn,
	}
	for _, p := range []*model.Partner{customer, supplier} {
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
		SKUCode: "BH01-0002", NameCN: "没有货源的产品", Unit: "PCS",
		Status: model.EnableStatusOn,
	}
	for _, p := range []*model.Product{p1, p2} {
		if err := productRepo.Create(ctx, p); err != nil {
			t.Fatalf("建产品失败: %v", err)
		}
	}

	sourceRepo := repository.NewProductSourceRepository(db)
	sourceSvc := NewProductSourceService(
		sourceRepo, productRepo, partnerRepo, currencySvc, WithProductSourceClock(clock))
	cheap, err := sourceSvc.Create(ctx, dto.ProductSourceInput{
		ProductID: p1.ID, PartnerID: supplier.ID, SupplierSKU: "B-100",
		PurchasePrice: money.MustParse("8"), MOQ: 1000, DeliveryDays: 20,
	})
	if err != nil {
		t.Fatalf("建货源失败: %v", err)
	}
	pricey, err := sourceSvc.Create(ctx, dto.ProductSourceInput{
		ProductID: p1.ID, PartnerID: supplier.ID, SupplierSKU: "A-100",
		PurchasePrice: money.MustParse("1.2"), CurrencyCode: "USD", MOQ: 500, DeliveryDays: 15,
	})
	if err != nil {
		t.Fatalf("建货源失败: %v", err)
	}

	docNoSvc := NewDocNoService(repository.NewDocSeqRepository(db), WithDocNoClock(clock))
	orderSvc := NewSalesOrderService(
		repository.NewSalesOrderRepository(db), partnerRepo, productRepo,
		repository.NewShipmentRepository(db), currencySvc, docNoSvc,
		NewCreditService(repository.NewCreditRepository(db), partnerRepo),
		WithSalesOrderClock(clock),
	)

	return quotationFixture{
		svc: NewQuotationService(
			repository.NewQuotationRepository(db), partnerRepo, productRepo, sourceRepo,
			currencySvc, docNoSvc, orderSvc,
			WithQuotationClock(clock),
		),
		orders:      orderSvc,
		now:         now,
		customerID:  customer.ID,
		supplierID:  supplier.ID,
		productID:   p1.ID,
		product2ID:  p2.ID,
		cheapSrcID:  cheap.ID,
		priceySrcID: pricey.ID,
	}
}

func TestQuotationRules(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 10, 28, 10, 0, 0, 0, time.Local)
	fx := newQuotationFixture(t, &now)

	input := dto.QuotationInput{
		PartnerID:    fx.customerID,
		TradeTerm:    "FOB",
		CurrencyCode: "usd",
		ValidUntil:   "2026-11-30",
		Items: []dto.QuotationItemInput{
			{ProductID: fx.productID, Qty: qtyOf(100), UnitPrice: money.MustParse("10")},
		},
	}

	var created *dto.QuotationDetail
	t.Run("金额与毛利由后端算，成本锁在明细上", func(t *testing.T) {
		q, err := fx.svc.Create(ctx, input)
		if err != nil {
			t.Fatalf("新建报价单失败: %v", err)
		}
		created = q

		// 报价单号由案号派生（{案号}-QT{NN}），首笔生意从 01 起。
		if q.BaseNo == "" || q.DocNo != q.BaseNo+"-QT01" {
			t.Errorf("doc_no/base_no = %q / %q，期望 {案号}-QT01", q.DocNo, q.BaseNo)
		}
		if q.Version != 1 || q.RootID != 0 {
			t.Errorf("version/root_id = %d/%d，期望 1/0", q.Version, q.RootID)
		}
		if q.CurrencyCode != "USD" || q.ExchangeRate.Cmp(money.MustParse("7.1")) != 0 {
			t.Errorf("币种/汇率 = %s/%s，期望 USD/7.1", q.CurrencyCode, q.ExchangeRate)
		}
		// 100 × 10 USD = 1000 USD；× 7.1 = 7100 CNY；成本取最低货源 8 CNY × 100 = 800。
		if q.AmountTotal.Cmp(money.MustParse("1000")) != 0 ||
			q.AmountBaseTotal.Cmp(money.MustParse("7100")) != 0 {
			t.Errorf("金额 = %s / %s，期望 1000 / 7100", q.AmountTotal, q.AmountBaseTotal)
		}
		if q.CostTotal.Cmp(money.MustParse("800")) != 0 ||
			q.GrossProfit.Cmp(money.MustParse("6300")) != 0 {
			t.Errorf("成本/毛利 = %s / %s，期望 800 / 6300", q.CostTotal, q.GrossProfit)
		}
		if q.MarginRate.Cmp(money.MustParse("0.887324")) != 0 {
			t.Errorf("毛利率 = %s，期望 0.887324", q.MarginRate)
		}

		item := q.Items[0]
		if item.CostPrice.Cmp(money.MustParse("8")) != 0 {
			t.Errorf("明细成本价 = %s，期望 8（最低货源）", item.CostPrice)
		}
		if item.SourceID == nil || *item.SourceID != fx.cheapSrcID {
			t.Errorf("明细未记录成本取自哪条货源，实际 %v", item.SourceID)
		}
		// 产品信息快照：产品改名后报价单上仍是发给客户的那个说法。
		if item.SKUCode != "BH01-0001" || item.NameCN != "不锈钢合页" || item.Unit != "PCS" {
			t.Errorf("产品快照缺失: %+v", item)
		}
	})

	t.Run("指定货源则按指定价", func(t *testing.T) {
		src := fx.priceySrcID
		in := input
		in.Items = []dto.QuotationItemInput{
			{ProductID: fx.productID, Qty: qtyOf(100), UnitPrice: money.MustParse("10"), SourceID: &src},
		}
		q, err := fx.svc.Create(ctx, in)
		if err != nil {
			t.Fatalf("新建报价单失败: %v", err)
		}
		if q.CostTotal.Cmp(money.MustParse("852")) != 0 {
			t.Errorf("成本 = %s，期望 852（1.2 USD × 7.1 × 100）", q.CostTotal)
		}
		if q.MarginRate.Cmp(money.MustParse("0.88")) != 0 {
			t.Errorf("毛利率 = %s，期望 0.88", q.MarginRate)
		}
	})

	t.Run("产品没有货源时不给报价", func(t *testing.T) {
		in := input
		in.Items = []dto.QuotationItemInput{{ProductID: fx.product2ID, Qty: qtyOf(10), UnitPrice: money.MustParse("10")}}
		_, err := fx.svc.Create(ctx, in)
		if e, ok := errs.From(err); !ok || e.Code != errs.CodeInvalidParam {
			t.Fatalf("期望 40001（请先维护货源），实际 %v", err)
		}
	})

	t.Run("报给供应商被拒", func(t *testing.T) {
		in := input
		in.PartnerID = fx.supplierID
		_, err := fx.svc.Create(ctx, in)
		if e, ok := errs.From(err); !ok || e.Code != errs.CodeInvalidParam {
			t.Fatalf("期望 40001，实际 %v", err)
		}
	})

	t.Run("有效期不能早于今天", func(t *testing.T) {
		in := input
		in.ValidUntil = "2026-10-01"
		_, err := fx.svc.Create(ctx, in)
		if e, ok := errs.From(err); !ok || e.Code != errs.CodeInvalidParam {
			t.Fatalf("期望 40001，实际 %v", err)
		}
	})

	t.Run("试算不落库且与保存口径一致", func(t *testing.T) {
		pv, err := fx.svc.Preview(ctx, dto.QuotationPreviewInput{
			CurrencyCode: "usd", Items: input.Items,
		})
		if err != nil {
			t.Fatalf("试算失败: %v", err)
		}
		// 定点之后可以要求完全相等：试算与保存走的是同一个折算函数，
		// 任何一位差异都说明两条路径的舍入时机不同。
		if pv.AmountBaseTotal.Cmp(created.AmountBaseTotal) != 0 ||
			pv.CostTotal.Cmp(created.CostTotal) != 0 ||
			pv.MarginRate.Cmp(created.MarginRate) != 0 {
			t.Errorf("试算 %s/%s/%s 与保存 %s/%s/%s 口径不一致",
				pv.AmountBaseTotal, pv.CostTotal, pv.MarginRate,
				created.AmountBaseTotal, created.CostTotal, created.MarginRate)
		}
		if pv.Items[0].SourceName != "永康五金厂" {
			t.Errorf("试算未给出成本来源，实际 %q", pv.Items[0].SourceName)
		}
	})

	t.Run("报价单只能确认或取消", func(t *testing.T) {
		_, err := fx.svc.UpdateStatus(ctx, created.ID, dto.QuotationStatusInput{
			Status: int8(model.DocumentStatusPreparing),
		})
		if e, ok := errs.From(err); !ok || e.Code != errs.CodeInvalidParam {
			t.Fatalf("期望 40001（报价单不该出现备货中），实际 %v", err)
		}
	})

	t.Run("确认后不可改也不可删", func(t *testing.T) {
		q, err := fx.svc.UpdateStatus(ctx, created.ID, dto.QuotationStatusInput{
			Status: int8(model.DocumentStatusConfirmed),
		})
		if err != nil {
			t.Fatalf("确认失败: %v", err)
		}
		if q.ConvertBlock != "" {
			t.Errorf("已确认且有效期内不该有拦截原因，实际 %q", q.ConvertBlock)
		}
		if _, err := fx.svc.Update(ctx, created.ID, input); err == nil {
			t.Fatal("已确认的报价单不该允许修改")
		} else if e, ok := errs.From(err); !ok || e.Code != errs.CodeConflict {
			t.Fatalf("期望 40901，实际 %v", err)
		}
		if err := fx.svc.Delete(ctx, created.ID); err == nil {
			t.Fatal("已确认的报价单不该允许删除")
		} else if e, ok := errs.From(err); !ok || e.Code != errs.CodeConflict {
			t.Fatalf("期望 40901，实际 %v", err)
		}
	})

	t.Run("另存新版本沿用单号并升版本", func(t *testing.T) {
		v2, err := fx.svc.NewVersion(ctx, created.ID, dto.QuotationVersionInput{})
		if err != nil {
			t.Fatalf("另存新版本失败: %v", err)
		}
		if v2.DocNo != created.DocNo || v2.Version != 2 {
			t.Errorf("新版本 = %s v%d，期望 %s v2", v2.DocNo, v2.Version, created.DocNo)
		}
		if v2.Status != model.DocumentStatusDraft {
			t.Errorf("新版本状态 = %d，期望草稿", v2.Status)
		}
		if v2.RootID != created.ID {
			t.Errorf("新版本 root_id = %d，期望指向第 1 版 %d", v2.RootID, created.ID)
		}
		if len(v2.Items) != 1 || v2.Items[0].CostPrice.Cmp(created.Items[0].CostPrice) != 0 {
			t.Errorf("新版本未复制明细或成本快照变了: %+v", v2.Items)
		}
	})

	t.Run("过期后不可确认，另存必须给新有效期", func(t *testing.T) {
		start := now
		defer func() { now = start }()

		short := input
		short.ValidUntil = "2026-10-28"
		q, err := fx.svc.Create(ctx, short)
		if err != nil {
			t.Fatalf("新建报价单失败: %v", err)
		}
		// 截止日当天仍有效。
		if q.Expired {
			t.Error("截止日当天不该算过期")
		}

		now = start.AddDate(0, 0, 5)
		got, err := fx.svc.Get(ctx, q.ID)
		if err != nil {
			t.Fatalf("查报价单失败: %v", err)
		}
		if !got.Expired || !strings.Contains(got.ConvertBlock, "有效期") {
			t.Errorf("过期后应拦住转订单，实际 expired=%v block=%q", got.Expired, got.ConvertBlock)
		}
		if _, err := fx.svc.UpdateStatus(ctx, q.ID, dto.QuotationStatusInput{
			Status: int8(model.DocumentStatusConfirmed),
		}); err == nil {
			t.Fatal("过期报价单不该允许确认")
		} else if e, ok := errs.From(err); !ok || e.Code != errs.CodeConflict {
			t.Fatalf("期望 40901，实际 %v", err)
		}

		if _, err := fx.svc.NewVersion(ctx, q.ID, dto.QuotationVersionInput{}); err == nil {
			t.Fatal("过期报价单另存新版本时应要求填新的有效期")
		}
		v2, err := fx.svc.NewVersion(ctx, q.ID, dto.QuotationVersionInput{ValidUntil: "2026-12-31"})
		if err != nil {
			t.Fatalf("带新有效期另存应成功: %v", err)
		}
		if v2.Expired {
			t.Error("新有效期下不该算过期")
		}
	})

	t.Run("取消是终态", func(t *testing.T) {
		q, err := fx.svc.Create(ctx, input)
		if err != nil {
			t.Fatalf("新建报价单失败: %v", err)
		}
		if _, err := fx.svc.UpdateStatus(ctx, q.ID, dto.QuotationStatusInput{
			Status: int8(model.DocumentStatusCancelled),
		}); err != nil {
			t.Fatalf("草稿直接作废应被允许: %v", err)
		}
		if _, err := fx.svc.UpdateStatus(ctx, q.ID, dto.QuotationStatusInput{
			Status: int8(model.DocumentStatusConfirmed),
		}); err == nil {
			t.Fatal("已取消的报价单不该再被确认")
		}
	})
}

func TestQuotationConvert(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 10, 28, 10, 0, 0, 0, time.Local)
	fx := newQuotationFixture(t, &now)

	input := dto.QuotationInput{
		PartnerID:    fx.customerID,
		TradeTerm:    "CIF",
		CurrencyCode: "usd",
		ValidUntil:   "2026-11-30",
		Items: []dto.QuotationItemInput{
			{ProductID: fx.productID, Qty: qtyOf(100), UnitPrice: money.MustParse("10")},
		},
	}

	t.Run("草稿不能转订单", func(t *testing.T) {
		q, err := fx.svc.Create(ctx, input)
		if err != nil {
			t.Fatalf("新建报价单失败: %v", err)
		}
		_, err = fx.svc.Convert(ctx, q.ID, dto.QuotationConvertInput{})
		if e, ok := errs.From(err); !ok || e.Code != errs.CodeConflict {
			t.Fatalf("期望 40901（还是草稿），实际 %v", err)
		}
	})

	t.Run("确认后转单生成同价订单并回填", func(t *testing.T) {
		q, err := fx.svc.Create(ctx, input)
		if err != nil {
			t.Fatalf("新建报价单失败: %v", err)
		}
		if _, err := fx.svc.UpdateStatus(ctx, q.ID, dto.QuotationStatusInput{
			Status: int8(model.DocumentStatusConfirmed),
		}); err != nil {
			t.Fatalf("确认失败: %v", err)
		}

		got, err := fx.svc.Convert(ctx, q.ID, dto.QuotationConvertInput{
			OrderDate: "2026-10-28", DeliveryDate: "2026-12-20",
		})
		if err != nil {
			t.Fatalf("转订单失败: %v", err)
		}
		if got.SalesOrderID == nil || got.SalesOrderDocNo == "" {
			t.Fatalf("报价单未回填订单引用: %+v", got)
		}
		if got.ConvertBlock == "" {
			t.Error("已转单的报价单应当说明不能再转")
		}

		order, err := fx.orders.Get(ctx, *got.SalesOrderID)
		if err != nil {
			t.Fatalf("查订单失败: %v", err)
		}
		if order.DocNo != got.SalesOrderDocNo {
			t.Errorf("订单单号 = %q，与回填的 %q 不一致", order.DocNo, got.SalesOrderDocNo)
		}
		// 一笔生意一个案号：订单沿用报价的案号，序号也一致（QT0n 转出必得 SO0n）。
		if order.BaseNo == "" || order.BaseNo != q.BaseNo {
			t.Errorf("订单案号 = %q，期望沿用报价单的 %q", order.BaseNo, q.BaseNo)
		}
		if _, seq, ok := model.ParseDealDocNo(q.DocNo, model.DocTypeQuotation); !ok {
			t.Fatalf("报价单号不是案号格式: %q", q.DocNo)
		} else if want := model.FormatDealDocNo(q.BaseNo, model.DocTypeSalesOrder, seq); order.DocNo != want {
			t.Errorf("订单单号 = %q，期望 %q", order.DocNo, want)
		}
		if order.Status != model.DocumentStatusDraft {
			t.Errorf("转出来的订单状态 = %d，期望草稿", order.Status)
		}
		if order.PartnerID != q.PartnerID || order.TradeTerm != q.TradeTerm {
			t.Errorf("客户/贸易术语未沿用报价单: %d/%s", order.PartnerID, order.TradeTerm)
		}
		// 汇率与金额都已是定点：转单必须一位不差地沿用报价单。
		if order.ExchangeRate.Cmp(q.ExchangeRate) != 0 {
			t.Errorf("订单汇率 = %s，期望沿用报价单的 %s", order.ExchangeRate, q.ExchangeRate)
		}
		if order.AmountBaseTotal.Cmp(q.AmountBaseTotal) != 0 {
			t.Errorf("订单本币总额 = %s，期望与报价单一致 %s",
				order.AmountBaseTotal, q.AmountBaseTotal)
		}
		if len(order.Items) != 1 || order.Items[0].UnitPrice.Cmp(money.MustParse("10")) != 0 ||
			!approxQty(order.Items[0].Qty, 100) {
			t.Errorf("订单明细未沿用报价单: %+v", order.Items)
		}

		// 重复转换
		if _, err := fx.svc.Convert(ctx, q.ID, dto.QuotationConvertInput{}); err == nil {
			t.Fatal("同一张报价单不该允许转两次")
		} else if e, ok := errs.From(err); !ok || e.Code != errs.CodeConflict {
			t.Fatalf("期望 40901，实际 %v", err)
		}

		// 已转单后不能再作废，也不能再改
		if _, err := fx.svc.UpdateStatus(ctx, q.ID, dto.QuotationStatusInput{
			Status: int8(model.DocumentStatusCancelled),
		}); err == nil {
			t.Fatal("已转单的报价单不该允许作废")
		}
		if _, err := fx.svc.Update(ctx, q.ID, input); err == nil {
			t.Fatal("已转单的报价单不该允许修改")
		}
	})

	t.Run("过期报价单不能转单", func(t *testing.T) {
		start := now
		defer func() { now = start }()

		short := input
		short.ValidUntil = "2026-10-28"
		q, err := fx.svc.Create(ctx, short)
		if err != nil {
			t.Fatalf("新建报价单失败: %v", err)
		}
		now = start.AddDate(0, 0, 3)
		_, err = fx.svc.Convert(ctx, q.ID, dto.QuotationConvertInput{})
		if e, ok := errs.From(err); !ok || e.Code != errs.CodeConflict {
			t.Fatalf("期望 40901（已过期），实际 %v", err)
		}
	})

	t.Run("交期早于下单日被拒", func(t *testing.T) {
		q, err := fx.svc.Create(ctx, input)
		if err != nil {
			t.Fatalf("新建报价单失败: %v", err)
		}
		if _, err := fx.svc.UpdateStatus(ctx, q.ID, dto.QuotationStatusInput{
			Status: int8(model.DocumentStatusConfirmed),
		}); err != nil {
			t.Fatalf("确认失败: %v", err)
		}
		_, err = fx.svc.Convert(ctx, q.ID, dto.QuotationConvertInput{
			OrderDate: "2026-11-10", DeliveryDate: "2026-11-01",
		})
		if e, ok := errs.From(err); !ok || e.Code != errs.CodeInvalidParam {
			t.Fatalf("期望 40001，实际 %v", err)
		}
	})
}
