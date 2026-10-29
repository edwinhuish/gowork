package service

import (
	"context"
	"testing"
	"time"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/pkg/money"
)

// TestRemoveAllocation 是 T-52 第一块的验收点：撤销单条核销，只动这一行。
//
// 在此之前撤销核销只有一条路——删掉整张收付款单。于是一笔核销录错，
// 同一张单上其它正确的核销也一起消失，客户对账时看到的是"那笔款子没收到过"。
func TestRemoveAllocation(t *testing.T) {
	now := time.Date(2026, 10, 29, 10, 0, 0, 0, time.Local)
	fx := newPaymentFixture(t, now)
	ctx := context.Background()

	p, err := fx.payments.Create(ctx, dto.PaymentInput{
		Direction:    10, // 收款
		PartnerID:    fx.customerID,
		PayDate:      fx.payDate.Format("2006-01-02"),
		CurrencyCode: "USD",
		Amount:       money.MustParse("500"),
		Allocations: []dto.PaymentAllocationInput{
			{TargetType: 10, TargetID: fx.receivableID, Amount: money.MustParse("300")},
			{TargetType: 10, TargetID: fx.receivableID, Amount: money.MustParse("200")},
		},
	})
	if err != nil {
		t.Fatalf("建收款单失败: %v", err)
	}
	if len(p.Allocations) != 2 {
		t.Fatalf("应有两条核销行，得到 %d", len(p.Allocations))
	}

	got, err := fx.payments.RemoveAllocation(ctx, p.ID, p.Allocations[0].ID)
	if err != nil {
		t.Fatalf("撤销核销失败: %v", err)
	}

	// 整张单还在，只少了一行——这正是这条改动要证明的事。
	if len(got.Allocations) != 1 {
		t.Fatalf("撤销后应剩一条核销行，得到 %d", len(got.Allocations))
	}
	if got.Allocations[0].Amount.Cmp(money.MustParse("200")) != 0 {
		t.Errorf("剩下那条应是 200，得到 %s", got.Allocations[0].Amount)
	}

	// 目标单据仍可读：回滚不能把应收一起弄坏。
	if _, err := fx.receivables.Get(ctx, fx.receivableID); err != nil {
		t.Fatalf("撤销核销后应收单应仍可读: %v", err)
	}

	// 撤销一条已被撤销的行：404，而不是悄悄成功。
	if _, err := fx.payments.RemoveAllocation(ctx, p.ID, p.Allocations[0].ID); err == nil {
		t.Fatal("重复撤销同一行应报错")
	}

	// 拿别的单的 id 来撤销：同样是 404，不能让一个人借别的单号动核销。
	other, err := fx.payments.Create(ctx, dto.PaymentInput{
		Direction:    10,
		PartnerID:    fx.customerID,
		PayDate:      fx.payDate.Format("2006-01-02"),
		CurrencyCode: "USD",
		Amount:       money.MustParse("100"),
		Allocations: []dto.PaymentAllocationInput{
			{TargetType: 10, TargetID: fx.receivableID, Amount: money.MustParse("100")},
		},
	})
	if err != nil {
		t.Fatalf("建第二张收款单失败: %v", err)
	}
	if _, err := fx.payments.RemoveAllocation(ctx, other.ID, got.Allocations[0].ID); err == nil {
		t.Fatal("用别的收付款单 id 撤销核销应报错")
	}
}
