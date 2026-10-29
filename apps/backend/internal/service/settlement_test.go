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

type settlementFixture struct {
	receivables     ReceivableService
	payables        PayableService
	db              *gorm.DB
	customerID      uint
	supplierID      uint
	salesOrderID    uint
	draftOrderID    uint
	purchaseOrderID uint
}

// newSettlementFixture 造一套最小可用的应收应付环境：
// 客户账期 30 天、销售订单 USD 1000 @7.1、采购订单 CNY 3000。
//
// 汇率表的当日值故意设成 7.5，与订单锁定的 7.1 不同——这样"取错了源"
// 会直接表现为断言失败，而不是两边都是 7.1 时看起来都对。
func newSettlementFixture(t *testing.T, now time.Time) settlementFixture {
	t.Helper()
	ctx := context.Background()

	db := openTestDB(t,
		&model.Currency{}, &model.ExchangeRate{}, &model.Partner{},
		&model.SalesOrder{}, &model.SalesOrderItem{},
		&model.PurchaseOrder{}, &model.PurchaseOrderItem{},
		&model.DocSeq{}, &model.Receivable{}, &model.Payable{})

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
	// 当日汇率 7.5，与订单锁定的 7.1 不同。
	if _, err := NewExchangeRateService(rateRepo, currencyRepo).Create(ctx, dto.ExchangeRateInput{
		CurrencyCode:  "USD",
		Rate:          7.5,
		EffectiveDate: now.Format(rateDateLayout),
	}); err != nil {
		t.Fatalf("登记汇率失败: %v", err)
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
	mkSalesOrder := func(docNo string, status model.DocumentStatus) uint {
		o := &model.SalesOrder{
			DocNo: docNo, PartnerID: customer.ID, TradeTerm: model.TradeTermFOB,
			Status: status, OrderDate: model.DateOnly(now),
			CurrencyCode: "USD", ExchangeRate: money.MustParse("7.1"),
			AmountTotal: money.MustParse("1000"), AmountBaseTotal: money.MustParse("7100"),
		}
		if err := salesOrderRepo.CreateWithItems(ctx, o, nil); err != nil {
			t.Fatalf("建销售订单失败: %v", err)
		}
		return o.ID
	}
	salesID := mkSalesOrder("SO-202610-0001", model.DocumentStatusConfirmed)
	draftID := mkSalesOrder("SO-202610-0002", model.DocumentStatusDraft)

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

	return settlementFixture{
		receivables: NewReceivableService(
			repository.NewReceivableRepository(db), salesOrderRepo, partnerRepo, docNoSvc,
			WithReceivableClock(clock),
		),
		payables: NewPayableService(
			repository.NewPayableRepository(db), purchaseRepo, partnerRepo, docNoSvc,
			WithPayableClock(clock),
		),
		db:              db,
		customerID:      customer.ID,
		supplierID:      supplier.ID,
		salesOrderID:    salesID,
		draftOrderID:    draftID,
		purchaseOrderID: purchase.ID,
	}
}

// TestReceivableLocksOrderRate 是 F3 的验收点：汇率随单据锁定。
// 当日汇率是 7.5，而订单上锁的是 7.1 —— 应收单必须取后者，否则同一笔生意
// 在订单与应收上会给出两个本币金额，对账时无法解释。
func TestReceivableLocksOrderRate(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 10, 28, 9, 0, 0, 0, time.Local)
	f := newSettlementFixture(t, now)

	m, err := f.receivables.Create(ctx, dto.ReceivableInput{
		SourceType: int8(model.SettlementSourceSalesOrder),
		SourceID:   f.salesOrderID,
	})
	if err != nil {
		t.Fatalf("由订单生成应收失败: %v", err)
	}

	if m.Amount.Cmp(money.MustParse("1000")) != 0 || m.CurrencyCode != "USD" {
		t.Errorf("金额与币种应取订单值，得到 %s %s", m.CurrencyCode, m.Amount)
	}
	if m.ExchangeRate.Cmp(money.MustParse("7.1")) != 0 {
		t.Errorf("汇率应锁定订单上的 7.1，得到 %s", m.ExchangeRate)
	}
	if want := money.MustParse("7100"); m.AmountBase.Cmp(want) != 0 {
		t.Errorf("本币金额应为 %s，得到 %s", want, m.AmountBase)
	}
	if m.PartnerID != f.customerID {
		t.Errorf("对方主体应取订单客户 %d，得到 %d", f.customerID, m.PartnerID)
	}
	// 客户账期 30 天：到期日 = 订单日 + 30。
	if want := model.DateOnly(now).AddDate(0, 0, 30); !m.DueDate.Equal(want) {
		t.Errorf("到期日应为 %s，得到 %s",
			want.Format("2006-01-02"), m.DueDate.Format("2006-01-02"))
	}
	if len(m.DocNo) < 3 || m.DocNo[:3] != "RC-" {
		t.Errorf("单号应以 RC- 开头，得到 %q", m.DocNo)
	}
}

// TestReceivableSplit 守"一单多期"：同一张订单开两期，第二期默认取余额，
// 第三期必须被拒——来源总额是硬上限。
func TestReceivableSplit(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 10, 28, 9, 0, 0, 0, time.Local)
	f := newSettlementFixture(t, now)

	if _, err := f.receivables.Create(ctx, dto.ReceivableInput{
		SourceType: int8(model.SettlementSourceSalesOrder),
		SourceID:   f.salesOrderID,
		Amount:     money.MustParse("300"),
	}); err != nil {
		t.Fatalf("开第一期失败: %v", err)
	}

	second, err := f.receivables.Create(ctx, dto.ReceivableInput{
		SourceType: int8(model.SettlementSourceSalesOrder),
		SourceID:   f.salesOrderID,
	})
	if err != nil {
		t.Fatalf("开第二期失败: %v", err)
	}
	if second.Amount.Cmp(money.MustParse("700")) != 0 {
		t.Errorf("第二期应取余额 700，得到 %s", second.Amount)
	}

	if _, err := f.receivables.Create(ctx, dto.ReceivableInput{
		SourceType: int8(model.SettlementSourceSalesOrder),
		SourceID:   f.salesOrderID,
		Amount:     money.MustParse("1"),
	}); err == nil {
		t.Error("已全额开票后再开应被拒绝")
	}
}

func TestReceivableRejectsDraftOrder(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 10, 28, 9, 0, 0, 0, time.Local)
	f := newSettlementFixture(t, now)

	if _, err := f.receivables.Create(ctx, dto.ReceivableInput{
		SourceType: int8(model.SettlementSourceSalesOrder),
		SourceID:   f.draftOrderID,
	}); err == nil {
		t.Error("草稿订单不应能开应收")
	}
}

// TestReceivableVoidFreesQuota：作废的单不占额度，否则一次开错就再也开不出正确的那张。
func TestReceivableVoidFreesQuota(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 10, 28, 9, 0, 0, 0, time.Local)
	f := newSettlementFixture(t, now)

	first, err := f.receivables.Create(ctx, dto.ReceivableInput{
		SourceType: int8(model.SettlementSourceSalesOrder),
		SourceID:   f.salesOrderID,
	})
	if err != nil {
		t.Fatalf("开票失败: %v", err)
	}

	void := int8(model.SettlementStatusVoid)
	if _, err := f.receivables.Update(ctx, first.ID, dto.ReceivableInput{
		SourceType: int8(model.SettlementSourceSalesOrder),
		SourceID:   f.salesOrderID,
		Status:     &void,
	}); err != nil {
		t.Fatalf("作废失败: %v", err)
	}

	if _, err := f.receivables.Create(ctx, dto.ReceivableInput{
		SourceType: int8(model.SettlementSourceSalesOrder),
		SourceID:   f.salesOrderID,
	}); err != nil {
		t.Errorf("作废后应能重新开票，得到错误: %v", err)
	}
}

func TestPayableFromPurchaseOrder(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 10, 28, 9, 0, 0, 0, time.Local)
	f := newSettlementFixture(t, now)

	m, err := f.payables.Create(ctx, dto.PayableInput{
		SourceType: int8(model.SettlementSourcePurchaseOrder),
		SourceID:   f.purchaseOrderID,
	})
	if err != nil {
		t.Fatalf("由采购单生成应付失败: %v", err)
	}
	if m.PartnerID != f.supplierID {
		t.Errorf("对方主体应取供应商 %d，得到 %d", f.supplierID, m.PartnerID)
	}
	if m.Amount.Cmp(money.MustParse("3000")) != 0 || m.CurrencyCode != "CNY" ||
		m.ExchangeRate.Cmp(money.FromInt(1)) != 0 {
		t.Errorf("应付应取采购单的金额与币种，得到 %s %s @%s",
			m.CurrencyCode, m.Amount, m.ExchangeRate)
	}
	// 供应商账期 0 = 即期，到期日等于单据日。
	if want := model.DateOnly(now); !m.DueDate.Equal(want) {
		t.Errorf("即期应付的到期日应为 %s，得到 %s",
			want.Format("2006-01-02"), m.DueDate.Format("2006-01-02"))
	}
	// 来源方向不能混：采购单不能开成应收。
	if _, err := f.receivables.Create(ctx, dto.ReceivableInput{
		SourceType: int8(model.SettlementSourcePurchaseOrder),
		SourceID:   f.purchaseOrderID,
	}); err == nil {
		t.Error("采购单不应能开应收")
	}
}

// TestReceivableSettledIsLocked：收过款的单不能改也不能删，
// 否则核销记录会指向一张金额已经变了的单据。
func TestReceivableSettledIsLocked(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 10, 28, 9, 0, 0, 0, time.Local)
	f := newSettlementFixture(t, now)

	m, err := f.receivables.Create(ctx, dto.ReceivableInput{
		SourceType: int8(model.SettlementSourceSalesOrder),
		SourceID:   f.salesOrderID,
	})
	if err != nil {
		t.Fatalf("开票失败: %v", err)
	}

	// T-21 才会写这个字段；这里直接改库，模拟"已经收过 100"。
	if err := f.db.Model(&model.Receivable{}).Where("id = ?", m.ID).
		Updates(map[string]any{"settled_amount": money.MustParse("100")}).Error; err != nil {
		t.Fatalf("模拟核销失败: %v", err)
	}

	if err := f.receivables.Delete(ctx, m.ID); err == nil {
		t.Error("已有核销的应收单不应能删除")
	}
	if _, err := f.receivables.Update(ctx, m.ID, dto.ReceivableInput{
		SourceType: int8(model.SettlementSourceSalesOrder),
		SourceID:   f.salesOrderID,
		Amount:     money.MustParse("500"),
	}); err == nil {
		t.Error("已有核销的应收单不应能改金额")
	}
}
