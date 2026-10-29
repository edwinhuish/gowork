package service

import (
	"context"
	"strings"
	"time"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/errs"
	"github.com/edwinhuish/gowork/internal/pkg/money"
)

// 客户 360 每块明细的行数。这一页是用来"判断接下来该跟哪一条"的，
// 每条链接都能点进源单据，全量去各自的列表页看。
const (
	partner360DefaultLimit = 10
	partner360MaxLimit     = 50
)

// Partner360 把一个主体的全貌聚到一页（T-30）。
//
// 为什么不是让前端各调五个列表接口：那样每块各带一套筛选与排序，
// "这家客户到底什么情况"就变成了五张互不相干的表；而且一屏五请求，
// 慢的那一个会把整页拖住。这里一次取全，页面只负责排布。
//
// 五个口径互不相同，都换算成本币再给合计：
//   - 订单：非草稿非取消，按订单的本币合计；
//   - 出货：含该客户订单行的票数（去重），拼柜时一票里有别家的货；
//   - 应收：未结清部分，超期按截止日算；
//   - 跟进 / 商机：商机只算未终结的。
func (s *reportService) Partner360(
	ctx context.Context, q dto.Partner360Query,
) (*dto.Partner360, error) {
	limit, err := partner360Limit(q.Limit)
	if err != nil {
		return nil, err
	}
	if q.PartnerID == 0 {
		return nil, errs.BadRequest("请指定主体")
	}

	// 先查抬头：主体不存在就 404，不必再跑后面五条查询。
	brief, err := s.repo.GetPartnerBrief(ctx, q.PartnerID)
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

	totals, err := s.repo.Partner360Totals(ctx, q.PartnerID)
	if err != nil {
		return nil, err
	}

	// 应收取**全部**未结清单据：合计必须是全量，截断只影响展示。
	receivables, err := s.repo.ListPartnerOutstandingReceivables(ctx, q.PartnerID)
	if err != nil {
		return nil, err
	}
	outstanding := money.Zero()
	overdue := money.Zero()
	receivableRows := make([]dto.Partner360Receivable, 0, len(receivables))
	for _, r := range receivables {
		base := r.AmountBase.Sub(r.SettledAmountBase).Round(4)
		outstanding = outstanding.Add(base)
		days := agingDays(asOf, model.DateOnly(r.DueDate))
		if days > 0 {
			overdue = overdue.Add(base)
		}
		receivableRows = append(receivableRows, dto.Partner360Receivable{
			ID:              r.ID,
			DocNo:           r.DocNo,
			CurrencyCode:    r.CurrencyCode,
			DueDate:         model.DateOnly(r.DueDate).Format(rateDateLayout),
			Outstanding:     r.Amount.Sub(r.SettledAmount).Round(4),
			OutstandingBase: base,
			OverdueDays:     max(days, 0),
		})
	}

	activities, err := s.repo.ListPartnerActivities(ctx, q.PartnerID, limit)
	if err != nil {
		return nil, err
	}
	opportunities, err := s.repo.ListPartnerOpportunities(ctx, q.PartnerID, limit)
	if err != nil {
		return nil, err
	}
	orders, err := s.repo.ListPartnerOrders(ctx, q.PartnerID, limit)
	if err != nil {
		return nil, err
	}
	shipments, err := s.repo.ListPartnerShipments(ctx, q.PartnerID, limit)
	if err != nil {
		return nil, err
	}

	// 应收明细在仓储里已按到期日升序（最早到期最该催），这里只截长度。
	if len(receivableRows) > limit {
		receivableRows = receivableRows[:limit]
	}

	report := &dto.Partner360{
		PartnerID:   brief.ID,
		PartnerCode: brief.Code,
		PartnerName: brief.Name,
		Country:     brief.Country,
		Role:        int8(brief.Role),
		AsOf:        asOf.Format(rateDateLayout),
		Totals: dto.Partner360Totals{
			OrderCount:                totals.OrderCount,
			OrderAmountBase:           totals.OrderAmountBase.Round(4),
			LastOrderDate:             dateOrEmpty(totals.LastOrderDate),
			ShipmentCount:             totals.ShipmentCount,
			ActivityCount:             totals.ActivityCount,
			LastActivityAt:            dateOrEmpty(totals.LastActivityAt),
			OpenOpportunityCount:      totals.OpenOpportunityCount,
			OpenOpportunityAmountBase: totals.OpenOpportunityAmountBase.Round(4),
			ReceivableOutstandingBase: outstanding.Round(4),
			ReceivableOverdueBase:     overdue.Round(4),
		},
		Activities:    make([]dto.Partner360Activity, 0, len(activities)),
		Opportunities: make([]dto.Partner360Opportunity, 0, len(opportunities)),
		Orders:        make([]dto.Partner360Order, 0, len(orders)),
		Shipments:     make([]dto.Partner360Shipment, 0, len(shipments)),
		Receivables:   receivableRows,
	}

	for _, a := range activities {
		report.Activities = append(report.Activities, dto.Partner360Activity{
			ID:           a.ID,
			Type:         a.Type,
			Subject:      a.Subject,
			OccurredAt:   model.DateOnly(a.OccurredAt).Format(rateDateLayout),
			NextFollowAt: dateOrEmpty(a.NextFollowAt),
		})
	}
	for _, o := range opportunities {
		report.Opportunities = append(report.Opportunities, dto.Partner360Opportunity{
			ID:                o.ID,
			Subject:           o.Subject,
			Stage:             int8(o.Stage),
			AmountBase:        o.AmountBase.Round(4),
			Probability:       o.Probability,
			ExpectedCloseDate: model.DateOnly(o.ExpectedCloseDate).Format(rateDateLayout),
		})
	}
	for _, o := range orders {
		report.Orders = append(report.Orders, dto.Partner360Order{
			ID:              o.ID,
			DocNo:           o.DocNo,
			OrderDate:       model.DateOnly(o.OrderDate).Format(rateDateLayout),
			Status:          int8(o.Status),
			CurrencyCode:    o.CurrencyCode,
			AmountBaseTotal: o.AmountBaseTotal.Round(4),
		})
	}
	for _, s := range shipments {
		report.Shipments = append(report.Shipments, dto.Partner360Shipment{
			ID:       s.ID,
			DocNo:    s.DocNo,
			ShipDate: model.DateOnly(s.ShipDate).Format(rateDateLayout),
			Status:   int8(s.Status),
		})
	}

	s.log.Debug("客户 360", "partner", brief.Code, "orders", len(report.Orders),
		"outstanding", outstanding, "overdue", overdue)
	return report, nil
}

// dateOrEmpty 把可空日期格式化成 YYYY-MM-DD，空值给空串而不是 "0001-01-01"。
func dateOrEmpty(t *time.Time) string {
	if t == nil || t.IsZero() {
		return ""
	}
	return model.DateOnly(*t).Format(rateDateLayout)
}

func partner360Limit(raw *int) (int, error) {
	if raw == nil {
		return partner360DefaultLimit, nil
	}
	if *raw < 0 {
		return 0, errs.BadRequest("limit 不能为负数")
	}
	if *raw == 0 || *raw > partner360MaxLimit {
		return partner360MaxLimit, nil
	}
	return *raw, nil
}
