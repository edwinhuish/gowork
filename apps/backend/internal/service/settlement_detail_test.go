package service

import (
	"context"
	"testing"
	"time"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/pkg/money"
)

// TestSettlementDetailAllocations 证明应收 / 应付详情能反查到"这笔钱是谁付的"。
//
// 详情页原本只有「已结清 12,000」这一个派生数字：它由核销表加总而来，
// 但对账时要回答的是「谁在什么时候付的、按哪个汇率折的」，
// 这些只在核销行与收付款单上，不在单据自己身上。
func TestSettlementDetailAllocations(t *testing.T) {
	now := time.Date(2026, 10, 29, 10, 0, 0, 0, time.Local)
	fx := newPaymentFixture(t, now)
	ctx := context.Background()

	// 还没收到钱：空数组，不是 nil 之外的东西——前端据此显示"还没有收款记录"。
	fresh, err := fx.receivables.Get(ctx, fx.receivableID)
	if err != nil {
		t.Fatalf("读应收详情失败: %v", err)
	}
	if len(fresh.Allocations) != 0 {
		t.Fatalf("未收款时不应有核销记录，得到 %d 条", len(fresh.Allocations))
	}

	p, err := fx.payments.Create(ctx, dto.PaymentInput{
		Direction:    10, // 收款
		PartnerID:    fx.customerID,
		PayDate:      fx.payDate.Format("2006-01-02"),
		CurrencyCode: "USD",
		Amount:       money.MustParse("400"),
		Allocations: []dto.PaymentAllocationInput{
			{TargetType: 10, TargetID: fx.receivableID, Amount: money.MustParse("400")},
		},
	})
	if err != nil {
		t.Fatalf("建收款单失败: %v", err)
	}

	got, err := fx.receivables.Get(ctx, fx.receivableID)
	if err != nil {
		t.Fatalf("读应收详情失败: %v", err)
	}
	if len(got.Allocations) != 1 {
		t.Fatalf("应有一条核销记录，得到 %d", len(got.Allocations))
	}
	a := got.Allocations[0]
	if a.PaymentDocNo != p.DocNo {
		t.Errorf("核销记录应指向收款单 %s，得到 %s", p.DocNo, a.PaymentDocNo)
	}
	if a.PaymentID != p.ID {
		t.Errorf("核销记录应带收款单 id %d，得到 %d", p.ID, a.PaymentID)
	}
	if a.Amount.Cmp(money.MustParse("400")) != 0 {
		t.Errorf("核销原币应是 400，得到 %s", a.Amount)
	}
	// 汇率取**到账日**（fixture 里 7.6），不是开单锁的 7.1，也不是开单当日的 7.5：
	// 取错源，汇兑损益报表和这里的折本币金额会一起错，而对账的人只看到两个数不一样。
	if a.ExchangeRate.Cmp(money.MustParse("7.6")) != 0 {
		t.Errorf("核销记录应带到账日汇率 7.6，得到 %s", a.ExchangeRate)
	}

	// 删掉收款单：核销行一起回滚，反查不该再列出这条指向不存在单据的记录。
	if err := fx.payments.Delete(ctx, p.ID); err != nil {
		t.Fatalf("删除收款单失败: %v", err)
	}
	after, err := fx.receivables.Get(ctx, fx.receivableID)
	if err != nil {
		t.Fatalf("删除收款单后读应收详情失败: %v", err)
	}
	if len(after.Allocations) != 0 {
		t.Fatalf("收款单删除后不应再有核销记录，得到 %d 条", len(after.Allocations))
	}
	if after.SettledAmount.Cmp(money.Zero()) != 0 {
		t.Errorf("删除收款单后已结清金额应回滚为 0，得到 %s", after.SettledAmount)
	}
}

// TestPayableDetailAllocations 是应收那一侧的镜像：付款也要能反查。
// 两张表分开，反查的 target_type 写反了不会编译报错，只会静默查不到。
func TestPayableDetailAllocations(t *testing.T) {
	now := time.Date(2026, 10, 29, 10, 0, 0, 0, time.Local)
	fx := newPaymentFixture(t, now)
	ctx := context.Background()

	p, err := fx.payments.Create(ctx, dto.PaymentInput{
		Direction:    20, // 付款
		PartnerID:    fx.supplierID,
		PayDate:      fx.payDate.Format("2006-01-02"),
		CurrencyCode: "CNY",
		Amount:       money.MustParse("3000"),
		Allocations: []dto.PaymentAllocationInput{
			{TargetType: 20, TargetID: fx.payableID, Amount: money.MustParse("3000")},
		},
	})
	if err != nil {
		t.Fatalf("建付款单失败: %v", err)
	}

	got, err := fx.payables.Get(ctx, fx.payableID)
	if err != nil {
		t.Fatalf("读应付详情失败: %v", err)
	}
	if len(got.Allocations) != 1 {
		t.Fatalf("应有一条付款记录，得到 %d", len(got.Allocations))
	}
	if got.Allocations[0].PaymentDocNo != p.DocNo {
		t.Errorf("付款记录应指向付款单 %s，得到 %s", p.DocNo, got.Allocations[0].PaymentDocNo)
	}
}
