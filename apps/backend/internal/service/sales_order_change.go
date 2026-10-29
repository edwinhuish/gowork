package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/errs"
	"github.com/edwinhuish/gowork/internal/pkg/money"
	"github.com/edwinhuish/gowork/internal/repository"
)

// Change 对一张已确认（或备货中）的订单做一次有记录的变更。
//
// 允许改什么：每一行的数量与单价、订单的交期。**不允许增删行**——
// 出货分配与以销定采的挂接都按明细行 id 引用，增删会让这些引用指向不存在的行。
// 要加减货就是另一笔生意，作废重下才是对的。
//
// 只允许改到不低于已出货量：已经发走的货不可能因为改单而回到仓库。
func (s *salesOrderService) Change(
	ctx context.Context, id uint, in dto.SalesOrderChangeInput,
) (*model.SalesOrderChange, error) {
	if s.changes == nil {
		return nil, errs.ErrInternal.WithMsg("变更单仓储未装配")
	}

	order, err := s.orders.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	switch order.Status {
	case model.DocumentStatusDraft:
		return nil, errs.ErrConflict.WithMsg("草稿订单直接编辑即可，不需要变更单")
	case model.DocumentStatusConfirmed, model.DocumentStatusPreparing:
		// 可以变更。
	default:
		return nil, errs.ErrConflict.WithMsg("已完成或已取消的订单不能再变更")
	}

	actor, ok := ActorFrom(ctx)
	if !ok {
		// 变更必须能回答"谁改的"，取不到身份就拒绝，而不是记一个空操作人。
		return nil, errs.ErrUnauthorized.WithMsg("无法确定操作人，变更必须留痕")
	}

	reason := strings.TrimSpace(in.Reason)
	if reason == "" {
		return nil, errs.BadRequest("变更原因必填")
	}

	shipped, err := s.shipments.ShippedQtyByOrder(ctx, id)
	if err != nil {
		return nil, err
	}

	byID := make(map[uint]*model.SalesOrderItem, len(order.Items))
	for i := range order.Items {
		byID[order.Items[i].ID] = &order.Items[i]
	}

	newValues := make(map[uint][2]money.Decimal, len(in.Lines))
	lines := make([]model.SalesOrderChangeLine, 0, len(in.Lines))
	patches := make([]repository.ItemPatch, 0, len(in.Lines))
	changed := false

	for _, row := range in.Lines {
		item, ok := byID[row.SalesOrderItemID]
		if !ok {
			return nil, errs.BadRequest(
				fmt.Sprintf("明细行 %d 不属于这张订单", row.SalesOrderItemID))
		}
		if _, dup := newValues[item.ID]; dup {
			return nil, errs.BadRequest(fmt.Sprintf("明细行 %s 重复提交", item.SKUCode))
		}

		qty := row.Qty.Round(3)
		if qty.Cmp(money.Zero()) <= 0 {
			return nil, errs.BadRequest("变更后的数量必须大于 0")
		}
		if row.UnitPrice.IsNegative() {
			return nil, errs.BadRequest("变更后的单价不能为负")
		}
		if qty.Cmp(shipped[item.ID]) < 0 {
			return nil, errs.ErrConflict.WithMsg(fmt.Sprintf(
				"%s 已出货 %s，变更后的数量不能低于已出货量", item.SKUCode, shipped[item.ID]))
		}
		if qty.Cmp(item.Qty) == 0 && row.UnitPrice.Cmp(item.UnitPrice) == 0 {
			// 提交了但没变：不进变更记录，也不当作一次改动。
			continue
		}

		changed = true
		newValues[item.ID] = [2]money.Decimal{qty, row.UnitPrice}

		// 金额按行重算，口径与建单时一致：数量先定到 3 位，再乘单价。
		amount := row.UnitPrice.Mul(qty).Round(4)
		amountBase := amount.Mul(order.ExchangeRate).Round(4)
		patches = append(patches, repository.ItemPatch{
			ItemID: item.ID,
			Fields: map[string]any{
				"qty":         qty,
				"unit_price":  row.UnitPrice,
				"amount":      amount,
				"amount_base": amountBase,
			},
		})
		lines = append(lines, model.SalesOrderChangeLine{
			SalesOrderItemID: item.ID,
			SKUCode:          item.SKUCode,
			BeforeQty:        item.Qty,
			AfterQty:         qty,
			BeforeUnitPrice:  item.UnitPrice,
			AfterUnitPrice:   row.UnitPrice,
		})
	}

	// 合计：没动过的行沿用原值，动过的取新值。
	afterTotal := money.Zero()
	for i := range order.Items {
		it := &order.Items[i]
		qty, price := it.Qty, it.UnitPrice
		if v, ok := newValues[it.ID]; ok {
			qty, price = v[0], v[1]
		}
		afterTotal = afterTotal.Add(price.Mul(qty).Round(4))
	}
	afterTotal = afterTotal.Round(4)
	afterBase := afterTotal.Mul(order.ExchangeRate).Round(4)

	// 交期：留空表示这次不动它。
	var afterDelivery *time.Time
	deliveryChanged := false
	if in.DeliveryDate != nil {
		d, err := parseOptionalDateOnly(*in.DeliveryDate)
		if err != nil {
			return nil, errs.BadRequest("交期格式应为 YYYY-MM-DD")
		}
		afterDelivery = d
		deliveryChanged = !sameDay(order.DeliveryDate, d)
		if deliveryChanged {
			changed = true
		}
	}

	if !changed {
		return nil, errs.BadRequest("这次变更没有任何改动")
	}

	seq, err := s.changes.NextSeq(ctx, id)
	if err != nil {
		return nil, err
	}
	change := &model.SalesOrderChange{
		SalesOrderID:       id,
		Seq:                seq,
		DocNo:              fmt.Sprintf("%s-CH%d", order.DocNo, seq),
		Reason:             reason,
		Operator:           actor.Username,
		BeforeTotal:        order.AmountTotal,
		AfterTotal:         afterTotal,
		BeforeDeliveryDate: order.DeliveryDate,
		AfterDeliveryDate:  afterDelivery,
	}

	orderFields := map[string]any{
		"amount_total":      afterTotal,
		"amount_base_total": afterBase,
	}
	if deliveryChanged {
		orderFields["delivery_date"] = afterDelivery
	}
	// 与编辑同一套乐观锁：两个人同时提交变更时，后一个应当被拒绝而不是叠加。
	if in.UpdatedAt != nil {
		orderFields[repository.FieldUpdatedAt] = *in.UpdatedAt
	}

	if err := s.changes.Apply(ctx, change, lines, patches, orderFields); err != nil {
		return nil, err
	}
	change.Lines = lines

	s.log.Info("订单变更",
		"doc_no", order.DocNo, "seq", seq, "operator", actor.Username,
		"reason", reason, "lines", len(lines),
		"before_total", change.BeforeTotal, "after_total", change.AfterTotal)
	return change, nil
}

func (s *salesOrderService) ListChanges(
	ctx context.Context, id uint,
) ([]model.SalesOrderChange, error) {
	if s.changes == nil {
		return nil, errs.ErrInternal.WithMsg("变更单仓储未装配")
	}
	// 订单不存在时先报 404，而不是返回一个空历史——
	// 空历史会让"这张单从没改过"与"这张单不存在"看起来一样。
	if _, err := s.orders.FindByID(ctx, id); err != nil {
		return nil, err
	}
	return s.changes.ListByOrder(ctx, id)
}

// sameDay 比较两个可空日期是否同一天。都为空算相同（都没定交期）。
func sameDay(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Format("2006-01-02") == b.Format("2006-01-02")
}
