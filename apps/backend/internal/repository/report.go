package repository

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/errs"
)

// OutstandingSettlement 是一张还没结清的单据，账龄报表的原料。
//
// 只取报表需要的列：报表要的是"谁欠多少、什么时候到期"，
// 把整张应收单（含来源单号、备注、状态机）读进来只会让聚合多背一份内存。
type OutstandingSettlement struct {
	PartnerID         uint
	PartnerCode       string
	PartnerName       string
	CurrencyCode      string
	Amount            float64
	SettledAmount     float64
	AmountBase        float64
	SettledAmountBase float64
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
	// 过滤条件里的 amount > settled_amount 用 0.0001 的余量：分批收付累加到
	// 最后一笔时可能差一个末位，那种单子已经结清，不该再出现在账龄里。
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
	SumChargesByShipment(ctx context.Context, shipmentIDs []uint) (map[uint]float64, error)
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
	Qty float64
	// ItemQty / ItemAmountBase 是所属销售行的整行数量与本币金额，用于按比例分摊收入。
	ItemQty        float64
	ItemAmountBase float64
}

// PurchaseCost 是某条销售订单行累计的采购数量与本币金额。
type PurchaseCost struct {
	Qty        float64
	AmountBase float64
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

	rows := make([]OutstandingSettlement, 0, 64)
	// 用 Table 而不是 Model：两张表结构相同，写成 Model(&model.Receivable{}) +
	// 切表名反而绕。代价是软删除条件要自己写 —— 漏了就会把已删的单据算进账龄。
	err = r.DB().WithContext(ctx).
		Table(table+" AS s").
		Select(`s.partner_id,
			p.code AS partner_code,
			p.name AS partner_name,
			s.currency_code,
			s.amount,
			s.settled_amount,
			s.amount_base,
			s.settled_amount_base,
			s.due_date`).
		Joins("LEFT JOIN md_partner AS p ON p.id = s.partner_id AND p.deleted_at IS NULL").
		Where("s.deleted_at IS NULL").
		Where("s.status <> ?", model.SettlementStatusVoid).
		Where("s.amount - s.settled_amount > ?", 0.0001).
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
		q := r.DB().WithContext(ctx).
			Table("biz_shipment AS s").
			Where("s.deleted_at IS NULL").
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
	err := r.DB().WithContext(ctx).
		Table("biz_shipment_allocation AS a").
		Select(`a.shipment_id,
			a.sales_order_item_id,
			a.qty,
			i.qty AS item_qty,
			i.amount_base AS item_amount_base`).
		Joins("JOIN biz_sales_order_item AS i ON i.id = a.sales_order_item_id AND i.deleted_at IS NULL").
		Where("a.deleted_at IS NULL").
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
		Qty              float64
		AmountBase       float64
	}
	err := r.DB().WithContext(ctx).
		Table("biz_purchase_order_item AS i").
		Select("i.sales_order_item_id, SUM(i.qty) AS qty, SUM(i.amount_base) AS amount_base").
		Joins("JOIN biz_purchase_order AS o ON o.id = i.purchase_order_id AND o.deleted_at IS NULL").
		Where("i.deleted_at IS NULL").
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
) (map[uint]float64, error) {
	out := make(map[uint]float64, len(shipmentIDs))
	if len(shipmentIDs) == 0 {
		return out, nil
	}

	var rows []struct {
		ShipmentID uint
		AmountBase float64
	}
	err := r.DB().WithContext(ctx).
		Table("biz_shipment_charge AS c").
		Select("c.shipment_id, SUM(c.amount_base) AS amount_base").
		Where("c.deleted_at IS NULL").
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
