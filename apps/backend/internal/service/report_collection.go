package service

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/errs"
	"github.com/edwinhuish/gowork/internal/pkg/money"
)

// 超期明细的行数上限。催款清单给到 500 行覆盖得住绝大多数情况，
// 再多就该按客户筛了，而不是把整张表塞进响应体。
const (
	collectionDefaultLimit = 50
	collectionMaxLimit     = 500
)

// Collection 给出期间回款率与截至 as_of 的超期明细（T-29）。
//
// 两件事放在一张表里，但**不能互相校验**：
//   - 回款率是流量：分子是期间内到账的钱（按到账日），分母是期间内到期的单据金额
//     （按到期日）。收的钱可能是在还上个月的账，所以回款率超过 100% 是正常现象。
//   - 超期明细是存量：截至 as_of 仍未核销、且已过到期日的单据，逐笔列出。
//
// 分母用"期间内到期的单据金额"而不是"期间内新增应收"：单据上没有开票日，
// 只有到期日；拿 created_at 充当开票日，会把补录的单据算到录入的那个月。
func (s *reportService) Collection(
	ctx context.Context, q dto.CollectionQuery,
) (*dto.CollectionReport, error) {
	target, err := agingTarget(q.Direction)
	if err != nil {
		return nil, err
	}
	from, to, labelFrom, labelTo, err := parsePeriod(q.From, q.To)
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
	limit, err := collectionLimit(q.Limit)
	if err != nil {
		return nil, err
	}

	docs, err := s.repo.ListOutstandingSettlements(ctx, target)
	if err != nil {
		return nil, err
	}
	due, err := s.repo.SumSettlementDue(ctx, target, from, to)
	if err != nil {
		return nil, err
	}
	received, err := s.repo.SumReceivedAllocations(
		ctx, paymentDirectionOf(target), target, from, to)
	if err != nil {
		return nil, err
	}

	outstanding := money.Zero()
	overdue := money.Zero()
	var buckets dto.AgingBuckets
	rows := make([]dto.CollectionRow, 0, len(docs))

	for _, d := range docs {
		outstandingBase := d.AmountBase.Sub(d.SettledAmountBase).Round(4)
		outstanding = outstanding.Add(outstandingBase)

		days := agingDays(asOf, model.DateOnly(d.DueDate))
		if days <= 0 {
			// 未到期的单据不进超期明细：这张表是催款清单，账龄才是分桶全貌。
			continue
		}

		overdue = overdue.Add(outstandingBase)
		buckets.Add(agingBucket(days), outstandingBase)
		rows = append(rows, dto.CollectionRow{
			ID:              d.ID,
			DocNo:           d.DocNo,
			PartnerID:       d.PartnerID,
			PartnerCode:     d.PartnerCode,
			PartnerName:     d.PartnerName,
			CurrencyCode:    d.CurrencyCode,
			DueDate:         model.DateOnly(d.DueDate).Format(rateDateLayout),
			OverdueDays:     days,
			Outstanding:     d.Amount.Sub(d.SettledAmount).Round(4),
			OutstandingBase: outstandingBase,
		})
	}

	// 逾期越久越先催，同一天到期的按金额大的先催。
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].OverdueDays != rows[j].OverdueDays {
			return rows[i].OverdueDays > rows[j].OverdueDays
		}
		if c := rows[i].OutstandingBase.Cmp(rows[j].OutstandingBase); c != 0 {
			return c > 0
		}
		return rows[i].DocNo < rows[j].DocNo
	})

	truncated := len(rows) > limit
	if truncated {
		rows = rows[:limit]
	}

	rate := money.Zero()
	if due.Cmp(money.Zero()) > 0 {
		rate = received.Div(due).Round(4)
	}

	report := &dto.CollectionReport{
		Direction:       int8(target),
		From:            labelFrom,
		To:              labelTo,
		AsOf:            asOf.Format(rateDateLayout),
		DueAmountBase:   due.Round(4),
		ReceivedBase:    received.Round(4),
		CollectionRate:  rate,
		OutstandingBase: outstanding.Round(4),
		OverdueBase:     overdue.Round(4),
		OverdueBuckets:  buckets,
		OverdueRows:     rows,
		Truncated:       truncated,
	}

	s.log.Debug("回款与超期", "direction", target.Label(), "from", labelFrom, "to", labelTo,
		"rate", report.CollectionRate, "overdue", report.OverdueBase, "rows", len(rows))
	return report, nil
}

// parsePeriod 把两个 YYYY-MM-DD 解析成闭区间，并返回回显用的规范化字符串。
//
// 账龄、回款率、销售看板共用这一份：三处各写一遍"哪个为空表示不限"，
// 迟早出现其中一处把空串当成了 1970-01-01。
func parsePeriod(from, to string) (
	fromPtr, toPtr *time.Time, labelFrom, labelTo string, err error,
) {
	if strings.TrimSpace(from) != "" {
		d, err := parseDateOnly(from)
		if err != nil {
			return nil, nil, "", "", err
		}
		fromPtr = &d
		labelFrom = d.Format(rateDateLayout)
	}
	if strings.TrimSpace(to) != "" {
		d, err := parseDateOnly(to)
		if err != nil {
			return nil, nil, "", "", err
		}
		toPtr = &d
		labelTo = d.Format(rateDateLayout)
	}
	if fromPtr != nil && toPtr != nil && fromPtr.After(*toPtr) {
		return nil, nil, "", "", errs.BadRequest("起始日期不能晚于结束日期")
	}
	return fromPtr, toPtr, labelFrom, labelTo, nil
}

// paymentDirectionOf 把核销对象翻成资金方向：应收对应收款，应付对应付款。
func paymentDirectionOf(target model.SettlementTarget) model.PaymentDirection {
	if target == model.SettlementTargetPayable {
		return model.PaymentDirectionOut
	}
	return model.PaymentDirectionIn
}

func collectionLimit(raw *int) (int, error) {
	if raw == nil {
		return collectionDefaultLimit, nil
	}
	if *raw < 0 {
		return 0, errs.BadRequest("limit 不能为负数")
	}
	if *raw == 0 || *raw > collectionMaxLimit {
		return collectionMaxLimit, nil
	}
	return *raw, nil
}
