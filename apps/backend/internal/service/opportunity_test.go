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

type opptyFixture struct {
	svc        OpportunityService
	partners   repository.PartnerRepository
	currencies repository.CurrencyRepository
	rates      repository.ExchangeRateRepository
}

func newOpportunityFixture(t *testing.T, now time.Time) opptyFixture {
	t.Helper()
	db := openTestDB(t,
		&model.Currency{}, &model.ExchangeRate{}, &model.Partner{}, &model.Opportunity{})

	currencyRepo := repository.NewCurrencyRepository(db)
	rateRepo := repository.NewExchangeRateRepository(db)
	currencySvc := NewCurrencyService(currencyRepo, rateRepo)
	clock := func() time.Time { return now }

	cny, err := currencySvc.Create(context.Background(), dto.CurrencyInput{
		Code: "CNY", Name: "人民币", IsBase: boolPtr(true),
	})
	if err != nil {
		t.Fatalf("建本币失败: %v", err)
	}
	_ = cny
	if _, err := currencySvc.Create(context.Background(), dto.CurrencyInput{
		Code: "USD", Name: "美元",
	}); err != nil {
		t.Fatalf("建外币失败: %v", err)
	}
	if _, err := NewExchangeRateService(rateRepo, currencyRepo).Create(context.Background(),
		dto.ExchangeRateInput{
			CurrencyCode:  "USD",
			Rate:          7.1,
			EffectiveDate: now.Format(rateDateLayout),
		}); err != nil {
		t.Fatalf("登记汇率失败: %v", err)
	}

	return opptyFixture{
		svc: NewOpportunityService(
			repository.NewOpportunityRepository(db),
			repository.NewPartnerRepository(db),
			currencySvc,
			WithOpportunityClock(clock),
		),
		partners:   repository.NewPartnerRepository(db),
		currencies: currencyRepo,
		rates:      rateRepo,
	}
}

func boolPtr(v bool) *bool { return &v }
func intPtr(v int) *int    { return &v }

func TestOpportunityRules(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 10, 28, 10, 0, 0, 0, time.Local)
	fx := newOpportunityFixture(t, now)

	partner := &model.Partner{
		Code: "C001", Name: "Ferreteria El Sol", Role: model.PartnerRoleCustomer,
		EntityType: model.PartnerEntityCompany, Status: model.EnableStatusOn,
	}
	if err := fx.partners.Create(ctx, partner); err != nil {
		t.Fatalf("建主体失败: %v", err)
	}

	base := func() dto.OpportunityInput {
		return dto.OpportunityInput{
			PartnerID:         partner.ID,
			Subject:           "2026 秋季五金补货",
			Stage:             int8(model.OpportunityStageQuoted),
			Amount:            money.MustParse("1000"),
			CurrencyCode:      "USD",
			ExpectedCloseDate: "2026-11-30",
		}
	}

	t.Run("外币金额落本币与汇率", func(t *testing.T) {
		m, err := fx.svc.Create(ctx, base())
		if err != nil {
			t.Fatalf("新建商机失败: %v", err)
		}
		if m.AmountBase.Cmp(money.MustParse("7100")) != 0 {
			t.Errorf("本币金额 = %s，期望 7100", m.AmountBase)
		}
		if m.ExchangeRate.Cmp(money.MustParse("7.1")) != 0 {
			t.Errorf("汇率 = %s，期望 7.1", m.ExchangeRate)
		}
		if m.Probability != 50 {
			t.Errorf("默认概率 = %d，期望 50（已报价）", m.Probability)
		}
	})

	t.Run("手填概率生效", func(t *testing.T) {
		in := base()
		in.Probability = intPtr(80)
		m, err := fx.svc.Create(ctx, in)
		if err != nil {
			t.Fatalf("新建商机失败: %v", err)
		}
		if m.Probability != 80 {
			t.Errorf("概率 = %d，期望 80", m.Probability)
		}
	})

	t.Run("赢单概率恒为 100", func(t *testing.T) {
		in := base()
		in.Stage = int8(model.OpportunityStageWon)
		in.Probability = intPtr(30)
		m, err := fx.svc.Create(ctx, in)
		if err != nil {
			t.Fatalf("新建商机失败: %v", err)
		}
		if m.Probability != 100 {
			t.Errorf("赢单概率 = %d，期望 100", m.Probability)
		}

		// 终态不可再改：要重启就新建一条，别把去年的赢单改回谈判中。
		in.Stage = int8(model.OpportunityStageNegotiating)
		if _, err := fx.svc.Update(ctx, m.ID, in); err == nil {
			t.Fatal("赢单改回谈判中应被拒绝")
		} else if e, ok := errs.From(err); !ok || e.Code != errs.CodeConflict {
			t.Fatalf("期望 40901，实际 %v", err)
		}
	})

	t.Run("阶段可以回退", func(t *testing.T) {
		m, err := fx.svc.Create(ctx, base())
		if err != nil {
			t.Fatalf("新建商机失败: %v", err)
		}
		in := base()
		in.Stage = int8(model.OpportunityStageNeed)
		updated, err := fx.svc.Update(ctx, m.ID, in)
		if err != nil {
			t.Fatalf("已报价退回需求确认应被允许，实际 %v", err)
		}
		if updated.Stage != model.OpportunityStageNeed {
			t.Errorf("阶段 = %d，期望 20", updated.Stage)
		}
	})

	t.Run("留空币种按本币", func(t *testing.T) {
		in := base()
		in.Amount = money.MustParse("500")
		in.CurrencyCode = ""
		m, err := fx.svc.Create(ctx, in)
		if err != nil {
			t.Fatalf("新建商机失败: %v", err)
		}
		if m.CurrencyCode != "CNY" ||
			m.AmountBase.Cmp(money.MustParse("500")) != 0 ||
			m.ExchangeRate.Cmp(money.FromInt(1)) != 0 {
			t.Errorf("留空币种应落本币 1:1，实际 %s / %s / %s",
				m.CurrencyCode, m.AmountBase, m.ExchangeRate)
		}
	})

	t.Run("非法阶段被拒", func(t *testing.T) {
		in := base()
		in.Stage = 99
		if _, err := fx.svc.Create(ctx, in); err == nil {
			t.Fatal("非法阶段应被拒绝")
		}
	})

	// 加权金额必须是定点相加：float64 下 33 + 0.1 会得到 33.099999999999994，
	// 而看板上的数就是客户与销售总监对账时看的那一个。
	t.Run("加权金额按定点算", func(t *testing.T) {
		sum := model.Opportunity{
			AmountBase: money.MustParse("100"), Probability: 33,
		}.WeightedAmount()
		sum = sum.Add(model.Opportunity{
			AmountBase: money.MustParse("0.1"), Probability: 100,
		}.WeightedAmount())
		if got := sum.Round(2).StringFixed(2); got != "33.10" {
			t.Errorf("加权金额合计 = %s，期望 33.10", got)
		}
	})

	t.Run("按阶段汇总", func(t *testing.T) {
		// 此时库里有：已报价 ×3（1000 USD / 1000 USD / 1000 USD 中的两条手填概率、赢单）、
		// 需求确认 ×1、本币 ×1。汇总只校验口径：条数与加权金额不相加原币。
		summary, err := fx.svc.Summary(ctx, dto.OpportunityQuery{PageQuery: dto.PageQuery{}})
		if err != nil {
			t.Fatalf("汇总失败: %v", err)
		}
		if summary.TotalCount < 5 {
			t.Fatalf("汇总条数 = %d，期望至少 5", summary.TotalCount)
		}
		weighted := money.Zero()
		for _, s := range summary.Stages {
			weighted = weighted.Add(s.Weighted)
		}
		if weighted.Cmp(summary.TotalWeighted) != 0 {
			t.Errorf("各阶段加权合计 %s 与总计 %s 不一致", weighted, summary.TotalWeighted)
		}
		if summary.TotalWeighted.Cmp(summary.TotalAmountBase) >= 0 {
			t.Errorf("加权金额应小于总额（概率都低于 100），实际 %s >= %s",
				summary.TotalWeighted, summary.TotalAmountBase)
		}

		// 只看未结：赢单与输单不计入。
		openOnly := int8(1)
		open, err := fx.svc.Summary(ctx, dto.OpportunityQuery{OpenOnly: &openOnly})
		if err != nil {
			t.Fatalf("汇总失败: %v", err)
		}
		if open.TotalCount >= summary.TotalCount {
			t.Errorf("未结条数 %d 应少于全部 %d", open.TotalCount, summary.TotalCount)
		}
	})
}
