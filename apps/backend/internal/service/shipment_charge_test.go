package service

import (
	"context"
	"testing"
	"time"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/money"
	"github.com/edwinhuish/gowork/internal/repository"
)

type chargeFixture struct {
	charges   ShipmentChargeService
	shipments ShipmentService
	// 一票草稿、一票已确认，另有一个货代与一个客户用于收款方校验。
	draftShipmentID     uint
	confirmedShipmentID uint
	forwarderID         uint
	customerID          uint
}

func newChargeFixture(t *testing.T, now time.Time) chargeFixture {
	t.Helper()
	ctx := context.Background()

	db := openTestDB(t,
		&model.Currency{}, &model.ExchangeRate{}, &model.Partner{},
		&model.Shipment{}, &model.ShipmentAllocation{},
		&model.ShipmentCharge{}, &model.CustomsDeclaration{})

	clock := func() time.Time { return now }
	currencyRepo := repository.NewCurrencyRepository(db)
	rateRepo := repository.NewExchangeRateRepository(db)
	currencySvc := NewCurrencyService(currencyRepo, rateRepo)

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
	forwarder := &model.Partner{
		Code: "F001", Name: "中远货代", Role: model.PartnerRoleForwarder,
		EntityType: model.PartnerEntityCompany, Status: model.EnableStatusOn,
	}
	customer := &model.Partner{
		Code: "C001", Name: "Ferreteria El Sol", Role: model.PartnerRoleCustomer,
		EntityType: model.PartnerEntityCompany, Status: model.EnableStatusOn,
	}
	for _, p := range []*model.Partner{forwarder, customer} {
		if err := partnerRepo.Create(ctx, p); err != nil {
			t.Fatalf("建主体失败: %v", err)
		}
	}

	shipmentRepo := repository.NewShipmentRepository(db)
	mkShipment := func(docNo string, status model.DocumentStatus) uint {
		s := &model.Shipment{DocNo: docNo, Status: status, ShipDate: now}
		if err := shipmentRepo.CreateWithAllocations(ctx, s, nil); err != nil {
			t.Fatalf("建出货批次失败: %v", err)
		}
		return s.ID
	}
	draftID := mkShipment("SH-2610-001", model.DocumentStatusDraft)
	confirmedID := mkShipment("SH-2610-002", model.DocumentStatusConfirmed)

	chargeRepo := repository.NewShipmentChargeRepository(db)
	return chargeFixture{
		charges: NewShipmentChargeService(
			chargeRepo, shipmentRepo, partnerRepo, currencySvc,
			WithShipmentChargeClock(clock),
		),
		shipments: NewShipmentService(
			shipmentRepo, repository.NewSalesOrderRepository(db), chargeRepo,
			repository.NewCustomsDeclarationRepository(db),
			NewDocNoService(repository.NewDocSeqRepository(db), WithDocNoClock(clock)),
			repository.NewTxManager(db), stubSalesOut{t},
			WithShipmentClock(clock),
		),
		draftShipmentID:     draftID,
		confirmedShipmentID: confirmedID,
		forwarderID:         forwarder.ID,
		customerID:          customer.ID,
	}
}

// TestShipmentChargeTotals 是 T-18 的验收点：外币费用按当日汇率折本币，
// 合计是把同一票下所有费用加起来——单票毛利（T-22）减的就是这个数。
func TestShipmentChargeTotals(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 10, 28, 9, 0, 0, 0, time.Local)
	f := newChargeFixture(t, now)

	freight, err := f.charges.Create(ctx, dto.ShipmentChargeInput{
		ShipmentID:   f.draftShipmentID,
		Type:         int8(model.ShipmentChargeFreight),
		PartnerID:    &f.forwarderID,
		Amount:       money.MustParse("1200"),
		CurrencyCode: "USD",
	})
	if err != nil {
		t.Fatalf("新增海运费失败: %v", err)
	}
	if want := money.MustParse("8520"); freight.AmountBase.Cmp(want) != 0 {
		t.Errorf("海运费折本币应为 %s，得到 %s", want, freight.AmountBase)
	}
	if freight.ExchangeRate.Cmp(money.MustParse("7.1")) != 0 {
		t.Errorf("汇率应锁定为 7.1，得到 %s", freight.ExchangeRate)
	}

	// 币种留空即本币，汇率固定为 1。
	customs, err := f.charges.Create(ctx, dto.ShipmentChargeInput{
		ShipmentID: f.draftShipmentID,
		Type:       int8(model.ShipmentChargeCustoms),
		Amount:     money.MustParse("300"),
	})
	if err != nil {
		t.Fatalf("新增报关费失败: %v", err)
	}
	if customs.CurrencyCode != "CNY" ||
		customs.AmountBase.Cmp(money.MustParse("300")) != 0 ||
		customs.ExchangeRate.Cmp(money.FromInt(1)) != 0 {
		t.Errorf("本币费用应原值入账，得到 %s %s @%s",
			customs.CurrencyCode, customs.AmountBase, customs.ExchangeRate)
	}

	sum, err := f.charges.Summary(ctx, f.draftShipmentID)
	if err != nil {
		t.Fatalf("取费用合计失败: %v", err)
	}
	if want := money.MustParse("8820"); sum.TotalBase.Cmp(want) != 0 {
		t.Errorf("费用合计应为 %s，得到 %s", want, sum.TotalBase)
	}

	// 另一票的费用不能混进来。
	other, err := f.charges.Summary(ctx, f.confirmedShipmentID)
	if err != nil {
		t.Fatalf("取另一票合计失败: %v", err)
	}
	if !other.TotalBase.IsZero() {
		t.Errorf("未录费用的票合计应为 0，得到 %s", other.TotalBase)
	}
}

// TestShipmentChargeAfterConfirm 守的是这条设计：货代账单晚于出货到达，
// 已确认的票仍要能补录费用，否则只能把已发出的货退回草稿。
func TestShipmentChargeAfterConfirm(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 10, 28, 9, 0, 0, 0, time.Local)
	f := newChargeFixture(t, now)

	_, err := f.charges.Create(ctx, dto.ShipmentChargeInput{
		ShipmentID: f.confirmedShipmentID,
		Type:       int8(model.ShipmentChargeTrucking),
		Amount:     money.MustParse("800"),
	})
	if err != nil {
		t.Fatalf("已确认的出货批次应允许补录费用，得到错误: %v", err)
	}
}

func TestShipmentChargeValidation(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 10, 28, 9, 0, 0, 0, time.Local)
	f := newChargeFixture(t, now)

	cases := []struct {
		name string
		in   dto.ShipmentChargeInput
	}{
		{
			name: "批次不存在",
			in: dto.ShipmentChargeInput{
				ShipmentID: 9999, Type: int8(model.ShipmentChargeFreight),
				Amount: money.MustParse("100"),
			},
		},
		{
			name: "类型未定义",
			in: dto.ShipmentChargeInput{
				ShipmentID: f.draftShipmentID, Type: 15, Amount: money.MustParse("100"),
			},
		},
		{
			name: "金额为负",
			in: dto.ShipmentChargeInput{
				ShipmentID: f.draftShipmentID, Type: int8(model.ShipmentChargeFreight),
				Amount: money.MustParse("-1"),
			},
		},
		{
			name: "收款方是客户",
			in: dto.ShipmentChargeInput{
				ShipmentID: f.draftShipmentID, Type: int8(model.ShipmentChargeFreight),
				Amount: money.MustParse("100"), PartnerID: &f.customerID,
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := f.charges.Create(ctx, c.in); err == nil {
				t.Errorf("%s 时应拒绝创建", c.name)
			}
		})
	}
}

// TestShipmentDeleteBlockedByCharges 守的是数据完整性：删掉批次，
// 它下面的费用会变成点不进去、也对不上账的孤儿行。
func TestShipmentDeleteBlockedByCharges(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 10, 28, 9, 0, 0, 0, time.Local)
	f := newChargeFixture(t, now)

	charge, err := f.charges.Create(ctx, dto.ShipmentChargeInput{
		ShipmentID: f.draftShipmentID,
		Type:       int8(model.ShipmentChargeOther),
		Amount:     money.MustParse("50"),
	})
	if err != nil {
		t.Fatalf("新增费用失败: %v", err)
	}

	if err := f.shipments.Delete(ctx, f.draftShipmentID); err == nil {
		t.Error("有费用挂着的批次应拒绝删除")
	}

	if err := f.charges.Delete(ctx, charge.ID); err != nil {
		t.Fatalf("删除费用失败: %v", err)
	}
	if err := f.shipments.Delete(ctx, f.draftShipmentID); err != nil {
		t.Errorf("费用删完后应能删批次，得到错误: %v", err)
	}
}
