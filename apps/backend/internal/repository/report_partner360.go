package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/errs"
	"github.com/edwinhuish/gowork/internal/pkg/money"
)

// PartnerBrief 是 360 视图抬头用的主体基本信息。
//
// 只取几个字：详情页本身已经把这些字段完整展示了一遍，
// 这里再回吐一整行主体，只会让响应里出现两份可能不同步的同一份数据。
type PartnerBrief struct {
	ID      uint
	Code    string
	Name    string
	Country string
	Role    model.PartnerRole
}

// Partner360Totals 是一个主体的关键数字：订单、出货、应收、跟进、商机。
//
// 五个口径各查一次而不是拼成一个大 UNION：它们分别来自五张没有共同键的表，
// 拼起来要写五次 LEFT JOIN 再 GROUP BY，出问题时连"哪个数是错的"都看不出来。
type Partner360Totals struct {
	OrderCount      int64
	OrderAmountBase money.Decimal
	LastOrderDate   *time.Time

	// ShipmentCount 是**含该客户订单行的**出货票数（去重）：拼柜时一票里有别的客户的货，
	// 按票头统计既没有客户字段，也会把这票整票算给这家。
	ShipmentCount int64

	ActivityCount  int64
	LastActivityAt *time.Time

	// 商机只算未终结（赢单 / 输单之外）的：已赢的已经变成订单，已输的不该再占预测。
	OpenOpportunityCount      int64
	OpenOpportunityAmountBase money.Decimal
}

// Partner360Activity 是一条跟进记录。
type Partner360Activity struct {
	ID           uint
	Type         string
	Subject      string
	OccurredAt   time.Time
	NextFollowAt *time.Time
}

// Partner360Opportunity 是一条还在谈的商机。
type Partner360Opportunity struct {
	ID                uint
	Subject           string
	Stage             model.OpportunityStage
	AmountBase        money.Decimal
	Probability       int
	ExpectedCloseDate time.Time
}

// Partner360Order 是一张销售订单。
type Partner360Order struct {
	ID              uint
	DocNo           string
	OrderDate       time.Time
	Status          model.DocumentStatus
	CurrencyCode    string
	AmountBaseTotal money.Decimal
}

// Partner360Shipment 是一票出货。
type Partner360Shipment struct {
	ID       uint
	DocNo    string
	ShipDate time.Time
	Status   model.DocumentStatus
}

// PartnerOutstandingReceivable 是一张还没结清的应收。
type PartnerOutstandingReceivable struct {
	ID                uint
	DocNo             string
	CurrencyCode      string
	DueDate           time.Time
	Amount            money.Decimal
	SettledAmount     money.Decimal
	AmountBase        money.Decimal
	SettledAmountBase money.Decimal
}

func (r *reportRepo) GetPartnerBrief(ctx context.Context, id uint) (*PartnerBrief, error) {
	var rows []PartnerBrief
	err := r.conn(ctx).
		Table("md_partner").
		Select("id, code, name, COALESCE(country, '') AS country, role").
		Where("id = ? AND deleted_at = 0", id).
		Limit(1).
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("查询主体 %d: %w", id, err)
	}
	if len(rows) == 0 {
		return nil, errs.ErrNotFound
	}
	return &rows[0], nil
}

func (r *reportRepo) Partner360Totals(
	ctx context.Context, partnerID uint,
) (*Partner360Totals, error) {
	out := &Partner360Totals{}

	var orders []struct {
		Count      int64
		AmountBase money.Decimal
	}
	if err := r.conn(ctx).
		Table("biz_sales_order").
		Select("COUNT(*) AS count, COALESCE(SUM(amount_base_total), 0) AS amount_base").
		Where("partner_id = ? AND deleted_at = 0", partnerID).
		Where("status NOT IN ?", []model.DocumentStatus{
			model.DocumentStatusDraft, model.DocumentStatusCancelled,
		}).
		Scan(&orders).Error; err != nil {
		return nil, fmt.Errorf("统计主体 %d 的订单: %w", partnerID, err)
	}
	if len(orders) > 0 {
		out.OrderCount = orders[0].Count
		out.OrderAmountBase = orders[0].AmountBase
	}
	// 最近日期单独取：MAX(date) 是个表达式，sqlite 会把结果当字符串返回，
	// 扫进 time.Time 直接报错；取一行原列，三个方言都能按声明类型还原。
	var lastOrder []struct{ OrderDate time.Time }
	if err := r.conn(ctx).
		Table("biz_sales_order").
		Select("order_date").
		Where("partner_id = ? AND deleted_at = 0", partnerID).
		Where("status NOT IN ?", []model.DocumentStatus{
			model.DocumentStatusDraft, model.DocumentStatusCancelled,
		}).
		Order("order_date DESC, id DESC").
		Limit(1).
		Scan(&lastOrder).Error; err != nil {
		return nil, fmt.Errorf("取主体 %d 的最近下单日: %w", partnerID, err)
	}
	if len(lastOrder) > 0 {
		d := lastOrder[0].OrderDate
		out.LastOrderDate = &d
	}

	var shipments []struct{ Count int64 }
	if err := r.conn(ctx).
		Table("biz_shipment AS s").
		Select("COUNT(DISTINCT s.id) AS count").
		Joins("JOIN biz_shipment_allocation AS a ON a.shipment_id = s.id AND a.deleted_at = 0").
		Joins("JOIN biz_sales_order_item AS i ON i.id = a.sales_order_item_id AND i.deleted_at = 0").
		Joins("JOIN biz_sales_order AS o ON o.id = i.sales_order_id AND o.deleted_at = 0").
		Where("o.partner_id = ? AND s.deleted_at = 0", partnerID).
		Where("s.status NOT IN ?", []model.DocumentStatus{
			model.DocumentStatusDraft, model.DocumentStatusCancelled,
		}).
		Scan(&shipments).Error; err != nil {
		return nil, fmt.Errorf("统计主体 %d 的出货: %w", partnerID, err)
	}
	if len(shipments) > 0 {
		out.ShipmentCount = shipments[0].Count
	}

	var activities []struct{ Count int64 }
	if err := r.conn(ctx).
		Table("crm_activity").
		Select("COUNT(*) AS count").
		Where("partner_id = ? AND deleted_at = 0", partnerID).
		Scan(&activities).Error; err != nil {
		return nil, fmt.Errorf("统计主体 %d 的跟进: %w", partnerID, err)
	}
	if len(activities) > 0 {
		out.ActivityCount = activities[0].Count
	}
	var lastActivity []struct{ OccurredAt time.Time }
	if err := r.conn(ctx).
		Table("crm_activity").
		Select("occurred_at").
		Where("partner_id = ? AND deleted_at = 0", partnerID).
		Order("occurred_at DESC, id DESC").
		Limit(1).
		Scan(&lastActivity).Error; err != nil {
		return nil, fmt.Errorf("取主体 %d 的最近跟进: %w", partnerID, err)
	}
	if len(lastActivity) > 0 {
		d := lastActivity[0].OccurredAt
		out.LastActivityAt = &d
	}

	var opportunities []struct {
		Count      int64
		AmountBase money.Decimal
	}
	if err := r.conn(ctx).
		Table("crm_opportunity").
		Select("COUNT(*) AS count, COALESCE(SUM(amount_base), 0) AS amount_base").
		Where("partner_id = ? AND deleted_at = 0", partnerID).
		Where("stage NOT IN ?", []model.OpportunityStage{
			model.OpportunityStageWon, model.OpportunityStageLost,
		}).
		Scan(&opportunities).Error; err != nil {
		return nil, fmt.Errorf("统计主体 %d 的商机: %w", partnerID, err)
	}
	if len(opportunities) > 0 {
		out.OpenOpportunityCount = opportunities[0].Count
		out.OpenOpportunityAmountBase = opportunities[0].AmountBase
	}

	return out, nil
}

func (r *reportRepo) ListPartnerActivities(
	ctx context.Context, partnerID uint, limit int,
) ([]Partner360Activity, error) {
	rows := make([]Partner360Activity, 0, limit)
	err := r.conn(ctx).
		Table("crm_activity").
		// type 在部分方言里是关键字，取别名避开各库的反引号差异。
		Select("id, type AS activity_type, subject, occurred_at, next_follow_at").
		Where("partner_id = ? AND deleted_at = 0", partnerID).
		Order("occurred_at DESC, id DESC").
		Limit(limit).
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("查询主体 %d 的跟进: %w", partnerID, err)
	}
	return rows, nil
}

func (r *reportRepo) ListPartnerOpportunities(
	ctx context.Context, partnerID uint, limit int,
) ([]Partner360Opportunity, error) {
	rows := make([]Partner360Opportunity, 0, limit)
	err := r.conn(ctx).
		Table("crm_opportunity").
		Select("id, subject, stage, amount_base, probability, expected_close_date").
		Where("partner_id = ? AND deleted_at = 0", partnerID).
		Where("stage NOT IN ?", []model.OpportunityStage{
			model.OpportunityStageWon, model.OpportunityStageLost,
		}).
		// 最近要成交的排前面：这张表是拿来决定"接下来该跟哪一条"的。
		Order("expected_close_date ASC, id DESC").
		Limit(limit).
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("查询主体 %d 的商机: %w", partnerID, err)
	}
	return rows, nil
}

func (r *reportRepo) ListPartnerOrders(
	ctx context.Context, partnerID uint, limit int,
) ([]Partner360Order, error) {
	rows := make([]Partner360Order, 0, limit)
	err := r.conn(ctx).
		Table("biz_sales_order").
		Select("id, doc_no, order_date, status, currency_code, amount_base_total").
		Where("partner_id = ? AND deleted_at = 0", partnerID).
		Where("status NOT IN ?", []model.DocumentStatus{
			model.DocumentStatusDraft, model.DocumentStatusCancelled,
		}).
		Order("order_date DESC, id DESC").
		Limit(limit).
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("查询主体 %d 的订单: %w", partnerID, err)
	}
	return rows, nil
}

func (r *reportRepo) ListPartnerShipments(
	ctx context.Context, partnerID uint, limit int,
) ([]Partner360Shipment, error) {
	rows := make([]Partner360Shipment, 0, limit)
	err := r.conn(ctx).
		Table("biz_shipment AS s").
		Select("DISTINCT s.id, s.doc_no, s.ship_date, s.status").
		Joins("JOIN biz_shipment_allocation AS a ON a.shipment_id = s.id AND a.deleted_at = 0").
		Joins("JOIN biz_sales_order_item AS i ON i.id = a.sales_order_item_id AND i.deleted_at = 0").
		Joins("JOIN biz_sales_order AS o ON o.id = i.sales_order_id AND o.deleted_at = 0").
		Where("o.partner_id = ? AND s.deleted_at = 0", partnerID).
		Where("s.status NOT IN ?", []model.DocumentStatus{
			model.DocumentStatusDraft, model.DocumentStatusCancelled,
		}).
		Order("s.ship_date DESC, s.id DESC").
		Limit(limit).
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("查询主体 %d 的出货: %w", partnerID, err)
	}
	return rows, nil
}

// ListPartnerOutstandingReceivables 取该主体全部未结清应收（**不截断**）。
//
// 不截断是给合计用的：超期金额要按全部单据算，拿了前 10 张去加，
// 得出的"逾期多少"只会比实际小，而这类数一旦偏小就没人会去查。
func (r *reportRepo) ListPartnerOutstandingReceivables(
	ctx context.Context, partnerID uint,
) ([]PartnerOutstandingReceivable, error) {
	rows := make([]PartnerOutstandingReceivable, 0, 16)
	err := r.conn(ctx).
		Table("biz_receivable").
		Select(`id, doc_no, currency_code, due_date,
			amount, settled_amount, amount_base, settled_amount_base`).
		Where("partner_id = ? AND deleted_at = 0", partnerID).
		Where("status <> ?", model.SettlementStatusVoid).
		Where("amount - settled_amount > ?", 0.0001).
		Order("due_date ASC, id ASC").
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("查询主体 %d 的未结清应收: %w", partnerID, err)
	}
	return rows, nil
}
