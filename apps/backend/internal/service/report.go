package service

import (
	"context"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/errs"
	"github.com/edwinhuish/gowork/internal/pkg/money"
	"github.com/edwinhuish/gowork/internal/repository"
)

// ReportService 是只读聚合服务：账龄（T-22）与单票毛利（T-22）。
//
// 它不持有任何单据的写权限，也不缓存：报表的每个数字都要能和明细逐笔对上，
// 缓存住一份旧数只会让"对不上"变成一次扯皮。数据量真的上来之后再谈缓存，
// 那时也应该由调用方决定缓存多久，而不是埋在聚合里。
type ReportService interface {
	Aging(ctx context.Context, q dto.AgingQuery) (*dto.AgingReport, error)
	ShipmentProfits(
		ctx context.Context, q dto.ShipmentProfitQuery,
	) (*dto.PageResult[dto.ShipmentProfit], error)
	// ExchangeGains 逐笔给出汇兑损益（T-23）：到账汇率与单据锁定汇率之差。
	ExchangeGains(
		ctx context.Context, q dto.ExchangeGainQuery,
	) (*dto.ExchangeGainReport, error)
	// SalesDashboard 按客户 / 产品 / 国家 / 月汇总订单额与毛利（T-28）。
	// 口径复用 ShipmentProfits，换维度只换分组键。
	SalesDashboard(ctx context.Context, q dto.SalesDashboardQuery) (*dto.SalesDashboard, error)
	// Collection 给出期间回款率与截至 as_of 的超期明细（T-29）。
	Collection(ctx context.Context, q dto.CollectionQuery) (*dto.CollectionReport, error)
	// Partner360 聚一个主体的订单、出货、应收、跟进、商机（T-30）。
	Partner360(ctx context.Context, q dto.Partner360Query) (*dto.Partner360, error)
}

type reportService struct {
	repo  repository.ReportRepository
	clock func() time.Time
	log   *slog.Logger
}

type ReportOption func(*reportService)

func WithReportClock(now func() time.Time) ReportOption {
	return func(s *reportService) { s.clock = now }
}

func WithReportLogger(l *slog.Logger) ReportOption {
	return func(s *reportService) { s.log = l }
}

func NewReportService(repo repository.ReportRepository, opts ...ReportOption) ReportService {
	s := &reportService{repo: repo, clock: time.Now, log: slog.Default()}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Aging 按主体给出未核销金额的账龄分桶。
//
// 三件事只在 service 做，不下推成 SQL：
//  1. 分桶边界与"逾期从哪天算"是业务口径，换库不该跟着换；
//  2. 跨币种只能加本币，这条规则要让读代码的人一眼看见；
//  3. 未结清单据的规模是"还没收回来的账"，天然有上限（几百到几千行），
//     在内存里聚合比写三段各库方言不同的日期函数更划算。
func (s *reportService) Aging(
	ctx context.Context, q dto.AgingQuery,
) (*dto.AgingReport, error) {
	target, err := agingTarget(q.Direction)
	if err != nil {
		return nil, err
	}

	asOf := model.DateOnly(s.clock())
	if strings.TrimSpace(q.AsOf) != "" {
		d, err := parseDateOnly(q.AsOf)
		if err != nil {
			return nil, err
		}
		asOf = d
	}

	docs, err := s.repo.ListOutstandingSettlements(ctx, target)
	if err != nil {
		return nil, err
	}

	rows := aggregateAging(docs, asOf, q.PartnerID)

	totalBase := sumBuckets(rows)
	report := &dto.AgingReport{
		Direction:      int8(target),
		AsOf:           asOf.Format(rateDateLayout),
		Rows:           rows,
		TotalBase:      totalBase,
		GrandTotalBase: totalBase.Total(),
	}

	s.log.Debug("账龄报表", "direction", target.Label(), "as_of", report.AsOf,
		"rows", len(rows), "total_base", report.GrandTotalBase)
	return report, nil
}

// ShipmentProfits 按票算毛利：收入 − 采购成本 − 运杂费。
//
// 三次批量查询而不是逐票查：一页 20 票若各查三次，就是 60 次往返；
// 而这三样（分配行、采购成本、运杂费）本来就能按 id 集合一次取回。
func (s *reportService) ShipmentProfits(
	ctx context.Context, q dto.ShipmentProfitQuery,
) (*dto.PageResult[dto.ShipmentProfit], error) {
	q.PageQuery.Normalize()
	page := repository.Pagination{Page: q.Page, PageSize: q.PageSize}

	heads, total, err := s.repo.ListShipments(
		ctx, repository.ShipmentProfitFilter{ShipmentID: q.ShipmentID}, page)
	if err != nil {
		return nil, err
	}

	ids := make([]uint, 0, len(heads))
	for _, h := range heads {
		ids = append(ids, h.ID)
	}

	lines, err := s.repo.ListShipmentLines(ctx, ids)
	if err != nil {
		return nil, err
	}
	byShipment := make(map[uint][]repository.ShipmentLine, len(heads))
	itemIDs := make([]uint, 0, len(lines))
	seen := make(map[uint]struct{}, len(lines))
	for _, l := range lines {
		byShipment[l.ShipmentID] = append(byShipment[l.ShipmentID], l)
		if _, ok := seen[l.SalesOrderItemID]; !ok {
			seen[l.SalesOrderItemID] = struct{}{}
			itemIDs = append(itemIDs, l.SalesOrderItemID)
		}
	}

	costs, err := s.repo.PurchaseCostBySalesItem(ctx, itemIDs)
	if err != nil {
		return nil, err
	}
	charges, err := s.repo.SumChargesByShipment(ctx, ids)
	if err != nil {
		return nil, err
	}

	list := make([]dto.ShipmentProfit, 0, len(heads))
	for _, h := range heads {
		list = append(list, buildShipmentProfit(h, byShipment[h.ID], costs, charges[h.ID]))
	}

	s.log.Debug("单票毛利", "shipments", len(list), "total", total)
	return dto.NewPageResult(list, total, q.Page, q.PageSize), nil
}

// ExchangeGains 逐笔算汇兑损益。
//
// 符号由核销对象决定，不由金额正负决定：
//   - 应收：到账汇率高说明同样的美元换回更多人民币 → 收益（正）；
//   - 应付：付款汇率高说明同样的美元多付了人民币 → 损失（负）。
//
// 两个方向都算作"到账本币 − 单据本币"，只差一个负号。把它写反了，
// 整张表的损益会整体变号，而数字看起来很"正常"——所以两个方向各有一条测试。
func (s *reportService) ExchangeGains(
	ctx context.Context, q dto.ExchangeGainQuery,
) (*dto.ExchangeGainReport, error) {
	filter, from, to, err := buildExchangeGainFilter(q)
	if err != nil {
		return nil, err
	}

	raw, err := s.repo.ListExchangeGains(ctx, filter)
	if err != nil {
		return nil, err
	}

	report := &dto.ExchangeGainReport{
		From:  from,
		To:    to,
		Lines: make([]dto.ExchangeGainLine, 0, len(raw)),
	}

	subtotals := make(map[string]*dto.ExchangeGainSubtotal, 4)
	for _, l := range raw {
		documentBase := l.Amount.Mul(l.DocumentRate).Round(4)
		gain := l.AmountBase.Sub(documentBase).Round(4)
		// 应付方向取反：多付出去的钱是损失。
		if l.TargetType == model.SettlementTargetPayable {
			gain = gain.Neg()
		}

		report.Lines = append(report.Lines, dto.ExchangeGainLine{
			PaymentID:    l.PaymentID,
			PaymentDocNo: l.PaymentDocNo,
			PayDate:      model.DateOnly(l.PayDate).Format(rateDateLayout),
			Direction:    int8(l.Direction),
			PartnerID:    l.PartnerID,
			PartnerCode:  l.PartnerCode,
			PartnerName:  l.PartnerName,
			CurrencyCode: l.CurrencyCode,
			TargetType:   int8(l.TargetType),
			TargetDocNo:  l.TargetDocNo,
			Amount:       l.Amount.Round(4),
			AmountBase:   l.AmountBase.Round(4),
			PaymentRate:  l.PaymentRate,
			DocumentRate: l.DocumentRate,
			DocumentBase: documentBase,
			GainBase:     gain,
		})

		sub, ok := subtotals[l.CurrencyCode]
		if !ok {
			sub = &dto.ExchangeGainSubtotal{CurrencyCode: l.CurrencyCode}
			subtotals[l.CurrencyCode] = sub
		}
		sub.Amount = sub.Amount.Add(l.Amount)
		sub.GainBase = sub.GainBase.Add(gain)
		report.TotalGainBase = report.TotalGainBase.Add(gain)
	}

	// 小计按币种编码排序：map 的遍历顺序每次都不同，直接输出会让同一张表两次刷新换序。
	codes := make([]string, 0, len(subtotals))
	for code := range subtotals {
		codes = append(codes, code)
	}
	sort.Strings(codes)
	report.Subtotals = make([]dto.ExchangeGainSubtotal, 0, len(codes))
	for _, code := range codes {
		sub := subtotals[code]
		report.Subtotals = append(report.Subtotals, dto.ExchangeGainSubtotal{
			CurrencyCode: sub.CurrencyCode,
			Amount:       sub.Amount.Round(4),
			GainBase:     sub.GainBase.Round(4),
		})
	}
	report.TotalGainBase = report.TotalGainBase.Round(4)

	s.log.Debug("汇兑损益", "from", report.From, "to", report.To,
		"lines", len(report.Lines), "total_base", report.TotalGainBase)
	return report, nil
}

// buildExchangeGainFilter 解析日期区间。留空表示不限，不是"今天"——
// 一张损益表默认看全部，比默认看空白的今天更符合"先看总量再筛"的用法。
func buildExchangeGainFilter(
	q dto.ExchangeGainQuery,
) (repository.ExchangeGainFilter, string, string, error) {
	filter := repository.ExchangeGainFilter{
		PartnerID:    q.PartnerID,
		CurrencyCode: strings.TrimSpace(q.CurrencyCode),
	}
	fromText := strings.TrimSpace(q.From)
	toText := strings.TrimSpace(q.To)

	if fromText != "" {
		from, err := parseDateOnly(fromText)
		if err != nil {
			return filter, "", "", err
		}
		filter.From = &from
	}
	if toText != "" {
		to, err := parseDateOnly(toText)
		if err != nil {
			return filter, "", "", err
		}
		filter.To = &to
	}
	if filter.From != nil && filter.To != nil && filter.From.After(*filter.To) {
		return filter, "", "", errs.BadRequest("起始日不能晚于截止日")
	}
	return filter, fromText, toText, nil
}

// buildShipmentProfit 把一票货的原料算成毛利。
//
// 两个分摊口径：
//   - 收入：本票数量 ÷ 销售行数量 × 该行本币金额。用比例而不是单价×数量，
//     是为了让折扣与行内舍入跟着分摊，各行之和与订单总额对得上；
//   - 成本：该销售行累计采购的本币金额 ÷ 累计数量（加权单价）× 本票数量。
//     按加权单价而不是按比例分摊采购总额，部分出货时才不会把整行的采购成本
//     一次性压到第一票上。
func buildShipmentProfit(
	head repository.ShipmentHead,
	lines []repository.ShipmentLine,
	costs map[uint]repository.PurchaseCost,
	charge money.Decimal,
) dto.ShipmentProfit {
	revenue, cost := money.Zero(), money.Zero()
	missing := false

	for _, l := range lines {
		// 分摊比例也走定点：数量与金额都已是 Decimal，比例不必再绕 float64。
		if l.ItemQty.Cmp(money.Zero()) > 0 {
			revenue = revenue.Add(l.ItemAmountBase.Mul(l.Qty.Div(l.ItemQty)))
		}

		pc, ok := costs[l.SalesOrderItemID]
		if !ok || pc.Qty.Cmp(money.Zero()) <= 0 {
			// 没有采购记录不等于成本为 0：这一行的毛利算不出来，
			// 标出来比给一个漂亮的数字有用。
			missing = true
			continue
		}
		cost = cost.Add(pc.AmountBase.Mul(l.Qty.Div(pc.Qty)))
	}

	revenue = revenue.Round(4)
	cost = cost.Round(4)
	charge = charge.Round(4)
	profit := revenue.Sub(cost).Sub(charge).Round(4)

	margin := money.Zero()
	if revenue.Cmp(money.Zero()) > 0 {
		margin = profit.Div(revenue).Round(4)
	}

	return dto.ShipmentProfit{
		ShipmentID:  head.ID,
		DocNo:       head.DocNo,
		ShipDate:    model.DateOnly(head.ShipDate).Format(rateDateLayout),
		Status:      int8(head.Status),
		Revenue:     revenue,
		Cost:        cost,
		Charge:      charge,
		Profit:      profit,
		Margin:      margin,
		CostMissing: missing,
	}
}

// agingTarget 把查询参数翻成核销对象：留空按应收（这张报表最常被问到的一面）。
func agingTarget(direction *int8) (model.SettlementTarget, error) {
	if direction == nil {
		return model.SettlementTargetReceivable, nil
	}
	target := model.SettlementTarget(*direction)
	switch target {
	case model.SettlementTargetReceivable, model.SettlementTargetPayable:
		return target, nil
	default:
		return 0, errs.BadRequest("方向不合法：10=应收，20=应付")
	}
}

// aggregateAging 把未结清单据按「主体 + 币种」聚成行，并按逾期天数分桶。
func aggregateAging(
	docs []repository.OutstandingSettlement, asOf time.Time, partnerID *uint,
) []dto.AgingRow {
	type rowKey struct {
		partnerID uint
		currency  string
	}

	acc := make(map[rowKey]*dto.AgingRow, len(docs))
	earliest := make(map[rowKey]time.Time, len(docs))

	for _, d := range docs {
		if partnerID != nil && d.PartnerID != *partnerID {
			continue
		}
		key := rowKey{partnerID: d.PartnerID, currency: d.CurrencyCode}
		row, ok := acc[key]
		if !ok {
			row = &dto.AgingRow{
				PartnerID:    d.PartnerID,
				PartnerCode:  d.PartnerCode,
				PartnerName:  d.PartnerName,
				CurrencyCode: d.CurrencyCode,
			}
			acc[key] = row
		}

		due := model.DateOnly(d.DueDate)
		bucket := agingBucket(agingDays(asOf, due))
		row.Amount.Add(bucket, d.Amount.Sub(d.SettledAmount).Round(4))
		row.AmountBase.Add(bucket, d.AmountBase.Sub(d.SettledAmountBase).Round(4))

		if cur, ok := earliest[key]; !ok || due.Before(cur) {
			earliest[key] = due
		}
	}

	rows := make([]dto.AgingRow, 0, len(acc))
	for key, row := range acc {
		row.Total = row.Amount.Total()
		row.TotalBase = row.AmountBase.Total()
		row.EarliestDueDate = earliest[key].Format(rateDateLayout)
		rows = append(rows, *row)
	}

	// 排序与 SQL 的 ORDER BY 同义：先主体编码、再币种，
	// 同一主体的多币种行因此相邻，报表读起来是"一个客户几行"。
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].PartnerCode != rows[j].PartnerCode {
			return rows[i].PartnerCode < rows[j].PartnerCode
		}
		if rows[i].PartnerName != rows[j].PartnerName {
			return rows[i].PartnerName < rows[j].PartnerName
		}
		return rows[i].CurrencyCode < rows[j].CurrencyCode
	})
	return rows
}

// sumBuckets 把各行按桶加成本币合计。原币不合计：跨币种的加法没有意义。
func sumBuckets(rows []dto.AgingRow) dto.AgingBuckets {
	var total dto.AgingBuckets
	for _, row := range rows {
		total.NotDue = total.NotDue.Add(row.AmountBase.NotDue)
		total.Days1To30 = total.Days1To30.Add(row.AmountBase.Days1To30)
		total.Days31To60 = total.Days31To60.Add(row.AmountBase.Days31To60)
		total.Days61To90 = total.Days61To90.Add(row.AmountBase.Days61To90)
		total.Over90 = total.Over90.Add(row.AmountBase.Over90)
	}
	return total
}

// agingDays 返回相对 asOf 的逾期天数：正数是已逾期天数，0 与负数表示未到期。
//
// 两端都先归一到日期再相减：到期日存的是 date，若拿带时分秒的 now 去比，
// 到期当天会算成"逾期 0.6 天"再被取整成 0，而第二天凌晨又跳一格，边界不可复现。
func agingDays(asOf, due time.Time) int {
	return int(model.DateOnly(asOf).Sub(model.DateOnly(due)).Hours() / 24)
}

// agingBucket 是分桶规则本体：0 未到期 / 1 1-30 / 2 31-60 / 3 61-90 / 4 90+。
//
// 到期当天算未到期（days = 0）：账期 30 天说的是"第 30 天付"，第 30 天还在
// 承诺期内，第 31 天才叫逾期。
func agingBucket(days int) int {
	switch {
	case days <= 0:
		return 0
	case days <= 30:
		return 1
	case days <= 60:
		return 2
	case days <= 90:
		return 3
	default:
		return 4
	}
}
