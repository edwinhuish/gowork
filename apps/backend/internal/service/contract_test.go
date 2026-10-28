package service

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/errs"
	"github.com/edwinhuish/gowork/internal/repository"
)

type contractFixture struct {
	svc           ContractService
	quotations    QuotationService
	orderRepo     repository.SalesOrderRepository
	quotationRepo repository.QuotationRepository
	contractRepo  repository.ContractRepository
	currencySvc   CurrencyService
	now           time.Time
	customerID    uint
	productID     uint
	// orderSeq 给每张测试订单发一个不重复的单号：doc_no 上有唯一索引，
	// 两个子测试各造一张同号订单，第二个会死在约束上而不是死在断言上。
	orderSeq *int
}

func newContractFixture(t *testing.T, now time.Time) contractFixture {
	t.Helper()
	ctx := context.Background()

	db := openTestDB(t,
		&model.Currency{}, &model.ExchangeRate{}, &model.Partner{}, &model.Product{},
		&model.ProductSource{}, &model.Quotation{}, &model.QuotationItem{},
		&model.SalesOrder{}, &model.SalesOrderItem{}, &model.DocSeq{},
		&model.Contract{}, &model.ContractTerm{})

	currencyRepo := repository.NewCurrencyRepository(db)
	rateRepo := repository.NewExchangeRateRepository(db)
	currencySvc := NewCurrencyService(currencyRepo, rateRepo)
	clock := func() time.Time { return now }

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
			Rate:          7.1,
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
	product := &model.Product{
		SKUCode: "BH01-0001", NameCN: "不锈钢合页", Spec: "100×75×3", Unit: "PCS",
		Status: model.EnableStatusOn,
	}
	if err := productRepo.Create(ctx, product); err != nil {
		t.Fatalf("建产品失败: %v", err)
	}

	sourceRepo := repository.NewProductSourceRepository(db)
	if _, err := NewProductSourceService(
		sourceRepo, productRepo, partnerRepo, currencySvc, WithProductSourceClock(clock),
	).Create(ctx, dto.ProductSourceInput{
		ProductID: product.ID, PartnerID: supplier.ID, SupplierSKU: "B-100",
		PurchasePrice: 8, MOQ: 1000, DeliveryDays: 20,
	}); err != nil {
		t.Fatalf("建货源失败: %v", err)
	}

	docNoSvc := NewDocNoService(repository.NewDocSeqRepository(db), WithDocNoClock(clock))
	orderRepo := repository.NewSalesOrderRepository(db)
	quotationRepo := repository.NewQuotationRepository(db)
	contractRepo := repository.NewContractRepository(db)

	// 报价服务只用来造来源单据；orders 传 nil 是因为这批测试不做报价转单。
	quotationSvc := NewQuotationService(
		quotationRepo, partnerRepo, productRepo, sourceRepo,
		currencySvc, docNoSvc, nil, WithQuotationClock(clock),
	)
	contractSvc := NewContractService(
		contractRepo, quotationRepo, orderRepo,
		currencySvc, docNoSvc,
		WithContractClock(clock),
	)

	return contractFixture{
		svc:           contractSvc,
		quotations:    quotationSvc,
		orderRepo:     orderRepo,
		quotationRepo: quotationRepo,
		contractRepo:  contractRepo,
		currencySvc:   currencySvc,
		now:           now,
		customerID:    customer.ID,
		productID:     product.ID,
		orderSeq:      new(int),
	}
}

// confirmedQuotation 造一张已确认的报价单：100 × 10 USD = 1000 USD = 7100 CNY。
func (f contractFixture) confirmedQuotation(t *testing.T) *dto.QuotationDetail {
	t.Helper()
	ctx := context.Background()

	q, err := f.quotations.Create(ctx, dto.QuotationInput{
		PartnerID:    f.customerID,
		TradeTerm:    "FOB",
		CurrencyCode: "USD",
		ValidUntil:   f.now.AddDate(0, 1, 0).Format("2006-01-02"),
		Items: []dto.QuotationItemInput{
			{ProductID: f.productID, Qty: 100, UnitPrice: 10},
		},
	})
	if err != nil {
		t.Fatalf("建报价单失败: %v", err)
	}
	q, err = f.quotations.UpdateStatus(ctx, q.ID, dto.QuotationStatusInput{
		Status: int8(model.DocumentStatusConfirmed),
	})
	if err != nil {
		t.Fatalf("确认报价单失败: %v", err)
	}
	return q
}

// orderFromQuotation 直接落一张订单，绕过订单服务：这批测试只关心合同怎么读它。
func (f contractFixture) orderFromQuotation(
	t *testing.T, status model.DocumentStatus, delivery *time.Time,
) *model.SalesOrder {
	t.Helper()
	*f.orderSeq++
	o := &model.SalesOrder{
		DocNo:           fmt.Sprintf("SO-202610-%04d", *f.orderSeq),
		PartnerID:       f.customerID,
		TradeTerm:       model.TradeTermFOB,
		Status:          status,
		OrderDate:       f.now,
		DeliveryDate:    delivery,
		CurrencyCode:    "USD",
		ExchangeRate:    7.1,
		AmountTotal:     1000,
		AmountBaseTotal: 7100,
	}
	if err := f.orderRepo.CreateWithItems(context.Background(), o, nil); err != nil {
		t.Fatalf("建订单失败: %v", err)
	}
	return o
}

func wantCode(t *testing.T, err error, code errs.Code, scene string) {
	t.Helper()
	e, ok := errs.From(err)
	if !ok || e.Code != code {
		t.Fatalf("%s：期望错误码 %d，实际 %v", scene, code, err)
	}
}

func TestContractGenerateFromQuotation(t *testing.T) {
	ctx := context.Background()
	fx := newContractFixture(t, time.Date(2026, 10, 28, 10, 0, 0, 0, time.Local))

	input := func(qid uint, ct model.ContractType, terms ...dto.ContractTermInput) dto.ContractGenerateInput {
		return dto.ContractGenerateInput{
			Type:       int8(ct),
			SourceType: int8(model.ContractSourceQuotation),
			SourceID:   qid,
			SignDate:   "2026-10-28",
			Terms:      terms,
		}
	}

	t.Run("表头与分期照抄来源单据并算出本币", func(t *testing.T) {
		q := fx.confirmedQuotation(t)
		c, err := fx.svc.Generate(ctx, input(q.ID, model.ContractTypePI,
			dto.ContractTermInput{Name: "订金 30%", Amount: 300, DueDate: "2026-11-05"},
			dto.ContractTermInput{Name: "发货前 30%", Amount: 300, DueDate: "2026-12-05"},
			dto.ContractTermInput{Name: "尾款 40%", Amount: 400, Trigger: "见提单副本后 7 天"},
		))
		if err != nil {
			t.Fatalf("生成合同失败: %v", err)
		}

		if !strings.HasPrefix(c.DocNo, "PI-202610-") {
			t.Errorf("doc_no = %q，期望 PI- 前缀", c.DocNo)
		}
		if c.Type != model.ContractTypePI || c.Status != model.DocumentStatusDraft {
			t.Errorf("type/status = %v/%v，期望 PI/draft", c.Type, c.Status)
		}
		if c.PartnerID != fx.customerID || c.SourceDocNo != q.DocNo || c.SourceID != q.ID {
			t.Errorf("来源快照不对: partner=%d source=%s/%d",
				c.PartnerID, c.SourceDocNo, c.SourceID)
		}
		// 金额与汇率照抄报价单，不重取当日汇率。
		if c.CurrencyCode != "USD" || !approx(c.ExchangeRate, 7.1) {
			t.Errorf("币种/汇率 = %s/%v，期望 USD/7.1", c.CurrencyCode, c.ExchangeRate)
		}
		if !approx(c.AmountTotal, 1000) || !approx(c.AmountBaseTotal, 7100) {
			t.Errorf("金额 = %v / %v，期望 1000 / 7100", c.AmountTotal, c.AmountBaseTotal)
		}
		// 报价单没有交期，合同也不该凭空编一个。
		if c.DeliveryDate != nil {
			t.Errorf("交期 = %v，期望为空", c.DeliveryDate)
		}

		if len(c.Terms) != 3 {
			t.Fatalf("分期数 = %d，期望 3", len(c.Terms))
		}
		for i, want := range []float64{2130, 2130, 2840} {
			if c.Terms[i].Seq != i+1 {
				t.Errorf("第 %d 期 seq = %d，期望 %d", i+1, c.Terms[i].Seq, i+1)
			}
			if !approx(c.Terms[i].AmountBase, want) {
				t.Errorf("第 %d 期本币 = %v，期望 %v", i+1, c.Terms[i].AmountBase, want)
			}
		}
		sum := 0.0
		for _, term := range c.Terms {
			sum += term.AmountBase
		}
		if !approx(sum, c.AmountBaseTotal) {
			t.Errorf("分期本币合计 = %v，期望与合同本币总额 %v 一致", sum, c.AmountBaseTotal)
		}
	})

	t.Run("分期合计不等于总额时拒绝", func(t *testing.T) {
		q := fx.confirmedQuotation(t)
		_, err := fx.svc.Generate(ctx, input(q.ID, model.ContractTypePI,
			dto.ContractTermInput{Name: "订金", Amount: 300},
			dto.ContractTermInput{Name: "尾款", Amount: 600},
		))
		wantCode(t, err, errs.CodeInvalidParam, "分期合计 900 与总额 1000 不符")
	})

	t.Run("同一来源同一类型只能生成一次", func(t *testing.T) {
		q := fx.confirmedQuotation(t)
		if _, err := fx.svc.Generate(ctx, input(q.ID, model.ContractTypePI,
			dto.ContractTermInput{Name: "全额", Amount: 1000},
		)); err != nil {
			t.Fatalf("首次生成失败: %v", err)
		}
		_, err := fx.svc.Generate(ctx, input(q.ID, model.ContractTypePI,
			dto.ContractTermInput{Name: "全额", Amount: 1000},
		))
		wantCode(t, err, errs.CodeConflict, "重复生成 PI")

		// PI 与销售合同是两条线：先收订金再签正式合同是正常流程。
		c, err := fx.svc.Generate(ctx, input(q.ID, model.ContractTypeSales,
			dto.ContractTermInput{Name: "全额", Amount: 1000},
		))
		if err != nil {
			t.Fatalf("同来源生成销售合同失败: %v", err)
		}
		if !strings.HasPrefix(c.DocNo, "SC-") {
			t.Errorf("销售合同 doc_no = %q，期望 SC- 前缀", c.DocNo)
		}
	})

	t.Run("草稿报价单不能生成合同", func(t *testing.T) {
		q, err := fx.quotations.Create(ctx, dto.QuotationInput{
			PartnerID:    fx.customerID,
			TradeTerm:    "FOB",
			CurrencyCode: "USD",
			ValidUntil:   "2026-11-30",
			Items:        []dto.QuotationItemInput{{ProductID: fx.productID, Qty: 100, UnitPrice: 10}},
		})
		if err != nil {
			t.Fatalf("建报价单失败: %v", err)
		}
		_, err = fx.svc.Generate(ctx, input(q.ID, model.ContractTypePI,
			dto.ContractTermInput{Name: "全额", Amount: 1000},
		))
		wantCode(t, err, errs.CodeConflict, "草稿报价单生成合同")
	})
}

func TestContractOrderSourceAndTransitions(t *testing.T) {
	ctx := context.Background()
	fx := newContractFixture(t, time.Date(2026, 10, 28, 10, 0, 0, 0, time.Local))
	delivery := time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)

	generate := func(orderID uint) (*model.Contract, error) {
		return fx.svc.Generate(ctx, dto.ContractGenerateInput{
			Type:       int8(model.ContractTypeSales),
			SourceType: int8(model.ContractSourceSalesOrder),
			SourceID:   orderID,
			SignDate:   "2026-10-28",
			Terms:      []dto.ContractTermInput{{Name: "全额", Amount: 1000}},
		})
	}

	t.Run("草稿订单不能生成合同", func(t *testing.T) {
		o := fx.orderFromQuotation(t, model.DocumentStatusDraft, &delivery)
		_, err := generate(o.ID)
		wantCode(t, err, errs.CodeConflict, "草稿订单生成合同")
	})

	t.Run("已确认订单生成合同并沿用交期", func(t *testing.T) {
		o := fx.orderFromQuotation(t, model.DocumentStatusConfirmed, &delivery)
		c, err := generate(o.ID)
		if err != nil {
			t.Fatalf("生成合同失败: %v", err)
		}
		if c.PartnerID != fx.customerID || !approx(c.AmountTotal, 1000) {
			t.Errorf("未照抄订单: partner=%d amount=%v", c.PartnerID, c.AmountTotal)
		}
		if c.DeliveryDate == nil || !c.DeliveryDate.Equal(delivery) {
			t.Errorf("交期 = %v，期望沿用订单交期 %v", c.DeliveryDate, delivery)
		}

		// 编辑：条款合计不符拒绝；改对之后交期可以清空，签订日保持原值。
		_, err = fx.svc.Update(ctx, c.ID, dto.ContractUpdateInput{
			Terms: []dto.ContractTermInput{{Name: "全额", Amount: 900}},
		})
		wantCode(t, err, errs.CodeInvalidParam, "编辑分期合计不符")

		updated, err := fx.svc.Update(ctx, c.ID, dto.ContractUpdateInput{
			Remark: "客户要求两期",
			Terms: []dto.ContractTermInput{
				{Name: "订金", Amount: 300},
				{Name: "尾款", Amount: 700},
			},
		})
		if err != nil {
			t.Fatalf("编辑合同失败: %v", err)
		}
		if updated.DeliveryDate != nil {
			t.Errorf("交期 = %v，期望清空", updated.DeliveryDate)
		}
		if !updated.SignDate.Equal(c.SignDate) {
			t.Errorf("签订日 = %v，期望保持 %v", updated.SignDate, c.SignDate)
		}
		if len(updated.Terms) != 2 || updated.Terms[1].Seq != 2 {
			t.Errorf("分期未整体替换: %+v", updated.Terms)
		}

		// 确认之后是对客户的承诺：不能再改、不能删、不能重复确认。
		confirmed, err := fx.svc.UpdateStatus(ctx, c.ID, dto.ContractStatusInput{
			Status: int8(model.DocumentStatusConfirmed),
		})
		if err != nil {
			t.Fatalf("确认合同失败: %v", err)
		}
		if confirmed.Status != model.DocumentStatusConfirmed {
			t.Errorf("status = %v，期望 confirmed", confirmed.Status)
		}

		_, err = fx.svc.Update(ctx, c.ID, dto.ContractUpdateInput{
			Terms: []dto.ContractTermInput{{Name: "全额", Amount: 1000}},
		})
		wantCode(t, err, errs.CodeConflict, "修改已确认合同")

		wantCode(t, fx.svc.Delete(ctx, c.ID), errs.CodeConflict, "删除已确认合同")
		_, err = fx.svc.UpdateStatus(ctx, c.ID, dto.ContractStatusInput{
			Status: int8(model.DocumentStatusConfirmed),
		})
		wantCode(t, err, errs.CodeConflict, "重复确认合同")

		// 取消是终态：保留记录供追溯，不抹掉。
		cancelled, err := fx.svc.UpdateStatus(ctx, c.ID, dto.ContractStatusInput{
			Status: int8(model.DocumentStatusCancelled),
		})
		if err != nil {
			t.Fatalf("取消合同失败: %v", err)
		}
		if cancelled.Status != model.DocumentStatusCancelled {
			t.Errorf("status = %v，期望 cancelled", cancelled.Status)
		}
	})
}

func TestContractListFilters(t *testing.T) {
	ctx := context.Background()
	fx := newContractFixture(t, time.Date(2026, 10, 28, 10, 0, 0, 0, time.Local))

	q := fx.confirmedQuotation(t)
	if _, err := fx.svc.Generate(ctx, dto.ContractGenerateInput{
		Type:       int8(model.ContractTypePI),
		SourceType: int8(model.ContractSourceQuotation),
		SourceID:   q.ID,
		Terms:      []dto.ContractTermInput{{Name: "全额", Amount: 1000}},
	}); err != nil {
		t.Fatalf("生成合同失败: %v", err)
	}

	piType := int8(model.ContractTypePI)
	res, err := fx.svc.List(ctx, dto.ContractQuery{Type: &piType})
	if err != nil {
		t.Fatalf("列表查询失败: %v", err)
	}
	if res.Total != 1 || len(res.List) != 1 {
		t.Fatalf("按类型筛选 total = %d，期望 1", res.Total)
	}

	// 列表不预加载条款：一页几十份、每份几期，带出来只为显示"分几期"不值得。
	if len(res.List[0].Terms) != 0 {
		t.Errorf("列表不应带出分期，实际 %d 条", len(res.List[0].Terms))
	}

	salesType := int8(model.ContractTypeSales)
	res, err = fx.svc.List(ctx, dto.ContractQuery{Type: &salesType})
	if err != nil {
		t.Fatalf("列表查询失败: %v", err)
	}
	if res.Total != 0 {
		t.Errorf("销售合同 total = %d，期望 0", res.Total)
	}

	// 未知取值不悄悄当成"不过滤"：那样筛选条一填错就看到全量数据。
	bad := int8(99)
	if _, err := fx.svc.List(ctx, dto.ContractQuery{Type: &bad}); err == nil {
		t.Fatal("非法 type 应该报错")
	}
}
