package service

import (
	"context"
	"testing"
	"time"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/errs"
	"github.com/edwinhuish/gowork/internal/pkg/money"
)

// 已确认的订单不能直接改，改要走变更单：订单更新到新值，同时留一条
// "谁因为什么把哪几行从多少改成了多少"。
func TestSalesOrderChange(t *testing.T) {
	ctx := WithActor(context.Background(), &Principal{UserID: 1, Username: "alice"})
	now := time.Date(2026, 10, 29, 10, 0, 0, 0, time.Local)
	fx := newSalesOrderFixture(t, &now)

	order, err := fx.svc.Create(ctx, dto.SalesOrderInput{
		PartnerID:    fx.customerID,
		TradeTerm:    "FOB",
		CurrencyCode: "USD",
		OrderDate:    "2026-10-29",
		Items: []dto.SalesOrderItemInput{
			{ProductID: fx.productID, Qty: qtyOf(100), UnitPrice: money.MustParse("10")},
			{ProductID: fx.product2ID, Qty: qtyOf(50), UnitPrice: money.MustParse("20")},
		},
	})
	if err != nil {
		t.Fatalf("新建订单失败: %v", err)
	}

	t.Run("草稿不需要变更单", func(t *testing.T) {
		_, err := fx.svc.Change(ctx, order.ID, dto.SalesOrderChangeInput{
			Reason: "草稿直接改",
			Lines: []dto.SalesOrderChangeLineInput{
				{SalesOrderItemID: order.Items[0].ID, Qty: qtyOf(90), UnitPrice: money.MustParse("10")},
			},
		})
		if e, ok := errs.From(err); !ok || e.Code != errs.CodeConflict {
			t.Fatalf("期望 40901，实际 %v", err)
		}
	})

	confirmed, err := fx.svc.UpdateStatus(ctx, order.ID, dto.SalesOrderStatusInput{
		Status: int8(model.DocumentStatusConfirmed),
	})
	if err != nil {
		t.Fatalf("确认订单失败: %v", err)
	}

	var change *model.SalesOrderChange
	t.Run("改量改价留痕", func(t *testing.T) {
		change, err = fx.svc.Change(ctx, order.ID, dto.SalesOrderChangeInput{
			Reason: "客户砍量，第二行同步谈价",
			Lines: []dto.SalesOrderChangeLineInput{
				{SalesOrderItemID: confirmed.Items[0].ID, Qty: qtyOf(80), UnitPrice: money.MustParse("10")},
				{SalesOrderItemID: confirmed.Items[1].ID, Qty: qtyOf(50), UnitPrice: money.MustParse("18")},
			},
		})
		if err != nil {
			t.Fatalf("变更失败: %v", err)
		}
		if change.Seq != 1 || change.DocNo != confirmed.DocNo+"-CH1" {
			t.Errorf("序号/单号 = %d/%s，期望 1/%s-CH1", change.Seq, change.DocNo, confirmed.DocNo)
		}
		if change.Operator != "alice" {
			t.Errorf("操作人 = %q，期望 alice", change.Operator)
		}
		if change.BeforeTotal.Cmp(money.MustParse("2000")) != 0 ||
			change.AfterTotal.Cmp(money.MustParse("1700")) != 0 {
			t.Errorf("变更前后总额 = %s/%s，期望 2000/1700", change.BeforeTotal, change.AfterTotal)
		}
		if len(change.Lines) != 2 {
			t.Fatalf("变更明细应有 2 行，得到 %d", len(change.Lines))
		}

		// 订单本身也更新了：合计由明细汇总，不是前端传的。
		got, err := fx.svc.Get(ctx, order.ID)
		if err != nil {
			t.Fatalf("读回订单: %v", err)
		}
		if got.AmountTotal.Cmp(money.MustParse("1700")) != 0 {
			t.Errorf("订单合计 = %s，期望 1700", got.AmountTotal)
		}
		if got.Items[0].Qty.Cmp(qtyOf(80)) != 0 {
			t.Errorf("第一行数量 = %s，期望 80", got.Items[0].Qty)
		}
		if got.Items[0].Amount.Cmp(money.MustParse("800")) != 0 {
			t.Errorf("第一行金额 = %s，期望 800", got.Items[0].Amount)
		}
		if got.AmountBaseTotal.Cmp(money.MustParse("12070")) != 0 {
			t.Errorf("订单本币合计 = %s，期望 12070（1700 × 7.1）", got.AmountBaseTotal)
		}
	})

	t.Run("历史按序号倒序可查", func(t *testing.T) {
		list, err := fx.svc.ListChanges(ctx, order.ID)
		if err != nil {
			t.Fatalf("查变更历史失败: %v", err)
		}
		if len(list) != 1 || list[0].Seq != 1 {
			t.Fatalf("变更历史 = %d 条，期望 1 条且 seq=1", len(list))
		}
		if len(list[0].Lines) != 2 {
			t.Errorf("历史明细应有 2 行，得到 %d", len(list[0].Lines))
		}
	})

	t.Run("没有实际改动时拒绝", func(t *testing.T) {
		_, err := fx.svc.Change(ctx, order.ID, dto.SalesOrderChangeInput{
			Reason: "什么都没改",
			Lines: []dto.SalesOrderChangeLineInput{
				{SalesOrderItemID: confirmed.Items[0].ID, Qty: qtyOf(80), UnitPrice: money.MustParse("10")},
			},
		})
		if e, ok := errs.From(err); !ok || e.Code != errs.CodeInvalidParam {
			t.Fatalf("期望 40001，实际 %v", err)
		}
	})

	t.Run("取不到操作人时拒绝", func(t *testing.T) {
		_, err := fx.svc.Change(context.Background(), order.ID, dto.SalesOrderChangeInput{
			Reason: "匿名改单",
			Lines: []dto.SalesOrderChangeLineInput{
				{SalesOrderItemID: confirmed.Items[0].ID, Qty: qtyOf(70), UnitPrice: money.MustParse("10")},
			},
		})
		if e, ok := errs.From(err); !ok || e.Code != errs.CodeUnauthorized {
			t.Fatalf("期望 40101，实际 %v", err)
		}
	})
}

// 变更与编辑共用同一套乐观锁：两个人各提交一次变更时，后一个不该叠加在前一个之上。
func TestSalesOrderChangeOptimisticLock(t *testing.T) {
	ctx := WithActor(context.Background(), &Principal{UserID: 1, Username: "bob"})
	now := time.Date(2026, 10, 29, 10, 0, 0, 0, time.Local)
	fx := newSalesOrderFixture(t, &now)

	order, err := fx.svc.Create(ctx, dto.SalesOrderInput{
		PartnerID: fx.customerID, TradeTerm: "FOB", CurrencyCode: "USD", OrderDate: "2026-10-29",
		Items: []dto.SalesOrderItemInput{
			{ProductID: fx.productID, Qty: qtyOf(100), UnitPrice: money.MustParse("10")},
		},
	})
	if err != nil {
		t.Fatalf("新建订单失败: %v", err)
	}
	if _, err := fx.svc.UpdateStatus(ctx, order.ID, dto.SalesOrderStatusInput{
		Status: int8(model.DocumentStatusConfirmed),
	}); err != nil {
		t.Fatalf("确认失败: %v", err)
	}
	confirmed, err := fx.svc.Get(ctx, order.ID)
	if err != nil {
		t.Fatalf("读回订单: %v", err)
	}
	stale := confirmed.UpdatedAt

	if _, err := fx.svc.Change(ctx, order.ID, dto.SalesOrderChangeInput{
		Reason: "第一个人改", UpdatedAt: &stale,
		Lines: []dto.SalesOrderChangeLineInput{
			{SalesOrderItemID: confirmed.Items[0].ID, Qty: qtyOf(90), UnitPrice: money.MustParse("10")},
		},
	}); err != nil {
		t.Fatalf("第一次变更应成功: %v", err)
	}

	time.Sleep(2 * time.Millisecond)

	if _, err := fx.svc.Change(ctx, order.ID, dto.SalesOrderChangeInput{
		Reason: "第二个人改", UpdatedAt: &stale,
		Lines: []dto.SalesOrderChangeLineInput{
			{SalesOrderItemID: confirmed.Items[0].ID, Qty: qtyOf(60), UnitPrice: money.MustParse("10")},
		},
	}); err == nil {
		t.Fatal("拿过期版本提交变更应被拒绝")
	} else if e, ok := errs.From(err); !ok || e.Code != errs.CodeConflict {
		t.Fatalf("期望 40901，实际 %v", err)
	}
}
