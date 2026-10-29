package service

import (
	"context"
	"sort"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/errs"
	"github.com/edwinhuish/gowork/internal/pkg/money"
)

// ProductProfit 给出某个产品逐票的获利情况（T-31 的下钻）。
//
// 与看板同一份分摊（computeSalesLines）：同一票里这个产品的收入、成本、
// 摊到的运杂费三样都要能对上。按**票**聚合而不是按订单行——
// 分批出货时同一个产品在一票里可能出现两次，分成两行会让"这一票赚了多少"看不出来。
func (s *reportService) ProductProfit(
	ctx context.Context, q dto.ProductProfitQuery,
) (*dto.ProductProfitReport, error) {
	if q.ProductID == 0 {
		return nil, errs.BadRequest("请指定产品")
	}
	filter, labelFrom, labelTo, err := salesDateRange(q.From, q.To)
	if err != nil {
		return nil, err
	}
	filter.ProductID = &q.ProductID

	lines, err := s.computeSalesLines(ctx, filter)
	if err != nil {
		return nil, err
	}

	type acc struct {
		row   dto.ProductProfitRow
		order int
	}
	byShipment := make(map[uint]*acc, len(lines))
	for i, l := range lines {
		item, ok := byShipment[l.shipmentID]
		if !ok {
			item = &acc{
				row: dto.ProductProfitRow{
					ShipmentID:  l.shipmentID,
					DocNo:       l.shipmentNo,
					ShipDate:    model.DateOnly(l.axis.date).Format(rateDateLayout),
					PartnerID:   l.axis.partnerID,
					PartnerName: l.axis.partnerName,
				},
				order: i,
			}
			byShipment[l.shipmentID] = item
		}
		item.row.Qty = item.row.Qty.Add(l.qty)
		item.row.Revenue = item.row.Revenue.Add(l.revenue)
		item.row.Cost = item.row.Cost.Add(l.cost)
		item.row.Charge = item.row.Charge.Add(l.charge)
		item.row.CostMissing = item.row.CostMissing || l.missing
	}

	rows := make([]dto.ProductProfitRow, 0, len(byShipment))
	totalRevenue, totalCost, totalCharge := money.Zero(), money.Zero(), money.Zero()
	missing := false
	for _, item := range byShipment {
		revenue := item.row.Revenue.Round(4)
		cost := item.row.Cost.Round(4)
		charge := item.row.Charge.Round(4)
		item.row.Qty = item.row.Qty.Round(3)
		item.row.Revenue = revenue
		item.row.Cost = cost
		item.row.Charge = charge
		item.row.Profit = revenue.Sub(cost).Sub(charge).Round(4)

		// 合计从没舍入的数加：报表上的合计是被人拿去对账的。
		totalRevenue = totalRevenue.Add(item.row.Revenue)
		totalCost = totalCost.Add(item.row.Cost)
		totalCharge = totalCharge.Add(item.row.Charge)
		missing = missing || item.row.CostMissing
		rows = append(rows, item.row)
	}

	// 最近出的货排前面：下钻是为了看"这个产品最近卖得怎么样"。
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].ShipDate != rows[j].ShipDate {
			return rows[i].ShipDate > rows[j].ShipDate
		}
		return rows[i].DocNo > rows[j].DocNo
	})

	profit := totalRevenue.Sub(totalCost).Sub(totalCharge).Round(4)
	margin := money.Zero()
	if totalRevenue.Cmp(money.Zero()) > 0 {
		margin = profit.Div(totalRevenue).Round(4)
	}

	report := &dto.ProductProfitReport{
		ProductID:   q.ProductID,
		From:        labelFrom,
		To:          labelTo,
		Rows:        rows,
		Revenue:     totalRevenue.Round(4),
		Cost:        totalCost.Round(4),
		Charge:      totalCharge.Round(4),
		Profit:      profit,
		Margin:      margin,
		CostMissing: missing,
	}

	s.log.Debug("产品毛利下钻", "product", q.ProductID, "shipments", len(rows),
		"revenue", report.Revenue, "profit", report.Profit)
	return report, nil
}
