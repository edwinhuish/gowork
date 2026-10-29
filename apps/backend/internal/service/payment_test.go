package service

import (
	"context"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/money"
	"github.com/edwinhuish/gowork/internal/repository"
)

type paymentFixture struct {
	payments     PaymentService
	receivables  ReceivableService
	payables     PayableService
	db           *gorm.DB
	customerID   uint
	supplierID   uint
	receivableID uint
	payableID    uint
	// payDate 是到账日，比开单日往后 10 天；汇率在那一天是 7.6，
	// 与订单上锁的 7.1、开单当日的 7.5 都不同——取错源会直接断言失败。
	payDate time.Time
}

// newPaymentFixture 在应收应付之上再造一层收付款环境：
// 一张 USD 1000 的应收（订单汇率 7.1）与一张 CNY 3000 的应付（汇率 1），
// 汇率表当日 7.5、到账日 7.6。
func newPaymentFixture(t *testing.T, now time.Time) paymentFixture {
	t.Helper()
	ctx := context.Background()

	db := openTestDB(t,
		&model.Currency{}, &model.ExchangeRate{}, &model.Partner{},
		&model.SalesOrder{}, &model.SalesOrderItem{},
		&model.PurchaseOrder{}, &model.PurchaseOrderItem{},
		&model.DocSeq{}, &model.Receivable{}, &model.Payable{},
		&model.Payment{}, &model.PaymentAllocation{})

	clock := func() time.Time { return now }
	partnerRepo := repository.NewPartnerRepository(db)
	currencyRepo := repository.NewCurrencyRepository(db)
	rateRepo := repository.NewExchangeRateRepository(db)
	docNoSvc := NewDocNoService(repository.NewDocSeqRepository(db), WithDocNoClock(clock))

	currencySvc := NewCurrencyService(currencyRepo, rateRepo)
	for _, c := range []dto.CurrencyInput{
		{Code: "CNY", Name: "人民币", IsBase: boolPtr(true)},
		{Code: "USD", Name: "美元"},
	} {
		if _, err := currencySvc.Create(ctx, c); err != nil {
			t.Fatalf("建币种 %s 失败: %v", c.Code, err)
		}
	}

	payDate := now.AddDate(0, 0, 10)
	rateSvc := NewExchangeRateService(rateRepo, currencyRepo)
	for _, r := range []dto.ExchangeRateInput{
		{CurrencyCode: "USD", Rate: 7.5, EffectiveDate: now.Format(rateDateLayout)},
		{CurrencyCode: "USD", Rate: 7.6, EffectiveDate: payDate.Format(rateDateLayout)},
	} {
		if _, err := rateSvc.Create(ctx, r); err != nil {
			t.Fatalf("登记汇率 %v 失败: %v", r.Rate, err)
		}
	}

	customer := &model.Partner{
		Code: "C001", Name: "Ferreteria El Sol", Role: model.PartnerRoleCustomer,
		EntityType: model.PartnerEntityCompany, Status: model.EnableStatusOn, CreditDays: 30,
	}
	supplier := &model.Partner{
		Code: "S001", Name: "宁波五金", Role: model.PartnerRoleSupplier,
		EntityType: model.PartnerEntityCompany, Status: model.EnableStatusOn, CreditDays: 0,
	}
	for _, p := range []*model.Partner{customer, supplier} {
		if err := partnerRepo.Create(ctx, p); err != nil {
			t.Fatalf("建主体失败: %v", err)
		}
	}

	salesOrderRepo := repository.NewSalesOrderRepository(db)
	salesOrder := &model.SalesOrder{
		DocNo: "SO-202610-0001", PartnerID: customer.ID, TradeTerm: model.TradeTermFOB,
		Status: model.DocumentStatusConfirmed, OrderDate: model.DateOnly(now),
		CurrencyCode: "USD", ExchangeRate: money.MustParse("7.1"),
		AmountTotal: money.MustParse("1000"), AmountBaseTotal: money.MustParse("7100"),
	}
	if err := salesOrderRepo.CreateWithItems(ctx, salesOrder, nil); err != nil {
		t.Fatalf("建销售订单失败: %v", err)
	}

	purchaseRepo := repository.NewPurchaseOrderRepository(db)
	purchase := &model.PurchaseOrder{
		DocNo: "PO-2610-001", SupplierID: supplier.ID, TradeTerm: model.TradeTermEXW,
		Status: model.DocumentStatusConfirmed, OrderDate: model.DateOnly(now),
		CurrencyCode: "CNY", ExchangeRate: money.FromInt(1),
		AmountTotal: money.MustParse("3000"), AmountBaseTotal: money.MustParse("3000"),
	}
	if err := purchaseRepo.CreateWithItems(ctx, purchase, nil); err != nil {
		t.Fatalf("建采购订单失败: %v", err)
	}

	receivables := NewReceivableService(
		repository.NewReceivableRepository(db), salesOrderRepo, partnerRepo, docNoSvc,
		WithReceivableClock(clock))
	payables := NewPayableService(
		repository.NewPayableRepository(db), purchaseRepo, partnerRepo, docNoSvc,
		WithPayableClock(clock))

	rc, err := receivables.Create(ctx, dto.ReceivableInput{
		SourceType: int8(model.SettlementSourceSalesOrder),
		SourceID:   salesOrder.ID,
	})
	if err != nil {
		t.Fatalf("开应收失败: %v", err)
	}
	py, err := payables.Create(ctx, dto.PayableInput{
		SourceType: int8(model.SettlementSourcePurchaseOrder),
		SourceID:   purchase.ID,
	})
	if err != nil {
		t.Fatalf("开应付失败: %v", err)
	}

	return paymentFixture{
		payments: NewPaymentService(
			repository.NewPaymentRepository(db),
			repository.NewReceivableRepository(db),
			repository.NewPayableRepository(db),
			currencySvc, docNoSvc,
			WithPaymentClock(clock)),
		receivables:  receivables,
		payables:     payables,
		db:           db,
		customerID:   customer.ID,
		supplierID:   supplier.ID,
		receivableID: rc.ID,
		payableID:    py.ID,
		payDate:      payDate,
	}
}

func (f paymentFixture) receivable(t *testing.T, id uint) model.Receivable {
	t.Helper()
	var m model.Receivable
	if err := f.db.First(&m, id).Error; err != nil {
		t.Fatalf("读应收单失败: %v", err)
	}
	return m
}

// TestPaymentUsesPayDateRate 是 T-21 的核心断言：收款按**到账日**汇率折本币，
// 而不是照抄应收单上锁的 7.1。差额（7600 - 7100 = 500）就是汇兑损益的原料。
func TestPaymentUsesPayDateRate(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 10, 28, 9, 0, 0, 0, time.Local)
	f := newPaymentFixture(t, now)

	m, err := f.payments.Create(ctx, dto.PaymentInput{
		Direction: int8(model.PaymentDirectionIn),
		PayDate:   f.payDate.Format("2006-01-02"),
		Allocations: []dto.PaymentAllocationInput{{
			TargetType: int8(model.SettlementTargetReceivable),
			TargetID:   f.receivableID,
			Amount:     money.MustParse("1000"),
		}},
	})
	if err != nil {
		t.Fatalf("登记收款失败: %v", err)
	}

	if m.ExchangeRate.Cmp(money.MustParse("7.6")) != 0 {
		t.Errorf("汇率应取到账日的 7.6，得到 %s", m.ExchangeRate)
	}
	if want := money.MustParse("7600"); m.AmountBase.Cmp(want) != 0 {
		t.Errorf("本币金额应为 %s，得到 %s", want, m.AmountBase)
	}
	if m.CurrencyCode != "USD" || m.PartnerID != f.customerID {
		t.Errorf("币种与对方主体应从核销目标带出，得到 %s / %d", m.CurrencyCode, m.PartnerID)
	}
	if m.Amount.Cmp(money.MustParse("1000")) != 0 {
		t.Errorf("金额留空应取核销合计 1000，得到 %s", m.Amount)
	}
	// 单号与核销行的单号摊平值。
	if len(m.DocNo) < 3 || m.DocNo[:3] != "PM-" {
		t.Errorf("单号应以 PM- 开头，得到 %q", m.DocNo)
	}
	if len(m.Allocations) != 1 {
		t.Fatalf("应有 1 条核销行，得到 %d", len(m.Allocations))
	}
	if a := m.Allocations[0]; a.TargetDocNo == "" ||
		a.AmountBase.Cmp(money.MustParse("7600")) != 0 {
		t.Errorf("核销行应带目标单号与按到账汇率折的本币金额，得到 %q / %s",
			a.TargetDocNo, a.AmountBase)
	}

	rc := f.receivable(t, f.receivableID)
	if rc.SettledAmount.Cmp(money.MustParse("1000")) != 0 || rc.Status != model.SettlementStatusSettled {
		t.Errorf("应收应被全额核销并结清，得到已核销 %s、状态 %s",
			rc.SettledAmount, rc.Status.Label())
	}
	// 应收单自己锁的汇率不因收款而改变。
	if rc.ExchangeRate.Cmp(money.MustParse("7.1")) != 0 {
		t.Errorf("应收单的锁定汇率不应被收款改动，得到 %s", rc.ExchangeRate)
	}
}

// TestPaymentSettlesPayable：付款核销应付，走的是同一套逻辑的另一支。
// 人民币付款汇率取 1，本币金额与应付单上锁定的一致，没有汇兑损益。
func TestPaymentSettlesPayable(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 10, 28, 9, 0, 0, 0, time.Local)
	f := newPaymentFixture(t, now)

	m, err := f.payments.Create(ctx, dto.PaymentInput{
		Direction: int8(model.PaymentDirectionOut),
		Allocations: []dto.PaymentAllocationInput{{
			TargetType: int8(model.SettlementTargetPayable),
			TargetID:   f.payableID,
			Amount:     money.MustParse("3000"),
		}},
	})
	if err != nil {
		t.Fatalf("登记付款失败: %v", err)
	}
	if m.PartnerID != f.supplierID || m.CurrencyCode != "CNY" {
		t.Errorf("对方主体与币种应从应付单带出，得到 %d / %s", m.PartnerID, m.CurrencyCode)
	}
	threeK := money.MustParse("3000")
	if m.Amount.Cmp(threeK) != 0 || m.AmountBase.Cmp(threeK) != 0 ||
		m.ExchangeRate.Cmp(money.FromInt(1)) != 0 {
		t.Errorf("本币付款应保持金额一致，得到 %s / %s @%s",
			m.Amount, m.AmountBase, m.ExchangeRate)
	}

	var py model.Payable
	if err := f.db.First(&py, f.payableID).Error; err != nil {
		t.Fatalf("读应付单失败: %v", err)
	}
	if py.SettledAmount.Cmp(threeK) != 0 || py.Status != model.SettlementStatusSettled {
		t.Errorf("应付应被全额核销并结清，得到已核销 %s、状态 %s",
			py.SettledAmount, py.Status.Label())
	}
}

// TestPaymentPartialThenFull 守分批收款：第一次 400 只是部分结清，补足余额才结清。
func TestPaymentPartialThenFull(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 10, 28, 9, 0, 0, 0, time.Local)
	f := newPaymentFixture(t, now)

	if _, err := f.payments.Create(ctx, dto.PaymentInput{
		Direction: int8(model.PaymentDirectionIn),
		Allocations: []dto.PaymentAllocationInput{{
			TargetType: int8(model.SettlementTargetReceivable),
			TargetID:   f.receivableID,
			Amount:     money.MustParse("400"),
		}},
	}); err != nil {
		t.Fatalf("登记第一笔收款失败: %v", err)
	}
	if rc := f.receivable(t, f.receivableID); rc.Status != model.SettlementStatusPartial {
		t.Errorf("收到 400 应是部分结清，得到 %s", rc.Status.Label())
	}

	if _, err := f.payments.Create(ctx, dto.PaymentInput{
		Direction: int8(model.PaymentDirectionIn),
		Allocations: []dto.PaymentAllocationInput{{
			TargetType: int8(model.SettlementTargetReceivable),
			TargetID:   f.receivableID,
			Amount:     money.MustParse("600"),
		}},
	}); err != nil {
		t.Fatalf("登记第二笔收款失败: %v", err)
	}
	rc := f.receivable(t, f.receivableID)
	if rc.SettledAmount.Cmp(money.MustParse("1000")) != 0 || rc.Status != model.SettlementStatusSettled {
		t.Errorf("两笔收齐后应结清，得到已核销 %s、状态 %s",
			rc.SettledAmount, rc.Status.Label())
	}
}

// TestPaymentRejectsOverAllocation：超收必须在事务里被拦下，
// 而且拦下之后目标单据的已核销金额不能留下半截。
func TestPaymentRejectsOverAllocation(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 10, 28, 9, 0, 0, 0, time.Local)
	f := newPaymentFixture(t, now)

	if _, err := f.payments.Create(ctx, dto.PaymentInput{
		Direction: int8(model.PaymentDirectionIn),
		Allocations: []dto.PaymentAllocationInput{{
			TargetType: int8(model.SettlementTargetReceivable),
			TargetID:   f.receivableID,
			Amount:     money.MustParse("1200"),
		}},
	}); err == nil {
		t.Fatal("核销超过应收金额应被拒绝")
	}
	if rc := f.receivable(t, f.receivableID); !rc.SettledAmount.IsZero() ||
		rc.Status != model.SettlementStatusOpen {
		t.Errorf("被拒的收款不应改动应收，得到已核销 %s、状态 %s",
			rc.SettledAmount, rc.Status.Label())
	}
}

// TestPaymentDirectionMustMatchTarget：收款只能核应收，付款只能核应付。
// 一旦写反，账龄报表会把付出去的钱算成收回来。
func TestPaymentDirectionMustMatchTarget(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 10, 28, 9, 0, 0, 0, time.Local)
	f := newPaymentFixture(t, now)

	if _, err := f.payments.Create(ctx, dto.PaymentInput{
		Direction: int8(model.PaymentDirectionOut),
		Allocations: []dto.PaymentAllocationInput{{
			TargetType: int8(model.SettlementTargetReceivable),
			TargetID:   f.receivableID,
			Amount:     money.MustParse("100"),
		}},
	}); err == nil {
		t.Error("付款不应能核销应收单")
	}

	if _, err := f.payments.Create(ctx, dto.PaymentInput{
		Direction: int8(model.PaymentDirectionIn),
		Allocations: []dto.PaymentAllocationInput{{
			TargetType: int8(model.SettlementTargetPayable),
			TargetID:   f.payableID,
			Amount:     money.MustParse("100"),
		}},
	}); err == nil {
		t.Error("收款不应能核销应付单")
	}
}

// TestPaymentDeleteRollsBack：核销不是冲销，撤销就是删单——目标的已核销金额
// 与状态一起回滚，核销行一并消失。
func TestPaymentDeleteRollsBack(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 10, 28, 9, 0, 0, 0, time.Local)
	f := newPaymentFixture(t, now)

	m, err := f.payments.Create(ctx, dto.PaymentInput{
		Direction: int8(model.PaymentDirectionIn),
		Allocations: []dto.PaymentAllocationInput{{
			TargetType: int8(model.SettlementTargetReceivable),
			TargetID:   f.receivableID,
			Amount:     money.MustParse("600"),
		}},
	})
	if err != nil {
		t.Fatalf("登记收款失败: %v", err)
	}
	if rc := f.receivable(t, f.receivableID); rc.Status != model.SettlementStatusPartial {
		t.Fatalf("前置条件不成立：应收应是部分结清，得到 %s", rc.Status.Label())
	}

	if err := f.payments.Delete(ctx, m.ID); err != nil {
		t.Fatalf("删除收款失败: %v", err)
	}

	rc := f.receivable(t, f.receivableID)
	if !rc.SettledAmount.IsZero() || !rc.SettledAmountBase.IsZero() {
		t.Errorf("删除后应收的已核销金额应归零，得到 %s / %s",
			rc.SettledAmount, rc.SettledAmountBase)
	}
	if rc.Status != model.SettlementStatusOpen {
		t.Errorf("删除后应收应回到未结清，得到 %s", rc.Status.Label())
	}

	var n int64
	if err := f.db.Model(&model.PaymentAllocation{}).
		Where("payment_id = ?", m.ID).Count(&n).Error; err != nil {
		t.Fatalf("统计核销行失败: %v", err)
	}
	if n != 0 {
		t.Errorf("删除收款后核销行应一并删除，剩下 %d 条", n)
	}
	if _, err := f.payments.Get(ctx, m.ID); err == nil {
		t.Error("删除后不应还能查到这张收款单")
	}
}

// TestPaymentPrepayNeedsAmount：不核销的预收款必须显式给金额与对方，
// 否则会得到一张 0 元、也不知道是谁付的收款单。
func TestPaymentPrepayNeedsAmount(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 10, 28, 9, 0, 0, 0, time.Local)
	f := newPaymentFixture(t, now)

	if _, err := f.payments.Create(ctx, dto.PaymentInput{
		Direction: int8(model.PaymentDirectionIn),
		PartnerID: f.customerID,
	}); err == nil {
		t.Error("没有金额也没有核销的预收款应被拒绝")
	}

	m, err := f.payments.Create(ctx, dto.PaymentInput{
		Direction:    int8(model.PaymentDirectionIn),
		PartnerID:    f.customerID,
		CurrencyCode: "CNY",
		Amount:       money.MustParse("500"),
	})
	if err != nil {
		t.Fatalf("登记预收款失败: %v", err)
	}
	if len(m.Allocations) != 0 {
		t.Errorf("预收款不应带核销行，得到 %d 条", len(m.Allocations))
	}
	if m.ExchangeRate.Cmp(money.FromInt(1)) != 0 || m.AmountBase.Cmp(money.MustParse("500")) != 0 {
		t.Errorf("本币收款的汇率应为 1、本币金额 500，得到 %s / %s",
			m.ExchangeRate, m.AmountBase)
	}
	// 预收款不核销，应收单不受影响。
	if rc := f.receivable(t, f.receivableID); !rc.SettledAmount.IsZero() {
		t.Errorf("预收款不应核销应收，得到已核销 %s", rc.SettledAmount)
	}
}

// TestPaymentRejectsMixedCurrency：同一笔收付款的核销目标币种必须一致，
// 否则这笔钱的"原币金额"到底是美元还是人民币就说不清了。
func TestPaymentRejectsMixedCurrency(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 10, 28, 9, 0, 0, 0, time.Local)
	f := newPaymentFixture(t, now)

	cny := &model.Receivable{
		Settlement: model.Settlement{
			DocNo: "RC-MIX-0001", PartnerID: f.customerID,
			SourceType: model.SettlementSourceSalesOrder, SourceID: 1,
			CurrencyCode: "CNY", ExchangeRate: money.FromInt(1),
			Amount: money.MustParse("100"), AmountBase: money.MustParse("100"),
			DueDate: model.DateOnly(now), Status: model.SettlementStatusOpen,
		},
	}
	if err := f.db.Create(cny).Error; err != nil {
		t.Fatalf("建人民币应收失败: %v", err)
	}

	if _, err := f.payments.Create(ctx, dto.PaymentInput{
		Direction: int8(model.PaymentDirectionIn),
		Allocations: []dto.PaymentAllocationInput{
			{
				TargetType: int8(model.SettlementTargetReceivable), TargetID: f.receivableID,
				Amount: money.MustParse("100"),
			},
			{
				TargetType: int8(model.SettlementTargetReceivable), TargetID: cny.ID,
				Amount: money.MustParse("100"),
			},
		},
	}); err == nil {
		t.Error("币种不一致的核销应被拒绝")
	}
}

// TestPaymentRejectsOtherPartner：一笔收款只能核销同一往来单位的单据，
// 否则客户 A 的回款会去平客户 B 的账。
func TestPaymentRejectsOtherPartner(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 10, 28, 9, 0, 0, 0, time.Local)
	f := newPaymentFixture(t, now)

	// 供应商的应收单只可能来自数据错乱，但校验必须挡住它——
	// 正是这种"不该存在"的行最容易被漏检。
	other := &model.Receivable{
		Settlement: model.Settlement{
			DocNo: "RC-MIX-0002", PartnerID: f.supplierID,
			SourceType: model.SettlementSourceSalesOrder, SourceID: 1,
			CurrencyCode: "USD", ExchangeRate: money.MustParse("7.1"),
			Amount: money.MustParse("100"), AmountBase: money.MustParse("710"),
			DueDate: model.DateOnly(now), Status: model.SettlementStatusOpen,
		},
	}
	if err := f.db.Create(other).Error; err != nil {
		t.Fatalf("建另一主体的应收失败: %v", err)
	}

	if _, err := f.payments.Create(ctx, dto.PaymentInput{
		Direction: int8(model.PaymentDirectionIn),
		Allocations: []dto.PaymentAllocationInput{
			{
				TargetType: int8(model.SettlementTargetReceivable), TargetID: f.receivableID,
				Amount: money.MustParse("100"),
			},
			{
				TargetType: int8(model.SettlementTargetReceivable), TargetID: other.ID,
				Amount: money.MustParse("100"),
			},
		},
	}); err == nil {
		t.Error("跨往来单位的核销应被拒绝")
	}
}
