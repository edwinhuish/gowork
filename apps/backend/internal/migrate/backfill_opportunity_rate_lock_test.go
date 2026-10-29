package migrate

import (
	"log/slog"
	"testing"
	"time"

	"github.com/edwinhuish/gowork/internal/infra/database"
	"github.com/edwinhuish/gowork/internal/model"
)

func TestBackfillOpportunityRateLockedOn(t *testing.T) {
	db := newTestDB(t)
	log := slog.New(slog.DiscardHandler)

	if err := database.AutoMigrate(db, &model.Opportunity{}); err != nil {
		t.Fatalf("建商机表: %v", err)
	}

	// 存量行：rate_locked_on 是 NULL（列是后加的）。折算日要补成录入那天，
	// 不是"今天"——那批预测确实是那天算的。
	old := time.Date(2026, 3, 15, 9, 30, 0, 0, time.Local)
	kept := &model.Opportunity{
		PartnerID: 1, Subject: "存量商机", Stage: model.OpportunityStageQuoted,
		CurrencyCode: "CNY", ExpectedCloseDate: old,
	}
	if err := db.Create(kept).Error; err != nil {
		t.Fatalf("写入商机: %v", err)
	}
	if err := db.Model(&model.Opportunity{}).Where("id = ?", kept.ID).
		Update("created_at", old).Update("rate_locked_on", nil).Error; err != nil {
		t.Fatalf("造存量数据: %v", err)
	}

	// 已经有了折算日的行不许被覆盖：它是重估留下的真实日期。
	locked := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	already := &model.Opportunity{
		PartnerID: 1, Subject: "已重估过的商机", Stage: model.OpportunityStageNegotiating,
		CurrencyCode: "CNY", ExpectedCloseDate: old, RateLockedOn: locked,
	}
	if err := db.Create(already).Error; err != nil {
		t.Fatalf("写入商机: %v", err)
	}

	if err := BackfillOpportunityRateLockedOn(db, log); err != nil {
		t.Fatalf("回填失败: %v", err)
	}

	var got model.Opportunity
	if err := db.First(&got, kept.ID).Error; err != nil {
		t.Fatalf("读取商机: %v", err)
	}
	if want := model.DateOnly(old); !got.RateLockedOn.Equal(want) {
		t.Errorf("折算日 = %s，期望 %s", got.RateLockedOn, want)
	}

	var untouched model.Opportunity
	if err := db.First(&untouched, already.ID).Error; err != nil {
		t.Fatalf("读取商机: %v", err)
	}
	if !untouched.RateLockedOn.Equal(locked) {
		t.Errorf("已有折算日被覆盖：%s，期望 %s", untouched.RateLockedOn, locked)
	}

	// 幂等：再跑一次不改任何东西，也不报错。
	if err := BackfillOpportunityRateLockedOn(db, log); err != nil {
		t.Fatalf("二次执行失败: %v", err)
	}
}
