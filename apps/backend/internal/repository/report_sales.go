package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/money"
)

// SalesFilter 是看板的时间范围，两端都是闭区间；nil 表示不限。
//
// 两个口径各按自己的业务日期筛：出货毛利看出货日，订单额看下单日。
// 用同一个日期去筛两件事，会让"8 月下单、9 月出货"这种常态偷偷落到没人预期的月份里。
type SalesFilter struct {
	From *time.Time
	To   *time.Time
}

// SalesShipmentLine 是一条出货分配行在报表里的原料。
//
// 带上客户与产品两套维度字段，是为了让聚合只在 service 里做：先按出货行取一次，
// 换维度就只是换分组键。若按维度分别写 SQL，四个维度就有四份口径，
// 迟早出现"按客户看是 21%，按产品看是 19%"这种谁也解释不清的差。
type SalesShipmentLine struct {
	ShipmentID       uint
	ShipDate         time.Time
	PartnerID        uint
	PartnerCode      string
	PartnerName      string
	Country          string
	ProductID        uint
	SKUCode          string
	ProductName      string
	SalesOrderItemID uint
	// Qty 是本票出货数量；ItemQty / ItemAmountBase 是所属销售行的整行数量与本币金额。
	Qty            money.Decimal
	ItemQty        money.Decimal
	ItemAmountBase money.Decimal
}

// ListSalesShipmentLines 取范围内参与统计的出货分配行。
//
// 草稿与已取消的票不进统计（与单票毛利同一条件）：草稿的数量还会改，
// 取消的货根本没出，算进来会让看板跟着草稿的编辑记录跳动。
func (r *reportRepo) ListSalesShipmentLines(
	ctx context.Context, f SalesFilter,
) ([]SalesShipmentLine, error) {
	lines := make([]SalesShipmentLine, 0, 256)
	q := r.conn(ctx).
		Table("biz_shipment_allocation AS a").
		Select(`a.shipment_id,
			s.ship_date,
			o.partner_id,
			COALESCE(pt.code, '') AS partner_code,
			COALESCE(pt.name, '') AS partner_name,
			COALESCE(pt.country, '') AS country,
			i.product_id,
			i.sku_code,
			i.name_cn AS product_name,
			a.sales_order_item_id,
			a.qty,
			i.qty AS item_qty,
			i.amount_base AS item_amount_base`).
		Joins("JOIN biz_shipment AS s ON s.id = a.shipment_id AND s.deleted_at = 0").
		Joins("JOIN biz_sales_order_item AS i ON i.id = a.sales_order_item_id AND i.deleted_at = 0").
		Joins("JOIN biz_sales_order AS o ON o.id = i.sales_order_id AND o.deleted_at = 0").
		Joins("LEFT JOIN md_partner AS pt ON pt.id = o.partner_id AND pt.deleted_at = 0").
		Where("a.deleted_at = 0").
		Where("s.status NOT IN ?", []model.DocumentStatus{
			model.DocumentStatusDraft, model.DocumentStatusCancelled,
		})
	if f.From != nil {
		q = q.Where("s.ship_date >= ?", *f.From)
	}
	if f.To != nil {
		q = q.Where("s.ship_date <= ?", *f.To)
	}

	if err := q.Scan(&lines).Error; err != nil {
		return nil, fmt.Errorf("查询出货分配行（看板）: %w", err)
	}
	return lines, nil
}

// SalesOrderLine 是一条销售订单明细行，只用来算「订单额」这一列。
type SalesOrderLine struct {
	OrderDate   time.Time
	PartnerID   uint
	PartnerCode string
	PartnerName string
	Country     string
	ProductID   uint
	SKUCode     string
	ProductName string
	AmountBase  money.Decimal
}

// ListSalesOrderLines 取范围内非草稿非取消的销售订单明细行。
//
// 按**明细行**而不是按订单取：看板要能按产品分组，
// 订单头只有一个客户，拆不到产品上。
func (r *reportRepo) ListSalesOrderLines(
	ctx context.Context, f SalesFilter,
) ([]SalesOrderLine, error) {
	lines := make([]SalesOrderLine, 0, 256)
	q := r.conn(ctx).
		Table("biz_sales_order_item AS i").
		Select(`o.order_date,
			o.partner_id,
			COALESCE(pt.code, '') AS partner_code,
			COALESCE(pt.name, '') AS partner_name,
			COALESCE(pt.country, '') AS country,
			i.product_id,
			i.sku_code,
			i.name_cn AS product_name,
			i.amount_base`).
		Joins("JOIN biz_sales_order AS o ON o.id = i.sales_order_id AND o.deleted_at = 0").
		Joins("LEFT JOIN md_partner AS pt ON pt.id = o.partner_id AND pt.deleted_at = 0").
		Where("i.deleted_at = 0").
		Where("o.status NOT IN ?", []model.DocumentStatus{
			model.DocumentStatusDraft, model.DocumentStatusCancelled,
		})
	if f.From != nil {
		q = q.Where("o.order_date >= ?", *f.From)
	}
	if f.To != nil {
		q = q.Where("o.order_date <= ?", *f.To)
	}

	if err := q.Scan(&lines).Error; err != nil {
		return nil, fmt.Errorf("查询销售订单行（看板）: %w", err)
	}
	return lines, nil
}
