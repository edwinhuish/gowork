package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/money"
)

// SumSettlementDue 汇总期间内**到期**的单据金额（本币），用于回款率的分母。
//
// 用单据金额而不是未核销额：分母回答的是"这段时间该收多少"，
// 已经收回的那部分同样是该收的钱；用未核销额当分母，会把
// "按时收齐"算成 0（都收完了，自然一分未结清）。
//
// 作废单据不算：它没有被承诺过。
func (r *reportRepo) SumSettlementDue(
	ctx context.Context,
	target model.SettlementTarget,
	from, to *time.Time,
) (money.Decimal, error) {
	table, err := settlementTable(target)
	if err != nil {
		return money.Zero(), err
	}

	q := r.conn(ctx).
		Table(table).
		Select("COALESCE(SUM(amount_base), 0) AS total").
		Where("deleted_at = 0").
		Where("status <> ?", model.SettlementStatusVoid)
	if from != nil {
		q = q.Where("due_date >= ?", *from)
	}
	if to != nil {
		q = q.Where("due_date <= ?", *to)
	}

	var row struct{ Total money.Decimal }
	if err := q.Scan(&row).Error; err != nil {
		return money.Zero(), fmt.Errorf("汇总期间到期金额(%s): %w", table, err)
	}
	return row.Total, nil
}

// SumReceivedAllocations 汇总期间内到账并核销的金额（本币），用于回款率的分子。
//
// 两个条件都要带：方向（收 / 付）决定这是不是"回款"，目标类型决定这笔钱
// 到底核销在应收还是应付上。只按方向统计，会把「预收」——还没对应任何单据的钱
// ——也算成回款，而它连是不是这笔生意的钱都还不知道。
//
// 日期取**到账日**（payment.pay_date），不取录入日：钱是哪天到的，
// 决定了它属于哪个月的现金流。
func (r *reportRepo) SumReceivedAllocations(
	ctx context.Context,
	direction model.PaymentDirection,
	target model.SettlementTarget,
	from, to *time.Time,
) (money.Decimal, error) {
	q := r.conn(ctx).
		Table("biz_payment_allocation AS a").
		Select("COALESCE(SUM(a.amount_base), 0) AS total").
		Joins("JOIN biz_payment AS p ON p.id = a.payment_id AND p.deleted_at = 0").
		Where("a.deleted_at = 0").
		Where("p.direction = ?", direction).
		Where("a.target_type = ?", target)
	if from != nil {
		q = q.Where("p.pay_date >= ?", *from)
	}
	if to != nil {
		q = q.Where("p.pay_date <= ?", *to)
	}

	var row struct{ Total money.Decimal }
	if err := q.Scan(&row).Error; err != nil {
		return money.Zero(), fmt.Errorf("汇总期间核销金额: %w", err)
	}
	return row.Total, nil
}
