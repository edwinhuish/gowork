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
		&model.Currency{}, &model.ExchangeRate{}, &model.Partner{},
		&model.Opportunity{}, &model.OpportunityStageLog{})

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
			Rate:          money.MustParse("7.1"),
			EffectiveDate: now.Format(rateDateLayout),
		}); err != nil {
		t.Fatalf("登记汇率失败: %v", err)
	}

	return opptyFixture{
		svc: NewOpportunityService(
			repository.NewOpportunityRepository(db),
			repository.NewPartnerRepository(db),
			currencySvc,
			repository.NewOpportunityStageLogRepository(db),
			repository.NewTxManager(db),
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

// TestOpportunityRateLock 钉住「汇率变动不改变历史商机预测」（T-61）。
//
// 三条线：改备注不重算、改金额才按当日汇率重算、重估必须显式触发。
func TestOpportunityRateLock(t *testing.T) {
	ctx := context.Background()
	day1 := time.Date(2026, 10, 28, 10, 0, 0, 0, time.Local)
	day2 := time.Date(2026, 11, 2, 10, 0, 0, 0, time.Local)
	now := day1

	db := openTestDB(t,
		&model.Currency{}, &model.ExchangeRate{}, &model.Partner{},
		&model.Opportunity{}, &model.OpportunityStageLog{})
	currencyRepo := repository.NewCurrencyRepository(db)
	rateRepo := repository.NewExchangeRateRepository(db)
	currencySvc := NewCurrencyService(currencyRepo, rateRepo)
	rateSvc := NewExchangeRateService(rateRepo, currencyRepo)
	partnerRepo := repository.NewPartnerRepository(db)

	if _, err := currencySvc.Create(ctx, dto.CurrencyInput{
		Code: "CNY", Name: "人民币", IsBase: boolPtr(true),
	}); err != nil {
		t.Fatalf("建本币失败: %v", err)
	}
	if _, err := currencySvc.Create(ctx, dto.CurrencyInput{
		Code: "USD", Name: "美元",
	}); err != nil {
		t.Fatalf("建外币失败: %v", err)
	}
	addRate := func(day time.Time, rate string) {
		t.Helper()
		if _, err := rateSvc.Create(ctx, dto.ExchangeRateInput{
			CurrencyCode:  "USD",
			Rate:          money.MustParse(rate),
			EffectiveDate: day.Format(rateDateLayout),
		}); err != nil {
			t.Fatalf("登记汇率失败: %v", err)
		}
	}
	addRate(day1, "7.1")

	svc := NewOpportunityService(
		repository.NewOpportunityRepository(db), partnerRepo, currencySvc,
		repository.NewOpportunityStageLogRepository(db), repository.NewTxManager(db),
		WithOpportunityClock(func() time.Time { return now }),
	)

	partner := &model.Partner{
		Code: "C001", Name: "Ferreteria El Sol", Role: model.PartnerRoleCustomer,
		EntityType: model.PartnerEntityCompany, Status: model.EnableStatusOn,
	}
	if err := partnerRepo.Create(ctx, partner); err != nil {
		t.Fatalf("建主体失败: %v", err)
	}

	in := dto.OpportunityInput{
		PartnerID:         partner.ID,
		Subject:           "2026 秋季五金补货",
		Stage:             int8(model.OpportunityStageQuoted),
		Amount:            money.MustParse("1000"),
		CurrencyCode:      "USD",
		ExpectedCloseDate: "2026-11-30",
	}
	m, err := svc.Create(ctx, in)
	if err != nil {
		t.Fatalf("新建商机失败: %v", err)
	}
	wantLocked := model.DateOnly(day1)
	if !m.RateLockedOn.Equal(wantLocked) {
		t.Errorf("折算日 = %s，期望 %s", m.RateLockedOn, wantLocked)
	}

	// 汇率变了。此后任何"不改金额"的保存都不该动本币金额。
	addRate(day2, "7.3")
	now = day2

	t.Run("改备注不重算", func(t *testing.T) {
		edited := in
		edited.Remark = "客户改口，先等他们的预算"
		edited.Stage = int8(model.OpportunityStageNegotiating)
		got, err := svc.Update(ctx, m.ID, edited)
		if err != nil {
			t.Fatalf("保存失败: %v", err)
		}
		wantDecimal(t, "本币金额（改备注后）", got.AmountBase, "7100")
		wantDecimal(t, "汇率（改备注后）", got.ExchangeRate, "7.1")
		if !got.RateLockedOn.Equal(wantLocked) {
			t.Errorf("折算日被改动：%s", got.RateLockedOn)
		}
	})

	t.Run("改金额才按当日汇率重算", func(t *testing.T) {
		edited := in
		edited.Amount = money.MustParse("2000")
		got, err := svc.Update(ctx, m.ID, edited)
		if err != nil {
			t.Fatalf("保存失败: %v", err)
		}
		wantDecimal(t, "本币金额（改金额后）", got.AmountBase, "14600")
		wantDecimal(t, "汇率（改金额后）", got.ExchangeRate, "7.3")
		if !got.RateLockedOn.Equal(model.DateOnly(day2)) {
			t.Errorf("折算日 = %s，期望 %s", got.RateLockedOn, model.DateOnly(day2))
		}
	})

	t.Run("显式重估", func(t *testing.T) {
		addRate(day2.AddDate(0, 0, 5), "7.5")
		now = day2.AddDate(0, 0, 5)

		got, err := svc.Revalue(ctx, m.ID)
		if err != nil {
			t.Fatalf("重估失败: %v", err)
		}
		wantDecimal(t, "本币金额（重估后）", got.AmountBase, "15000")
		wantDecimal(t, "汇率（重估后）", got.ExchangeRate, "7.5")
	})

	t.Run("终态不再重估", func(t *testing.T) {
		won := in
		won.Stage = int8(model.OpportunityStageWon)
		closed, err := svc.Update(ctx, m.ID, won)
		if err != nil {
			t.Fatalf("置为赢单失败: %v", err)
		}
		if _, err := svc.Revalue(ctx, closed.ID); err == nil {
			t.Fatal("赢单商机重估应被拒绝")
		} else if e, ok := errs.From(err); !ok || e.Code != errs.CodeConflict {
			t.Fatalf("期望 40901，实际 %v", err)
		}
	})
}

// TestOpportunityStageLog 钉住阶段变更留痕（T-61）。
//
// 验收是「阶段变更可按时间回看」：所以既要有从哪到哪，也要有谁、什么时候；
// 而改备注这类不改变阶段的操作不该往时间线里塞噪声。
func TestOpportunityStageLog(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 10, 28, 10, 0, 0, 0, time.Local)

	db := openTestDB(t,
		&model.Currency{}, &model.ExchangeRate{}, &model.Partner{},
		&model.Opportunity{}, &model.OpportunityStageLog{})
	currencySvc := NewCurrencyService(
		repository.NewCurrencyRepository(db), repository.NewExchangeRateRepository(db))
	partnerRepo := repository.NewPartnerRepository(db)
	svc := NewOpportunityService(
		repository.NewOpportunityRepository(db), partnerRepo, currencySvc,
		repository.NewOpportunityStageLogRepository(db), repository.NewTxManager(db),
		WithOpportunityClock(func() time.Time { return now }),
	)

	partner := &model.Partner{
		Code: "C001", Name: "Ferreteria El Sol", Role: model.PartnerRoleCustomer,
		EntityType: model.PartnerEntityCompany, Status: model.EnableStatusOn,
	}
	if err := partnerRepo.Create(ctx, partner); err != nil {
		t.Fatalf("建主体失败: %v", err)
	}

	// 本币：本用例只用本币金额，折算比恒为 1，不必再登记汇率。
	if _, err := currencySvc.Create(ctx, dto.CurrencyInput{
		Code: "CNY", Name: "人民币", IsBase: boolPtr(true),
	}); err != nil {
		t.Fatalf("建本币失败: %v", err)
	}

	// 带身份的 ctx：留痕要能回答"谁改的"，匿名调用的记录没有意义。
	ctx = WithActor(ctx, &Principal{UserID: 7, Username: "amy", Nickname: "Amy 陈"})

	in := dto.OpportunityInput{
		PartnerID:         partner.ID,
		Subject:           "2026 秋季五金补货",
		Stage:             int8(model.OpportunityStageQualify),
		Amount:            money.MustParse("1000"),
		CurrencyCode:      "CNY",
		ExpectedCloseDate: "2026-11-30",
	}
	m, err := svc.Create(ctx, in)
	if err != nil {
		t.Fatalf("新建商机失败: %v", err)
	}

	logs, err := svc.StageLogs(ctx, m.ID)
	if err != nil {
		t.Fatalf("查阶段留痕失败: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("新建后应有 1 条留痕，实际 %d", len(logs))
	}
	if logs[0].FromStage != 0 || logs[0].ToStage != model.OpportunityStageQualify {
		t.Errorf("首条留痕应为 0 → 10，实际 %d → %d", logs[0].FromStage, logs[0].ToStage)
	}
	if logs[0].ActorID != 7 || logs[0].ActorName != "Amy 陈" {
		t.Errorf("操作人应为 7 / Amy 陈，实际 %d / %s", logs[0].ActorID, logs[0].ActorName)
	}

	// 改备注、改金额但不动阶段：不留痕。
	edited := in
	edited.Remark = "客户问了两次运费"
	edited.Amount = money.MustParse("1200")
	if _, err := svc.Update(ctx, m.ID, edited); err != nil {
		t.Fatalf("保存失败: %v", err)
	}
	if logs, _ = svc.StageLogs(ctx, m.ID); len(logs) != 1 {
		t.Fatalf("不动阶段的保存不该留痕，实际 %d 条", len(logs))
	}

	// 阶段前进一格：留一条，且能看出从哪来。
	next := edited
	next.Stage = int8(model.OpportunityStageQuoted)
	if _, err := svc.Update(ctx, m.ID, next); err != nil {
		t.Fatalf("改阶段失败: %v", err)
	}
	if logs, err = svc.StageLogs(ctx, m.ID); err != nil {
		t.Fatalf("查阶段留痕失败: %v", err)
	}
	if len(logs) != 2 {
		t.Fatalf("改阶段后应有 2 条留痕，实际 %d", len(logs))
	}
	if logs[1].FromStage != model.OpportunityStageQualify ||
		logs[1].ToStage != model.OpportunityStageQuoted {
		t.Errorf("第二条留痕应为 10 → 30，实际 %d → %d",
			logs[1].FromStage, logs[1].ToStage)
	}
	if logs[1].Probability != 50 {
		t.Errorf("留痕里的赢率 = %d，期望 50（已报价默认值）", logs[1].Probability)
	}

	// 不存在的商机：404，而不是空数组——两者在界面上长得不一样。
	if _, err := svc.StageLogs(ctx, 99999); err == nil {
		t.Fatal("不存在的商机应返回 404")
	}
}
