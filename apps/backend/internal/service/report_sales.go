package service

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/pkg/errs"
	"github.com/edwinhuish/gowork/internal/pkg/money"
	"github.com/edwinhuish/gowork/internal/repository"
)

// 看板的维度取值。
//
// 业务员维度要等 T-32 把单据的 owner_id 做出来：现在没有任何列能回答"这单是谁的"，
// 给一个 400 比让这个维度静默消失好——后者会让人以为"公司没有业务员业绩这回事"。
const (
	salesDimCustomer = "customer"
	salesDimProduct  = "product"
	salesDimCountry  = "country"
	salesDimMonth    = "month"
)

// 看板返回行数的默认值与上限。上限不是防用户，是防一次把几万行塞进响应体：
// 看板是用来找"该看哪几行"的，全量明细该去导出。
const (
	salesDefaultLimit = 20
	salesMaxLimit     = 500
)

// salesAxis 是一条数据在四个维度上的坐标。
//
// 出货行与订单行共用它：换维度就是换一把 key，取数口径不变。
type salesAxis struct {
	partnerID   uint
	partnerCode string
	partnerName string
	country     string
	productID   uint
	skuCode     string
	productName string
	// date 对该行自己的业务日期：出货行是出货日，订单行是下单日。
	date time.Time
}

// keyOf 按维度给出分组键与显示名。
func (a salesAxis) keyOf(dim string) (string, string) {
	switch dim {
	case salesDimProduct:
		label := strings.TrimSpace(a.skuCode)
		if name := strings.TrimSpace(a.productName); name != "" {
			if label != "" {
				label += " · "
			}
			label += name
		}
		if label == "" {
			label = "未填产品"
		}
		return strconv.FormatUint(uint64(a.productID), 10), label
	case salesDimCountry:
		country := strings.TrimSpace(a.country)
		if country == "" {
			// 没填国家的主体照样要出现在表里：把它归进"未填"，比让它从看板上消失好。
			return "", "未填国家"
		}
		return country, country
	case salesDimMonth:
		month := a.date.Format("2006-01")
		return month, month
	default: // salesDimCustomer
		label := strings.TrimSpace(a.partnerCode)
		if name := strings.TrimSpace(a.partnerName); name != "" {
			if label != "" {
				label += " · "
			}
			label += name
		}
		if label == "" {
			label = "未填客户"
		}
		return strconv.FormatUint(uint64(a.partnerID), 10), label
	}
}

// salesBucket 是聚合中的一个桶，金额全部本币。
type salesBucket struct {
	label       string
	orderAmount money.Decimal
	revenue     money.Decimal
	cost        money.Decimal
	charge      money.Decimal
	shipments   map[uint]struct{}
	missing     bool
}

// SalesDashboard 按维度汇总订单额与毛利（T-28）。
//
// 口径与 ShipmentProfits **完全相同**：收入按「本票数量 ÷ 销售行数量」分摊行金额，
// 成本按采购加权单价 × 本票数量，运杂费按收入占比摊到各出货行，毛利 = 收入 − 成本 − 运杂费。
// 差别只在最后一步的分组——正因为每个数字都先摊到**出货行**上，
// 换维度只是换分组键，同一时期的合计在任何维度下都相等。
func (s *reportService) SalesDashboard(
	ctx context.Context, q dto.SalesDashboardQuery,
) (*dto.SalesDashboard, error) {
	dim, err := salesDimension(q.Dimension)
	if err != nil {
		return nil, err
	}
	filter, labelFrom, labelTo, err := salesDateRange(q.From, q.To)
	if err != nil {
		return nil, err
	}
	limit, err := salesLimit(q.Limit)
	if err != nil {
		return nil, err
	}

	lines, err := s.repo.ListSalesShipmentLines(ctx, filter)
	if err != nil {
		return nil, err
	}

	itemIDs := make([]uint, 0, len(lines))
	shipmentIDs := make([]uint, 0, len(lines))
	seenItem := make(map[uint]struct{}, len(lines))
	seenShipment := make(map[uint]struct{}, len(lines))
	for _, l := range lines {
		if _, ok := seenItem[l.SalesOrderItemID]; !ok {
			seenItem[l.SalesOrderItemID] = struct{}{}
			itemIDs = append(itemIDs, l.SalesOrderItemID)
		}
		if _, ok := seenShipment[l.ShipmentID]; !ok {
			seenShipment[l.ShipmentID] = struct{}{}
			shipmentIDs = append(shipmentIDs, l.ShipmentID)
		}
	}

	costs, err := s.repo.PurchaseCostBySalesItem(ctx, itemIDs)
	if err != nil {
		return nil, err
	}
	charges, err := s.repo.SumChargesByShipment(ctx, shipmentIDs)
	if err != nil {
		return nil, err
	}

	// 第一遍：收入与成本，同时把每票的收入合计出来——运杂费要按它分摊。
	type computedLine struct {
		axis       salesAxis
		shipmentID uint
		revenue    money.Decimal
		cost       money.Decimal
		missing    bool
	}
	computed := make([]computedLine, 0, len(lines))
	revenueByShipment := make(map[uint]money.Decimal, len(shipmentIDs))
	for _, l := range lines {
		revenue := money.Zero()
		// 分摊比例也走定点：数量与金额都已是 Decimal，比例不必再绕 float64。
		if l.ItemQty.Cmp(money.Zero()) > 0 {
			revenue = l.ItemAmountBase.Mul(l.Qty.Div(l.ItemQty))
		}

		cost := money.Zero()
		missing := false
		pc, ok := costs[l.SalesOrderItemID]
		if !ok || pc.Qty.Cmp(money.Zero()) <= 0 {
			// 没有采购记录不等于成本为 0：这一行的毛利算不出来，标出来比给个漂亮的数字有用。
			missing = true
		} else {
			cost = pc.AmountBase.Mul(l.Qty.Div(pc.Qty))
		}

		revenueByShipment[l.ShipmentID] = revenueByShipment[l.ShipmentID].Add(revenue)
		computed = append(computed, computedLine{
			axis: salesAxis{
				partnerID: l.PartnerID, partnerCode: l.PartnerCode, partnerName: l.PartnerName,
				country: l.Country, productID: l.ProductID,
				skuCode: l.SKUCode, productName: l.ProductName, date: l.ShipDate,
			},
			shipmentID: l.ShipmentID,
			revenue:    revenue,
			cost:       cost,
			missing:    missing,
		})
	}

	// 运杂费按票发生，但看板要按行归集：按收入占比摊到每行。
	// 每票最后一行的份额取「票上总额 − 已经摊出去的」，这样四舍五入的零头不会丢，
	// 各维度算出来的运杂费合计与单票毛利逐票相加完全相等。
	remaining := make(map[uint]money.Decimal, len(charges))
	for id, v := range charges {
		remaining[id] = v
	}
	lastIndex := make(map[uint]int, len(shipmentIDs))
	for i, l := range computed {
		lastIndex[l.shipmentID] = i
	}
	lineCharge := make([]money.Decimal, len(computed))
	for i, l := range computed {
		if lastIndex[l.shipmentID] == i {
			lineCharge[i] = remaining[l.shipmentID]
			continue
		}
		total := charges[l.shipmentID]
		base := revenueByShipment[l.shipmentID]
		share := money.Zero()
		if total.IsPositive() && base.IsPositive() {
			share = total.Mul(l.revenue.Div(base)).Round(4)
		}
		lineCharge[i] = share
		remaining[l.shipmentID] = remaining[l.shipmentID].Sub(share)
	}

	buckets := make(map[string]*salesBucket, 16)
	bucket := func(key, label string) *salesBucket {
		b, ok := buckets[key]
		if !ok {
			b = &salesBucket{label: label, shipments: make(map[uint]struct{}, 4)}
			buckets[key] = b
		}
		return b
	}

	for i, l := range computed {
		key, label := l.axis.keyOf(dim)
		b := bucket(key, label)
		b.revenue = b.revenue.Add(l.revenue)
		b.cost = b.cost.Add(l.cost)
		b.charge = b.charge.Add(lineCharge[i])
		b.shipments[l.shipmentID] = struct{}{}
		if l.missing {
			b.missing = true
		}
	}

	orders, err := s.repo.ListSalesOrderLines(ctx, filter)
	if err != nil {
		return nil, err
	}
	for _, o := range orders {
		key, label := salesAxis{
			partnerID: o.PartnerID, partnerCode: o.PartnerCode, partnerName: o.PartnerName,
			country: o.Country, productID: o.ProductID,
			skuCode: o.SKUCode, productName: o.ProductName, date: o.OrderDate,
		}.keyOf(dim)
		b := bucket(key, label)
		b.orderAmount = b.orderAmount.Add(o.AmountBase)
	}

	// 合计从**未舍入**的桶里加：先按行四舍五入再相加，会和真实合计差出几分钱，
	// 而报表上的合计是被人拿去对账的。
	total := &salesBucket{shipments: make(map[uint]struct{}, len(shipmentIDs))}
	rows := make([]dto.SalesDashboardRow, 0, len(buckets))
	for key, b := range buckets {
		rows = append(rows, buildSalesRow(key, b))
		total.orderAmount = total.orderAmount.Add(b.orderAmount)
		total.revenue = total.revenue.Add(b.revenue)
		total.cost = total.cost.Add(b.cost)
		total.charge = total.charge.Add(b.charge)
		total.missing = total.missing || b.missing
		for id := range b.shipments {
			total.shipments[id] = struct{}{}
		}
	}

	sortSalesRows(rows, dim)

	truncated := false
	if len(rows) > limit {
		rows = rows[:limit]
		truncated = true
	}

	report := &dto.SalesDashboard{
		Dimension: dim,
		From:      labelFrom,
		To:        labelTo,
		Rows:      rows,
		Total:     buildSalesRow("", total),
		Truncated: truncated,
	}

	s.log.Debug("销售看板", "dimension", dim, "rows", len(rows),
		"revenue", report.Total.Revenue, "profit", report.Total.Profit, "truncated", truncated)
	return report, nil
}

func buildSalesRow(key string, b *salesBucket) dto.SalesDashboardRow {
	revenue := b.revenue.Round(4)
	cost := b.cost.Round(4)
	charge := b.charge.Round(4)
	profit := revenue.Sub(cost).Sub(charge).Round(4)

	margin := money.Zero()
	if revenue.Cmp(money.Zero()) > 0 {
		margin = profit.Div(revenue).Round(4)
	}

	return dto.SalesDashboardRow{
		Key:         key,
		Label:       b.label,
		OrderAmount: b.orderAmount.Round(4),
		Revenue:     revenue,
		Cost:        cost,
		Charge:      charge,
		Profit:      profit,
		Margin:      margin,
		Shipments:   len(b.shipments),
		CostMissing: b.missing,
	}
}

// sortSalesRows 决定谁排前面。
//
// 月份按时间升序（趋势要顺着读），其余维度按收入降序：
// 看板的第一眼是"钱从哪来"，毛利异常的那几行由毛利率列自己露出来（前端列可排序）。
func sortSalesRows(rows []dto.SalesDashboardRow, dim string) {
	sort.SliceStable(rows, func(i, j int) bool {
		if dim == salesDimMonth {
			return rows[i].Key < rows[j].Key
		}
		if c := rows[i].Revenue.Cmp(rows[j].Revenue); c != 0 {
			return c > 0
		}
		return rows[i].Label < rows[j].Label
	})
}

func salesDimension(raw string) (string, error) {
	switch strings.TrimSpace(raw) {
	case "", salesDimCustomer:
		return salesDimCustomer, nil
	case salesDimProduct:
		return salesDimProduct, nil
	case salesDimCountry:
		return salesDimCountry, nil
	case salesDimMonth:
		return salesDimMonth, nil
	case "salesperson":
		return "", errs.BadRequest("业务员维度要等 T-32 把单据的 owner_id 做出来")
	default:
		return "", errs.BadRequest("维度取 customer / product / country / month")
	}
}

// salesDateRange 把两个 YYYY-MM-DD 解析成闭区间，并返回回显用的规范化字符串。
func salesDateRange(from, to string) (repository.SalesFilter, string, string, error) {
	fromPtr, toPtr, labelFrom, labelTo, err := parsePeriod(from, to)
	if err != nil {
		return repository.SalesFilter{}, "", "", err
	}
	return repository.SalesFilter{From: fromPtr, To: toPtr}, labelFrom, labelTo, nil
}

func salesLimit(raw *int) (int, error) {
	if raw == nil {
		return salesDefaultLimit, nil
	}
	if *raw < 0 {
		return 0, errs.BadRequest("limit 不能为负数")
	}
	if *raw == 0 || *raw > salesMaxLimit {
		return salesMaxLimit, nil
	}
	return *raw, nil
}
