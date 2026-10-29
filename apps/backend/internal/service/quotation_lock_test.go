package service

import (
	"context"
	"testing"
	"time"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/pkg/errs"
	"github.com/edwinhuish/gowork/internal/pkg/money"
)

// 两个人同时打开同一张草稿，先后保存：后保存的那个不该把前一个的明细整片覆盖掉。
//
// 明细是"删旧建新"，覆盖之后无从恢复——所以这里必须拒绝，而不是体贴地合并。
func TestQuotationOptimisticLock(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 10, 29, 10, 0, 0, 0, time.Local)
	fx := newQuotationFixture(t, &now)

	base := dto.QuotationInput{
		PartnerID:    fx.customerID,
		TradeTerm:    "FOB",
		CurrencyCode: "USD",
		ValidUntil:   "2026-11-30",
		Items: []dto.QuotationItemInput{
			{ProductID: fx.productID, Qty: qtyOf(100), UnitPrice: money.MustParse("10")},
		},
	}
	created, err := fx.svc.Create(ctx, base)
	if err != nil {
		t.Fatalf("新建报价单失败: %v", err)
	}
	// 这一版就是"打开页面时读到的那份底稿"。
	stale := created.UpdatedAt

	first := base
	first.Remark = "第一个人改的"
	first.UpdatedAt = &stale
	if _, err := fx.svc.Update(ctx, created.ID, first); err != nil {
		t.Fatalf("带着当前版本更新应当成功: %v", err)
	}

	// gorm 写 updated_at 用系统时间：两次更新落在同一毫秒时条件仍会命中，
	// 那样这条用例就测不到任何东西了。隔开一点，测的是语义不是时钟精度。
	time.Sleep(2 * time.Millisecond)

	second := base
	second.Remark = "第二个人改的"
	second.UpdatedAt = &stale
	if _, err := fx.svc.Update(ctx, created.ID, second); err == nil {
		t.Fatal("拿过期版本更新应当被拒绝")
	} else if e, ok := errs.From(err); !ok || e.Code != errs.CodeConflict {
		t.Fatalf("期望 40901，实际 %v", err)
	}

	// 不带版本时按普通更新处理：脚本、迁移与老客户端不该被这条规则挡住。
	third := base
	third.Remark = "不带版本"
	if _, err := fx.svc.Update(ctx, created.ID, third); err != nil {
		t.Fatalf("不带版本的更新应当成功: %v", err)
	}
	if got, _ := fx.svc.Get(ctx, created.ID); got.Remark != "不带版本" {
		t.Errorf("备注 = %q，期望 %q", got.Remark, "不带版本")
	}
}
