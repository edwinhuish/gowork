package repository

import (
	"context"
	"fmt"
	"sort"
	"time"

	"gorm.io/gorm"

	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/errs"
	"github.com/edwinhuish/gowork/internal/pkg/money"
)

// OutstandingSettlement 是一张还没结清的单据，账龄报表的原料。
//
// 只取报表需要的列：报表要的是"谁欠多少、什么时候到期"，
// 把整张应收单（含来源单号、备注、状态机）读进来只会让聚合多背一份内存。
type OutstandingSettlement struct {
	ID                uint
	DocNo             string
	PartnerID         uint
	PartnerCode       string
	PartnerName       string
	CurrencyCode      string
	Amount            money.Decimal
	SettledAmount     money.Decimal
	AmountBase        money.Decimal
	SettledAmountBase money.Decimal
	// Credited 是这张单已开的贷项合计（T-52b2）。未结清额 =
	// Amount − SettledAmount − Credited，三处报表与核销校验必须同口径。
	Credited     money.Decimal
	CreditedBase money.Decimal
	// DueDate 只用到日期部分；分桶按整天算，带时分秒会让边界日期飘到隔壁桶。
	DueDate time.Time
}

// ReportRepository 是只读聚合的入口：账龄（T-22）与单票毛利（T-22）。
//
// 单独成一个仓储而不是往应收应付里加方法：报表横跨多张表（单据 + 主体 +
// 采购行 + 费用），而应收应付仓储的职责是"守各自单据的写入规则"。
// 混在一起，写路径的约束会被报表的读需求牵着走。
type ReportRepository interface {
	// ListOutstandingSettlements 列出未结清且未作废的应收或应付。
	//
	// 过滤条件里的 amount > settled_amount 用 0.0001 的余量，这是**存储层**的：
	// sqlite 把 DECIMAL 存成 REAL，减出来的可能是 1e-17 的尾巴，
	// 那种行已经结清，不该以"还有 0.00 未结"的身份出现在账龄里。
	ListOutstandingSettlements(
		ctx context.Context, target model.SettlementTarget,
	) ([]OutstandingSettlement, error)

	// ---- 单票毛利（T-22）----

	// ListShipments 列出参与统计的出货批次，按出货日倒序。
	//
	// 草稿与已取消的票不进统计：草稿的数量还会改，取消的货根本没出，
	// 把它们算进来会让"这票赚没赚"跟着草稿的编辑记录跳动。
	ListShipments(ctx context.Context, f ShipmentProfitFilter, p Pagination) ([]ShipmentHead, int64, error)
	// ListShipmentLines 取这些票的分配行，并带上所属销售行的数量与本币金额。
	//
	// 带上销售行的整行金额而不是只带单价：收入要按"分配数量 ÷ 行数量"分摊，
	// 这样折扣、舍入与行总额对得上，不会出现各行分摊之和与订单总额差几分钱。
	ListShipmentLines(ctx context.Context, shipmentIDs []uint) ([]ShipmentLine, error)
	// PurchaseCostBySalesItem 按销售订单行汇总采购数量与本币金额。
	//
	// 草稿与已取消的采购单不算成本：草稿还只是询价记录，取消的那张根本没买。
	PurchaseCostBySalesItem(ctx context.Context, salesItemIDs []uint) (map[uint]PurchaseCost, error)
	// SumChargesByShipment 按票汇总运杂费本币金额（单票毛利的减项）。
	SumChargesByShipment(ctx context.Context, shipmentIDs []uint) (map[uint]money.Decimal, error)

	// ---- 待办与提醒（T-34）----

	// ListTodoFollowups 取到期待跟进的记录（next_follow_at < asOf）。
	ListTodoFollowups(
		ctx context.Context, asOf time.Time, ownerID *uint, limit int,
	) ([]TodoFollowup, int64, error)
	// ListTodoReceivables 取已到期待收的应收（due_date <= asOf 且未结清）。
	ListTodoReceivables(
		ctx context.Context, asOf time.Time, ownerID *uint, limit int,
	) ([]TodoReceivable, int64, error)
	// ListMissingRates 取今天还没有汇率的外币。
	ListMissingRates(ctx context.Context, asOf time.Time) ([]TodoCurrency, error)

	// ---- 客户 360（T-30）----

	// GetPartnerBrief 取主体抬头（编码 / 名称 / 国别 / 角色位），不存在返回 ErrNotFound。
	GetPartnerBrief(ctx context.Context, id uint) (*PartnerBrief, error)
	// Partner360Totals 汇总订单、出货、应收、跟进、商机五个口径的关键数字。
	Partner360Totals(ctx context.Context, partnerID uint) (*Partner360Totals, error)

	ListPartnerActivities(ctx context.Context, partnerID uint, limit int) ([]Partner360Activity, error)
	ListPartnerOpportunities(
		ctx context.Context, partnerID uint, limit int,
	) ([]Partner360Opportunity, error)
	ListPartnerOrders(ctx context.Context, partnerID uint, limit int) ([]Partner360Order, error)
	ListPartnerShipments(ctx context.Context, partnerID uint, limit int) ([]Partner360Shipment, error)
	// ListPartnerOutstandingReceivables 取全部未结清应收（不截断），合计要用。
	ListPartnerOutstandingReceivables(
		ctx context.Context, partnerID uint,
	) ([]PartnerOutstandingReceivable, error)

	// ---- 回款与超期（T-29）----

	// SumSettlementDue 汇总期间内到期的应收 / 应付金额（本币），回款率的分母。
	SumSettlementDue(
		ctx context.Context, target model.SettlementTarget, from, to *time.Time,
	) (money.Decimal, error)
	// SumReceivedAllocations 汇总期间内到账并核销的金额（本币），回款率的分子。
	SumReceivedAllocations(
		ctx context.Context,
		direction model.PaymentDirection, target model.SettlementTarget,
		from, to *time.Time,
	) (money.Decimal, error)

	// ---- 销售看板（T-28）----

	// ListSalesShipmentLines 取范围内参与统计的出货分配行，带上客户与产品两套维度字段。
	//
	// 一次取全、在 service 里换分组键：若每个维度各写一份 SQL，四个维度就有四份口径，
	// 迟早出现"按客户看 21%、按产品看 19%"这种谁也解释不清的差。
	ListSalesShipmentLines(ctx context.Context, f SalesFilter) ([]SalesShipmentLine, error)
	// ListSalesOrderLines 取范围内的销售订单明细行，用于「订单额」这一列。
	// 草稿与已取消的订单不算：草稿还会改，取消的那张不会成交。
	ListSalesOrderLines(ctx context.Context, f SalesFilter) ([]SalesOrderLine, error)

	// ListExchangeGains 列出每一笔核销产生的汇兑差额原料（T-23）。
	//
	// 一行 = 一次核销：收付款单上锁的到账汇率 vs 目标单据上锁的开单汇率。
	// 损益本身不在 SQL 里算：符号要看核销对象是应收还是应付，
	// 而这个方向语义属于业务口径，放在 service 才能被一条测试钉住。
	ListExchangeGains(ctx context.Context, f ExchangeGainFilter) ([]ExchangeGainLine, error)
}

// ExchangeGainFilter 是汇兑损益的筛选条件。
//
// 只按到账日筛：损益发生在钱动的那天，不是单据开的那天。
type ExchangeGainFilter struct {
	PartnerID    *uint
	CurrencyCode string
	From, To     *time.Time
}

// ExchangeGainLine 是一笔核销的原料：核销多少原币、按什么汇率折的本币、
// 目标单据当时锁的是什么汇率。
type ExchangeGainLine struct {
	PaymentID    uint
	PaymentDocNo string
	PayDate      time.Time
	Direction    model.PaymentDirection
	PartnerID    uint
	PartnerCode  string
	PartnerName  string
	CurrencyCode string
	// PaymentRate 是到账日汇率；DocumentRate 是目标单据锁定汇率。
	PaymentRate  money.Decimal
	DocumentRate money.Decimal
	TargetType   model.SettlementTarget
	TargetDocNo  string
	// Amount 是核销原币；AmountBase 是按到账汇率折的本币。
	Amount     money.Decimal
	AmountBase money.Decimal
}

// ShipmentProfitFilter 是单票毛利的筛选条件。
type ShipmentProfitFilter struct {
	// ShipmentID 只看一票：出货详情页的毛利面板只需要这一票。
	ShipmentID *uint
}

// ShipmentHead 是参与统计的一票货（只取报表要用的列）。
type ShipmentHead struct {
	ID       uint
	DocNo    string
	ShipDate time.Time
	Status   model.DocumentStatus
}

// ShipmentLine 是一行出货分配，带上它所摊的那条销售行。
type ShipmentLine struct {
	ShipmentID       uint
	SalesOrderItemID uint
	// Qty 是本票出的数量。
	Qty money.Decimal
	// ItemQty / ItemAmountBase 是所属销售行的整行数量与本币金额，用于按比例分摊收入。
	ItemQty        money.Decimal
	ItemAmountBase money.Decimal
}

// PurchaseCost 是某条销售订单行累计的采购数量与本币金额。
type PurchaseCost struct {
	Qty        money.Decimal
	AmountBase money.Decimal
}

type reportRepo struct {
	*Base[model.Receivable]
}

func NewReportRepository(db *gorm.DB) ReportRepository {
	return &reportRepo{Base: NewBase[model.Receivable](db)}
}

func (r *reportRepo) ListOutstandingSettlements(
	ctx context.Context, target model.SettlementTarget,
) ([]OutstandingSettlement, error) {
	table, err := settlementTable(target)
	if err != nil {
		return nil, err
	}

	// 求和类字段一律扫进 money.Decimal：SQL 里算出来的是十进制字面量，
	// 让它落到 float64 上，报表的合计立刻与明细对不上。
	// 应收扣贷项、应付扣借项（T-52b3）：两张反向表都把"这笔钱不用收了/不用付了"
	// 记在自己的表里，所以这里要按 target 选子查询，不能写死 credit。
	offsetExpr := creditSubquery("s")
	offsetBaseExpr := creditBaseSubquery("s")
	if target == model.SettlementTargetPayable {
		offsetExpr = debitSubquery("s")
		offsetBaseExpr = debitBaseSubquery("s")
	}

	rows := make([]OutstandingSettlement, 0, 64)
	// 用 Table 而不是 Model：两张表结构相同，写成 Model(&model.Receivable{}) +
	// 切表名反而绕。代价是软删除条件要自己写 —— 漏了就会把已删的单据算进账龄。
	err = r.conn(ctx).
		Table(table+" AS s").
		Select(`s.id,
			s.doc_no,
			s.partner_id,
			p.code AS partner_code,
			p.name AS partner_name,
			s.currency_code,
			s.amount,
			s.settled_amount,
			s.amount_base,
			s.settled_amount_base,
			`+offsetExpr+` AS credited,
			`+offsetBaseExpr+` AS credited_base,
			s.due_date`).
		Joins("LEFT JOIN md_partner AS p ON p.id = s.partner_id AND p.deleted_at = 0").
		Where("s.deleted_at = 0").
		Where("s.status <> ?", model.SettlementStatusVoid).
		// 扣掉贷项：退过货的单只剩很小一笔未结时会掉出账龄，
		// 这正是它该有的样子——那部分钱客户不用付了。
		Where("s.amount - s.settled_amount - "+offsetExpr+" > ?", 0.0001).
		Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("查询未结清单据(%s): %w", table, err)
	}
	return rows, nil
}

func (r *reportRepo) ListShipments(
	ctx context.Context, f ShipmentProfitFilter, p Pagination,
) ([]ShipmentHead, int64, error) {
	p = p.normalize()

	// 两次查询的条件必须一致（先 count 再取页），所以收成一个闭包：
	// 复制一份条件出来改，迟早会出现"count 的与查出来的不是同一批"。
	base := func() *gorm.DB {
		q := r.conn(ctx).
			Table("biz_shipment AS s").
			Where("s.deleted_at = 0").
			Where("s.status NOT IN ?", []model.DocumentStatus{
				model.DocumentStatusDraft, model.DocumentStatusCancelled,
			})
		if f.ShipmentID != nil {
			q = q.Where("s.id = ?", *f.ShipmentID)
		}
		return q
	}

	var total int64
	if err := base().Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("统计出货票数: %w", err)
	}

	rows := make([]ShipmentHead, 0, p.PageSize)
	if err := base().
		Select("s.id, s.doc_no, s.ship_date, s.status").
		Order("s.ship_date DESC, s.id DESC").
		Offset(p.Offset()).
		Limit(p.PageSize).
		Find(&rows).Error; err != nil {
		return nil, 0, fmt.Errorf("查询出货批次: %w", err)
	}
	return rows, total, nil
}

func (r *reportRepo) ListShipmentLines(
	ctx context.Context, shipmentIDs []uint,
) ([]ShipmentLine, error) {
	if len(shipmentIDs) == 0 {
		return nil, nil
	}

	lines := make([]ShipmentLine, 0, len(shipmentIDs)*4)
	err := r.conn(ctx).
		Table("biz_shipment_allocation AS a").
		Select(`a.shipment_id,
			a.sales_order_item_id,
			a.qty,
			i.qty AS item_qty,
			i.amount_base AS item_amount_base`).
		Joins("JOIN biz_sales_order_item AS i ON i.id = a.sales_order_item_id AND i.deleted_at = 0").
		Where("a.deleted_at = 0").
		Where("a.shipment_id IN ?", shipmentIDs).
		Find(&lines).Error
	if err != nil {
		return nil, fmt.Errorf("查询出货分配行: %w", err)
	}
	return lines, nil
}

func (r *reportRepo) PurchaseCostBySalesItem(
	ctx context.Context, salesItemIDs []uint,
) (map[uint]PurchaseCost, error) {
	out := make(map[uint]PurchaseCost, len(salesItemIDs))
	if len(salesItemIDs) == 0 {
		return out, nil
	}

	var rows []struct {
		SalesOrderItemID uint
		Qty              money.Decimal
		AmountBase       money.Decimal
	}
	err := r.conn(ctx).
		Table("biz_purchase_order_item AS i").
		Select("i.sales_order_item_id, SUM(i.qty) AS qty, SUM(i.amount_base) AS amount_base").
		Joins("JOIN biz_purchase_order AS o ON o.id = i.purchase_order_id AND o.deleted_at = 0").
		Where("i.deleted_at = 0").
		Where("i.sales_order_item_id IN ?", salesItemIDs).
		Where("o.status NOT IN ?", []model.DocumentStatus{
			model.DocumentStatusDraft, model.DocumentStatusCancelled,
		}).
		Group("i.sales_order_item_id").
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("汇总采购成本: %w", err)
	}
	for _, row := range rows {
		out[row.SalesOrderItemID] = PurchaseCost{Qty: row.Qty, AmountBase: row.AmountBase}
	}
	return out, nil
}

func (r *reportRepo) SumChargesByShipment(
	ctx context.Context, shipmentIDs []uint,
) (map[uint]money.Decimal, error) {
	out := make(map[uint]money.Decimal, len(shipmentIDs))
	if len(shipmentIDs) == 0 {
		return out, nil
	}

	var rows []struct {
		ShipmentID uint
		AmountBase money.Decimal
	}
	err := r.conn(ctx).
		Table("biz_shipment_charge AS c").
		Select("c.shipment_id, SUM(c.amount_base) AS amount_base").
		Where("c.deleted_at = 0").
		Where("c.shipment_id IN ?", shipmentIDs).
		Group("c.shipment_id").
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("汇总运杂费: %w", err)
	}
	for _, row := range rows {
		out[row.ShipmentID] = row.AmountBase
	}
	return out, nil
}

func (r *reportRepo) ListExchangeGains(
	ctx context.Context, f ExchangeGainFilter,
) ([]ExchangeGainLine, error) {
	// 应收与应付分两次查：两边的目标表不同，用 UNION 也能写，
	// 但两段 SQL 合起来会多出十几个参数、报错时也看不出是哪一半出的问题。
	targets := []struct {
		table string
		kind  model.SettlementTarget
	}{
		{"biz_receivable", model.SettlementTargetReceivable},
		{"biz_payable", model.SettlementTargetPayable},
	}

	lines := make([]ExchangeGainLine, 0, 64)
	for _, t := range targets {
		q := r.conn(ctx).
			Table("biz_payment_allocation AS a").
			Select(`p.id AS payment_id,
				p.doc_no AS payment_doc_no,
				p.pay_date,
				p.direction,
				p.partner_id,
				COALESCE(pt.code, '') AS partner_code,
				COALESCE(pt.name, '') AS partner_name,
				p.currency_code,
				p.exchange_rate AS payment_rate,
				d.exchange_rate AS document_rate,
				a.target_type,
				a.target_doc_no,
				a.amount,
				a.amount_base`).
			Joins("JOIN biz_payment AS p ON p.id = a.payment_id AND p.deleted_at = 0").
			Joins("JOIN "+t.table+" AS d ON d.id = a.target_id AND d.deleted_at = 0").
			Joins("LEFT JOIN md_partner AS pt ON pt.id = p.partner_id AND pt.deleted_at = 0").
			Where("a.deleted_at = 0").
			Where("a.target_type = ?", t.kind)

		if f.PartnerID != nil {
			q = q.Where("p.partner_id = ?", *f.PartnerID)
		}
		if f.CurrencyCode != "" {
			q = q.Where("p.currency_code = ?", f.CurrencyCode)
		}
		// 日期用闭区间：报表问的是"这段时间里到账的钱"，两头都该算进来。
		if f.From != nil {
			q = q.Where("p.pay_date >= ?", *f.From)
		}
		if f.To != nil {
			q = q.Where("p.pay_date <= ?", *f.To)
		}

		var batch []ExchangeGainLine
		if err := q.Order("p.pay_date DESC, p.id DESC, a.id DESC").Find(&batch).Error; err != nil {
			return nil, fmt.Errorf("查询汇兑损益(%s): %w", t.table, err)
		}
		lines = append(lines, batch...)
	}

	// 两批合起来再排一次：否则会出现"应收一段、应付一段"，
	// 而读这张表的人关心的是"这个月哪几笔钱产生了差额"。
	sort.SliceStable(lines, func(i, j int) bool {
		if !lines[i].PayDate.Equal(lines[j].PayDate) {
			return lines[i].PayDate.After(lines[j].PayDate)
		}
		if lines[i].PaymentID != lines[j].PaymentID {
			return lines[i].PaymentID > lines[j].PaymentID
		}
		return lines[i].TargetDocNo < lines[j].TargetDocNo
	})
	return lines, nil
}

// settlementTable 把核销对象翻译成表名。非法取值在入口就拒掉：
// 静默回退到应收会让"付款方向的账龄"悄悄变成客户欠款。
func settlementTable(target model.SettlementTarget) (string, error) {
	switch target {
	case model.SettlementTargetReceivable:
		return "biz_receivable", nil
	case model.SettlementTargetPayable:
		return "biz_payable", nil
	default:
		return "", errs.BadRequest("核销对象类型不合法：10=应收单，20=应付单")
	}
}
