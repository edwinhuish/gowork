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

// newMilestoneOrder 建一张下单日在过去的采购单，返回订单与它唯一的一行。
func newMilestoneOrder(t *testing.T, f purchaseFixture) (*model.PurchaseOrder, uint) {
	t.Helper()
	ctx := context.Background()
	o, err := f.svc.Create(ctx, dto.PurchaseOrderInput{
		SupplierID: f.supplierID, TradeTerm: "FOB", CurrencyCode: "CNY",
		OrderDate: "2026-10-01",
		Items: []dto.PurchaseOrderItemInput{
			{ProductID: f.productID, Qty: money.MustParse("100"), UnitPrice: money.MustParse("8")},
		},
	})
	if err != nil {
		t.Fatalf("建采购单失败: %v", err)
	}
	return o, o.Items[0].ID
}

// 节点可单独补录，且留操作人；只提交一条不影响别的节点。
func TestSaveMilestonesPartial(t *testing.T) {
	now := time.Date(2026, 10, 28, 10, 0, 0, 0, time.Local)
	f := newPurchaseFixture(t, &now)
	ctx := WithActor(context.Background(), scopeActor(7, model.RoleStaff))
	order, itemID := newMilestoneOrder(t, f)

	rows, err := f.svc.SaveMilestones(ctx, order.ID, dto.PurchaseMilestoneSaveInput{
		Milestones: []dto.PurchaseMilestoneInput{
			{PurchaseOrderItemID: itemID, Node: 20, PlanDate: "2026-10-20", DoneDate: "2026-10-18"},
		},
	})
	if err != nil {
		t.Fatalf("补录失败: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("应有一条节点，得到 %d", len(rows))
	}
	if rows[0].DoneDate == nil || model.DateOnly(*rows[0].DoneDate).Format("2006-01-02") != "2026-10-18" {
		t.Errorf("完成日不对: %+v", rows[0].DoneDate)
	}
	if rows[0].Operator != "tester" {
		// 完成日期非空才记补录人：只排计划的人不该被记成"完成人"。
		t.Logf("操作人为 %q（由 scopeActor 的 username 决定）", rows[0].Operator)
	}

	// 再补一条别的节点：前面那条必须原样还在。
	rows, err = f.svc.SaveMilestones(ctx, order.ID, dto.PurchaseMilestoneSaveInput{
		Milestones: []dto.PurchaseMilestoneInput{
			{PurchaseOrderItemID: itemID, Node: 30, PlanDate: "2026-10-25"},
		},
	})
	if err != nil {
		t.Fatalf("第二次补录失败: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("应累计两条节点（部分提交不该覆盖别的），得到 %d", len(rows))
	}
	for _, row := range rows {
		if row.Node == model.PurchaseNodeProduced && row.DoneDate == nil {
			t.Error("第一次补录的完成日被第二次请求清掉了")
		}
		if row.Node == model.PurchaseNodeInspected && row.Operator != "" {
			t.Error("只有计划、没有完成日时不该记补录人")
		}
	}
}

// 出运早于下单必须拒：一旦落库，逾期视图与账期都会跟着错。
func TestSaveMilestonesRejectsShippedBeforeOrdered(t *testing.T) {
	now := time.Date(2026, 10, 28, 10, 0, 0, 0, time.Local)
	f := newPurchaseFixture(t, &now)
	ctx := context.Background()
	order, itemID := newMilestoneOrder(t, f)

	// 下单日 2026-10-01；出运写成 2026-09-20。
	_, err := f.svc.SaveMilestones(ctx, order.ID, dto.PurchaseMilestoneSaveInput{
		Milestones: []dto.PurchaseMilestoneInput{
			{PurchaseOrderItemID: itemID, Node: 50, DoneDate: "2026-09-20"},
		},
	})
	wantCode(t, err, errs.CodeInvalidParam, "出运早于下单")

	// 边界：等于下单日应当允许。
	if _, err := f.svc.SaveMilestones(ctx, order.ID, dto.PurchaseMilestoneSaveInput{
		Milestones: []dto.PurchaseMilestoneInput{
			{PurchaseOrderItemID: itemID, Node: 50, DoneDate: "2026-10-01"},
		},
	}); err != nil {
		t.Errorf("等于下单日应当允许，得到 %v", err)
	}
}

func TestSaveMilestonesRejects(t *testing.T) {
	now := time.Date(2026, 10, 28, 10, 0, 0, 0, time.Local)
	f := newPurchaseFixture(t, &now)
	ctx := context.Background()
	order, itemID := newMilestoneOrder(t, f)

	t.Run("节点非法", func(t *testing.T) {
		_, err := f.svc.SaveMilestones(ctx, order.ID, dto.PurchaseMilestoneSaveInput{
			Milestones: []dto.PurchaseMilestoneInput{
				{PurchaseOrderItemID: itemID, Node: 15},
			},
		})
		wantCode(t, err, errs.CodeInvalidParam, "节点取值")
	})

	t.Run("明细不属于该订单", func(t *testing.T) {
		_, err := f.svc.SaveMilestones(ctx, order.ID, dto.PurchaseMilestoneSaveInput{
			Milestones: []dto.PurchaseMilestoneInput{
				{PurchaseOrderItemID: 9999, Node: 20},
			},
		})
		wantCode(t, err, errs.CodeInvalidParam, "明细归属")
	})

	t.Run("同一节点提交两次", func(t *testing.T) {
		_, err := f.svc.SaveMilestones(ctx, order.ID, dto.PurchaseMilestoneSaveInput{
			Milestones: []dto.PurchaseMilestoneInput{
				{PurchaseOrderItemID: itemID, Node: 20, PlanDate: "2026-10-10"},
				{PurchaseOrderItemID: itemID, Node: 20, PlanDate: "2026-10-11"},
			},
		})
		wantCode(t, err, errs.CodeInvalidParam, "重复节点")
	})

	t.Run("订单不存在", func(t *testing.T) {
		_, err := f.svc.SaveMilestones(ctx, 9999, dto.PurchaseMilestoneSaveInput{
			Milestones: []dto.PurchaseMilestoneInput{
				{PurchaseOrderItemID: itemID, Node: 20},
			},
		})
		wantCode(t, err, errs.CodeNotFound, "订单不存在")
	})
}

// 逾期视图：计划日已过且未完成才算；列表按节点筛，并投影最靠前的那个。
func TestPurchaseOverdueNodeFilter(t *testing.T) {
	now := time.Date(2026, 10, 28, 10, 0, 0, 0, time.Local)
	f := newPurchaseFixture(t, &now)
	ctx := context.Background()
	order, itemID := newMilestoneOrder(t, f)

	// 验货计划 10-20（已过且未完成）、入仓计划 11-30（未到期）。
	if _, err := f.svc.SaveMilestones(ctx, order.ID, dto.PurchaseMilestoneSaveInput{
		Milestones: []dto.PurchaseMilestoneInput{
			{PurchaseOrderItemID: itemID, Node: 30, PlanDate: "2026-10-20"},
			{PurchaseOrderItemID: itemID, Node: 40, PlanDate: "2026-11-30"},
		},
	}); err != nil {
		t.Fatalf("补录失败: %v", err)
	}

	node30 := int8(model.PurchaseNodeInspected)
	res, err := f.svc.List(ctx, dto.PurchaseOrderQuery{OverdueNode: &node30})
	if err != nil {
		t.Fatalf("按逾期节点筛失败: %v", err)
	}
	if res.Total != 1 {
		t.Fatalf("验货逾期应筛出 1 张，得到 %d", res.Total)
	}
	if res.List[0].OverdueNode != "验货" {
		t.Errorf("应投影出最靠前的逾期节点「验货」，得到 %q", res.List[0].OverdueNode)
	}

	// 未到期的节点不算逾期。
	node40 := int8(model.PurchaseNodeWarehoused)
	if got, err := f.svc.List(ctx, dto.PurchaseOrderQuery{OverdueNode: &node40}); err != nil {
		t.Fatalf("按未到期节点筛失败: %v", err)
	} else if got.Total != 0 {
		t.Errorf("计划日未到不该算逾期，得到 %d 张", got.Total)
	}

	// 补上完成日之后就退出逾期视图。
	if _, err := f.svc.SaveMilestones(ctx, order.ID, dto.PurchaseMilestoneSaveInput{
		Milestones: []dto.PurchaseMilestoneInput{
			{PurchaseOrderItemID: itemID, Node: 30, PlanDate: "2026-10-20", DoneDate: "2026-10-21"},
		},
	}); err != nil {
		t.Fatalf("补完成日失败: %v", err)
	}
	if got, err := f.svc.List(ctx, dto.PurchaseOrderQuery{OverdueNode: &node30}); err != nil {
		t.Fatalf("再筛失败: %v", err)
	} else if got.Total != 0 {
		t.Errorf("已完成的节点不该再逾期，得到 %d 张", got.Total)
	}

	// 不筛的时候，逾期投影仍然带出来（列表上那一列）。
	all, err := f.svc.List(ctx, dto.PurchaseOrderQuery{})
	if err != nil {
		t.Fatalf("查列表失败: %v", err)
	}
	if len(all.List) != 1 || all.List[0].OverdueNode != "" {
		t.Errorf("没有逾期节点时不该投影，得到 %q", all.List[0].OverdueNode)
	}
}

func TestPurchaseOverdueNodeRejectsInvalid(t *testing.T) {
	now := time.Date(2026, 10, 28, 10, 0, 0, 0, time.Local)
	f := newPurchaseFixture(t, &now)
	bad := int8(15)
	_, err := f.svc.List(context.Background(), dto.PurchaseOrderQuery{OverdueNode: &bad})
	wantCode(t, err, errs.CodeInvalidParam, "非法节点")
}
