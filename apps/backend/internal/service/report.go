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
		GrandTotalBase: money.Round(totalBase.Total(), 4),
	}

	s.log.Debug("账龄报表", "direction", target.Label(), "as_of", report.AsOf,
		"rows", len(rows), "total_base", report.GrandTotalBase)
	return report, nil
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
		row.Amount.Add(bucket, money.Round(d.Amount-d.SettledAmount, 4))
		row.AmountBase.Add(bucket, money.Round(d.AmountBase-d.SettledAmountBase, 4))

		if cur, ok := earliest[key]; !ok || due.Before(cur) {
			earliest[key] = due
		}
	}

	rows := make([]dto.AgingRow, 0, len(acc))
	for key, row := range acc {
		row.Total = money.Round(row.Amount.Total(), 4)
		row.TotalBase = money.Round(row.AmountBase.Total(), 4)
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
		total.NotDue += row.AmountBase.NotDue
		total.Days1To30 += row.AmountBase.Days1To30
		total.Days31To60 += row.AmountBase.Days31To60
		total.Days61To90 += row.AmountBase.Days61To90
		total.Over90 += row.AmountBase.Over90
	}
	return dto.AgingBuckets{
		NotDue:     money.Round(total.NotDue, 4),
		Days1To30:  money.Round(total.Days1To30, 4),
		Days31To60: money.Round(total.Days31To60, 4),
		Days61To90: money.Round(total.Days61To90, 4),
		Over90:     money.Round(total.Over90, 4),
	}
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
