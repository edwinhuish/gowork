package repository

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/money"
)

// TodoFollowup 是一条"该跟进了"的记录：下次跟进日已经过去。
type TodoFollowup struct {
	ID           uint
	PartnerID    uint
	PartnerName  string
	Subject      string
	NextFollowAt time.Time
}

// TodoReceivable 是一张"该催了"的应收：到期日已到（或已过）且未结清。
type TodoReceivable struct {
	ID              uint
	DocNo           string
	PartnerID       uint
	PartnerName     string
	DueDate         time.Time
	OutstandingBase money.Decimal
}

// TodoCurrency 是一个今天没有汇率的外币。
type TodoCurrency struct {
	Code string
	Name string
}

// ListTodoFollowups 取到期待跟进的记录。
//
// "到期"按 `next_follow_at < 今天` 算：当天跟进的还没过完，不该在早上就红一片。
// ownerID 非空时只看自己（与未分配）客户的跟进——提醒不该把人叫去看别人的客户。
func (r *reportRepo) ListTodoFollowups(
	ctx context.Context, asOf time.Time, ownerID *uint, limit int,
) ([]TodoFollowup, int64, error) {
	base := func() *gorm.DB {
		q := r.conn(ctx).
			Table("crm_activity AS a").
			Joins("LEFT JOIN md_partner AS p ON p.id = a.partner_id AND p.deleted_at = 0").
			Where("a.deleted_at = 0").
			Where("a.next_follow_at IS NOT NULL").
			Where("a.next_follow_at < ?", asOf)
		if ownerID != nil {
			q = q.Where("(p.owner_id = ? OR p.owner_id = 0)", *ownerID)
		}
		return q
	}

	var total int64
	if err := base().Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("统计待跟进: %w", err)
	}

	rows := make([]TodoFollowup, 0, limit)
	err := base().
		Select(`a.id, a.partner_id, COALESCE(p.name, '') AS partner_name,
			a.subject, a.next_follow_at`).
		Order("a.next_follow_at ASC, a.id ASC").
		Limit(limit).
		Scan(&rows).Error
	if err != nil {
		return nil, 0, fmt.Errorf("查询待跟进: %w", err)
	}
	return rows, total, nil
}

// ListTodoReceivables 取已到期待收的应收。
//
// 口径与账龄一致：作废不算、按未核销金额算、到期当天算"已到期"
// ——催款是当天的事，等到明天再提醒就晚了一天。
func (r *reportRepo) ListTodoReceivables(
	ctx context.Context, asOf time.Time, ownerID *uint, limit int,
) ([]TodoReceivable, int64, error) {
	base := func() *gorm.DB {
		q := r.conn(ctx).
			Table("biz_receivable AS r").
			Joins("LEFT JOIN md_partner AS p ON p.id = r.partner_id AND p.deleted_at = 0").
			Where("r.deleted_at = 0").
			Where("r.status <> ?", model.SettlementStatusVoid).
			Where("r.amount - r.settled_amount > ?", 0.0001).
			Where("r.due_date <= ?", asOf)
		if ownerID != nil {
			q = q.Where("(p.owner_id = ? OR p.owner_id = 0)", *ownerID)
		}
		return q
	}

	var total int64
	if err := base().Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("统计应收待催: %w", err)
	}

	rows := make([]TodoReceivable, 0, limit)
	err := base().
		Select(`r.id, r.doc_no, r.partner_id, COALESCE(p.name, '') AS partner_name,
			r.due_date, r.amount_base - r.settled_amount_base AS outstanding_base`).
		Order("r.due_date ASC, r.id ASC").
		Limit(limit).
		Scan(&rows).Error
	if err != nil {
		return nil, 0, fmt.Errorf("查询应收待催: %w", err)
	}
	return rows, total, nil
}

// TodoInspection 是一条已过计划日的验货节点。
type TodoInspection struct {
	ID              uint
	PurchaseOrderID uint
	DocNo           string
	PartnerName     string
	SKUCode         string
	PlanDate        time.Time
}

// ListTodoInspections 取已过计划日、还没做完成的验货节点（T-62）。
//
// 口径：plan_date <= asOf 且 done_date 为空，只看 node = 验货。
// 计划日为空的不算——还没跟工厂谈定日期的节点，说"逾期"没有依据。
//
// 数据范围与其它待办一致：采购单经 partner_id 继承归属，业务员只看自己的
// （与未分配的）。
func (r *reportRepo) ListTodoInspections(
	ctx context.Context, asOf time.Time, ownerID *uint, limit int,
) ([]TodoInspection, int64, error) {
	base := func() *gorm.DB {
		q := r.conn(ctx).
			Table("biz_purchase_milestone AS m").
			Joins("JOIN biz_purchase_order AS o ON o.id = m.purchase_order_id AND o.deleted_at = 0").
			Joins("JOIN biz_purchase_order_item AS i ON i.id = m.purchase_order_item_id AND i.deleted_at = 0").
			Joins("LEFT JOIN md_partner AS p ON p.id = o.supplier_id AND p.deleted_at = 0").
			Where("m.deleted_at = 0").
			Where("m.node = ?", model.PurchaseNodeInspected).
			Where("m.done_date IS NULL").
			Where("m.plan_date IS NOT NULL").
			Where("m.plan_date <= ?", asOf).
			// 草稿与已取消的采购单不催验货：货还没让工厂做，验什么。
			Where("o.status NOT IN ?", []model.DocumentStatus{
				model.DocumentStatusDraft, model.DocumentStatusCancelled,
			})
		if ownerID != nil {
			q = q.Where("(p.owner_id = ? OR p.owner_id = 0)", *ownerID)
		}
		return q
	}

	var total int64
	if err := base().Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("统计待验货: %w", err)
	}

	rows := make([]TodoInspection, 0, limit)
	err := base().
		Select(`m.id, m.purchase_order_id, o.doc_no,
			COALESCE(p.name, '') AS partner_name, i.sku_code, m.plan_date`).
		Order("m.plan_date ASC, m.id ASC").
		Limit(limit).
		Scan(&rows).Error
	if err != nil {
		return nil, 0, fmt.Errorf("查询待验货: %w", err)
	}
	return rows, total, nil
}

// ListMissingRates 取今天还没有汇率的外币。
//
// 缺汇率不是"看着不舒服"：报价与开单都要拿当天的汇率锁定本币金额，
// 少了它，录单的人在保存那一刻才发现，而那时客户还在电话上等。
func (r *reportRepo) ListMissingRates(
	ctx context.Context, asOf time.Time,
) ([]TodoCurrency, error) {
	rows := make([]TodoCurrency, 0, 8)
	err := r.conn(ctx).
		Table("md_currency AS c").
		Select("c.code, c.name").
		Where("c.deleted_at = 0").
		Where("c.is_base = ?", false).
		Where("c.status = ?", model.EnableStatusOn).
		Where(`NOT EXISTS (
			SELECT 1 FROM md_exchange_rate AS r
			WHERE r.deleted_at = 0 AND r.currency_code = c.code AND r.effective_date = ?
		)`, asOf).
		Order("c.code ASC").
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("查询缺汇率的币种: %w", err)
	}
	return rows, nil
}

// TodoContractTerm 是一条已到期、还没收到的合同分期。
type TodoContractTerm struct {
	ID          uint
	ContractID  uint
	DocNo       string
	PartnerName string
	Seq         int
	Name        string
	DueDate     time.Time
	AmountBase  money.Decimal
}

// ListTodoContractTerms 取已到期的合同分期，还没收到的那些（T-61）。
//
// 「收到」看这一期挂的应收单：没开票（receivable_id 为空）算未收，
// 开了票但还有未结清余额（amount − settled − credited）也算未收。
// 不存一个 paid 布尔值：录错了无从纠正，也解释不了"显示已收但账上还有余额"。
func (r *reportRepo) ListTodoContractTerms(
	ctx context.Context, asOf time.Time, ownerID *uint, limit int,
) ([]TodoContractTerm, int64, error) {
	base := func() *gorm.DB {
		q := r.conn(ctx).
			Table("biz_contract_term AS t").
			Joins("JOIN biz_contract AS c ON c.id = t.contract_id AND c.deleted_at = 0").
			Joins("LEFT JOIN md_partner AS p ON p.id = c.partner_id AND p.deleted_at = 0").
			Joins("LEFT JOIN biz_receivable AS r ON r.id = t.receivable_id AND r.deleted_at = 0").
			Where("t.deleted_at = 0").
			Where("t.due_date IS NOT NULL").
			Where("t.due_date <= ?", asOf).
			// 没开票，或开了票还有余额，都算没收到。
			Where("(t.receivable_id IS NULL OR r.id IS NULL OR r.amount - r.settled_amount - "+
				creditSubquery("r")+" > ?)", 0.0001).
			Where("c.status NOT IN ?", []model.DocumentStatus{
				model.DocumentStatusDraft, model.DocumentStatusCancelled,
			})
		if ownerID != nil {
			q = q.Where("(p.owner_id = ? OR p.owner_id = 0)", *ownerID)
		}
		return q
	}

	var total int64
	if err := base().Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("统计待收合同分期: %w", err)
	}

	rows := make([]TodoContractTerm, 0, limit)
	err := base().
		Select(`t.id, t.contract_id, c.doc_no, COALESCE(p.name, '') AS partner_name,
			t.seq, t.name, t.due_date, t.amount_base`).
		Order("t.due_date ASC, t.id ASC").
		Limit(limit).
		Scan(&rows).Error
	if err != nil {
		return nil, 0, fmt.Errorf("查询待收合同分期: %w", err)
	}
	return rows, total, nil
}
