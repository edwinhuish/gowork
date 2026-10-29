package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/errs"
)

// Milestones 取一张采购单的跟单节点（T-38）。
//
// 只有详情带：列表要的是"有没有逾期"，那一个字符串就够，把整张节点表塞进列表
// 会让分页查询多出十倍的行数。
func (s *purchaseOrderService) Milestones(
	ctx context.Context, orderID uint,
) ([]model.PurchaseMilestone, error) {
	if _, err := s.orders.FindByID(ctx, orderID); err != nil {
		return nil, err
	}
	return s.milestones.ListByOrder(ctx, orderID)
}

// SaveMilestones 补录节点，**只处理提交上来的那几条**。
//
// 为什么是部分提交而不是整体替换：跟单是随手补录的——今天只填了"生产完成"，
// 明天才验货。要求前端每次把五节点全传一遍，等于逼它先读一遍再原样回传，
// 而这个过程中任何一个节点被另一个人改过，就会被这次保存悄悄覆盖掉。
func (s *purchaseOrderService) SaveMilestones(
	ctx context.Context, orderID uint, in dto.PurchaseMilestoneSaveInput,
) ([]model.PurchaseMilestone, error) {
	order, err := s.orders.FindByID(ctx, orderID)
	if err != nil {
		return nil, err
	}
	if len(in.Milestones) == 0 {
		return nil, errs.BadRequest("请至少提交一个节点")
	}

	items := make(map[uint]struct{}, len(order.Items))
	for _, item := range order.Items {
		items[item.ID] = struct{}{}
	}

	existing, err := s.milestones.ListByOrder(ctx, orderID)
	if err != nil {
		return nil, err
	}

	// 基准日：这一行认的"下单那天"。
	// 优先用下单节点的实际完成日（工厂真正开工那天可能晚于录单日），
	// 没录过就退回采购订单的下单日。
	base := model.DateOnly(order.OrderDate)
	for _, m := range existing {
		if m.Node == model.PurchaseNodeOrdered && m.DoneDate != nil {
			base = model.DateOnly(*m.DoneDate)
		}
	}
	for _, row := range in.Milestones {
		if model.PurchaseNode(row.Node) == model.PurchaseNodeOrdered && strings.TrimSpace(row.DoneDate) != "" {
			if d, err := parseDateOnly(row.DoneDate); err == nil {
				base = model.DateOnly(d)
			}
		}
	}

	operator := ""
	if actor, ok := ActorFrom(ctx); ok {
		operator = actor.Username
	}

	rows := make([]model.PurchaseMilestone, 0, len(in.Milestones))
	seen := make(map[[2]uint]struct{}, len(in.Milestones))
	for _, row := range in.Milestones {
		node := model.PurchaseNode(row.Node)
		if !node.IsValid() {
			return nil, errs.BadRequest(fmt.Sprintf("节点取值不合法：%d", row.Node))
		}
		if _, ok := items[row.PurchaseOrderItemID]; !ok {
			return nil, errs.BadRequest("这条明细不属于该采购订单")
		}
		key := [2]uint{row.PurchaseOrderItemID, uint(node)}
		if _, ok := seen[key]; ok {
			return nil, errs.BadRequest("同一明细的同一节点提交了两次")
		}
		seen[key] = struct{}{}

		plan, err := parseMilestoneDate(row.PlanDate)
		if err != nil {
			return nil, err
		}
		done, err := parseMilestoneDate(row.DoneDate)
		if err != nil {
			return nil, err
		}
		// 出运早于下单是最典型的一类错录（把"预计出运"填成了实际完成），
		// 一旦落库，逾期视图与账期都会跟着错，所以这里直接拒。
		for _, d := range []*time.Time{plan, done} {
			if d != nil && model.DateOnly(*d).Before(base) {
				return nil, errs.BadRequest(fmt.Sprintf(
					"%s 的日期（%s）早于下单日（%s）",
					node.Label(), d.Format(rateDateLayout), base.Format(rateDateLayout)))
			}
		}

		// 完成日期非空才记补录人：只排了计划的人不该被记成"完成人"。
		rowOperator := ""
		if done != nil {
			rowOperator = operator
		}

		rows = append(rows, model.PurchaseMilestone{
			PurchaseOrderID:     orderID,
			PurchaseOrderItemID: row.PurchaseOrderItemID,
			Node:                node,
			PlanDate:            plan,
			DoneDate:            done,
			Operator:            rowOperator,
			Remark:              strings.TrimSpace(row.Remark),
		})
	}

	if err := s.milestones.Upsert(ctx, rows); err != nil {
		return nil, err
	}
	s.log.Info("补录跟单节点", "order", order.DocNo, "count", len(rows), "operator", operator)
	return s.milestones.ListByOrder(ctx, orderID)
}

// attachOverdueNodes 给列表补上"最靠前的逾期未完成节点"。
//
// 一次查完所有单再填，不按单循环：一页 20 张单就是 20 次往返，
// 而这一列只是列表上的一个提示。
func (s *purchaseOrderService) attachOverdueNodes(
	ctx context.Context, list []model.PurchaseOrder,
) error {
	if len(list) == 0 {
		return nil
	}
	ids := make([]uint, 0, len(list))
	for _, o := range list {
		ids = append(ids, o.ID)
	}
	overdue, err := s.milestones.OverdueNodes(ctx, ids, model.DateOnly(s.clock()))
	if err != nil {
		return err
	}
	for i := range list {
		if node, ok := overdue[list[i].ID]; ok {
			list[i].OverdueNode = node.Label()
		}
	}
	return nil
}

// parseMilestoneDate 把可空日期串解析成指针；空串就是"没有这个日期"。
func parseMilestoneDate(raw string) (*time.Time, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	d, err := parseDateOnly(raw)
	if err != nil {
		return nil, err
	}
	return &d, nil
}
