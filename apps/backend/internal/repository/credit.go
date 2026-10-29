package repository

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/money"
)

// CreditExposure 是客户已经占用的信用：还没收回来的钱 + 已经承诺但还没开票的货。
//
// 两块拼起来才是"敞口"：只算应收会漏掉已确认还没开票的订单，只算订单会漏掉
// 开票后还没收到的款——两头都算，才等于"这个客户身上压着多少钱"。
type CreditExposure struct {
	// OpenReceivableBase 是未结清应收的本币余额（amount_base − settled_amount_base）。
	OpenReceivableBase money.Decimal
	// UnbilledOrderBase 是已确认及以上状态、还没生成应收的订单本币金额。
	UnbilledOrderBase money.Decimal
}

// Total 是敞口合计，单位本币。
func (e CreditExposure) Total() money.Decimal {
	return e.OpenReceivableBase.Add(e.UnbilledOrderBase)
}

// CreditRepository 只负责回答"这个主体占了多少信用"。
//
// 单独成一个仓储而不是挂到应收/订单仓储上：敞口横跨两张表（应收 + 订单），
// 挂在任何一边都会让那一边的仓储去读另一边的表。
type CreditRepository interface {
	Exposure(ctx context.Context, partnerID uint) (CreditExposure, error)
}

type creditRepo struct {
	*Base[model.Receivable]
}

func NewCreditRepository(db *gorm.DB) CreditRepository {
	return &creditRepo{Base: NewBase[model.Receivable](db)}
}

func (r *creditRepo) Exposure(ctx context.Context, partnerID uint) (CreditExposure, error) {
	var out CreditExposure

	// 未结清应收：排除作废单。差值与 0.0001 比较是**存储层**的余量：
	// sqlite 把 DECIMAL 存成 REAL，同一笔余额减自己可能留下 1e-17 的尾巴，
	// 那种行不该以"还有 0.00 未结"的身份出现在账龄里。
	var open struct{ Amount money.Decimal }
	err := r.conn(ctx).
		Table("biz_receivable").
		Where("deleted_at = 0").
		Where("partner_id = ?", partnerID).
		Where("status <> ?", model.SettlementStatusVoid).
		// 已开的贷项要扣掉（T-52b）：客户退货冲减掉的那部分不该再算进他的敞口，
		// 否则一张退过货的单会让这个客户看起来像欠着钱，下一次下单被信用额度拦住。
		Where("amount_base - settled_amount_base - "+creditBaseSubquery+" > ?", 0.0001).
		Select("COALESCE(SUM(amount_base - settled_amount_base - " + creditBaseSubquery + "), 0) AS amount").
		Scan(&open).Error
	if err != nil {
		return out, fmt.Errorf("汇总未结清应收: %w", err)
	}
	out.OpenReceivableBase = open.Amount

	// 已承诺未开票：状态在 20~50 之间且没有 source_type=10 的应收单。
	// 「没有应收单」用 NOT EXISTS 判断，而不是拿订单表去 left join 应收：
	// 应收是一期一张单，主键之外的任何关联写法都会在将来分期的表结构上演变成重复计数。
	var unbilled struct{ Amount money.Decimal }
	err = r.conn(ctx).
		Table("biz_sales_order AS o").
		Where("o.deleted_at = 0").
		Where("o.partner_id = ?", partnerID).
		Where("o.status IN ?", []model.DocumentStatus{
			model.DocumentStatusConfirmed, model.DocumentStatusPreparing,
			model.DocumentStatusShipped, model.DocumentStatusCompleted,
		}).
		Where(`NOT EXISTS (
			SELECT 1 FROM biz_receivable AS r
			WHERE r.deleted_at = 0
			  AND r.source_type = ?
			  AND r.source_id = o.id
		)`, model.SettlementSourceSalesOrder).
		Select("COALESCE(SUM(o.amount_base_total), 0) AS amount").
		Scan(&unbilled).Error
	if err != nil {
		return out, fmt.Errorf("汇总未开票订单: %w", err)
	}
	out.UnbilledOrderBase = unbilled.Amount

	return out, nil
}

// creditBaseSubquery 是「这张应收已开的贷项合计（本币）」的相关子查询。
//
// 写成常量是为了让用到它的地方拼的是同一段 SQL：敞口、账龄、回款率
// 三处各抄一遍，改一处忘两处就会变成"三个地方算出三个余额"。
const creditBaseSubquery = "COALESCE((SELECT SUM(c.amount_base) FROM biz_credit_note c " +
	"WHERE c.receivable_id = biz_receivable.id AND c.deleted_at = 0), 0)"
