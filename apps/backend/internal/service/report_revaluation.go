package service

import (
	"context"
	"strings"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/errs"
	"github.com/edwinhuish/gowork/internal/pkg/money"
)

// Revaluation 出期末重估：把未核销的外币余额按指定日汇率重算，差额单列（T-60）。
//
// 与已实现的汇兑损益（`/reports/exchange-gains`）分工：
//   - 汇兑损益只覆盖**已核销**的部分——钱真的到账了，差额已经实现；
//   - 重估覆盖**还没核销**的部分——账面仍按开单时的历史汇率挂着，
//     按今天的汇率看，这笔应收（或应付）已经不值账面上那个数了。
//
// 它是报表不是凭证：不落库、不改单据。要不要做调整分录是会计的事，
// 本系统没有总账，硬写一张"调整单"只会让应收与账面对不上。
func (s *reportService) Revaluation(
	ctx context.Context, q dto.RevaluationQuery,
) (*dto.RevaluationReport, error) {
	if s.currencies == nil {
		return nil, errs.New(errs.CodeInternal, "期末重估未接汇率服务，请检查 bootstrap")
	}
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

	baseCode, err := s.currencies.BaseCode(ctx)
	if err != nil {
		return nil, err
	}
	baseDP, err := s.currencies.Decimals(ctx, baseCode)
	if err != nil {
		return nil, err
	}

	// 直接复用账龄那份未结清数据：口径只有一份（amount − settled − credited），
	// 重估自己再写一套 SQL，迟早出现"账龄说欠 1000、重估说欠 900"。
	items, err := s.repo.ListOutstandingSettlements(ctx, target)
	if err != nil {
		return nil, err
	}

	report := &dto.RevaluationReport{
		AsOf:      asOf.Format(rateDateLayout),
		BaseCode:  baseCode,
		Direction: int8(target),
		Rows:      make([]dto.RevaluationRow, 0, len(items)),
	}

	// 汇率按币种缓存：一票一张单，同币种的单可能几十张，
	// 每张都查一次汇率表既慢又容易读到不一致的值（中间有人补录了汇率）。
	rates := make(map[string]money.Decimal, 4)
	for _, it := range items {
		if it.CurrencyCode == "" || it.CurrencyCode == baseCode {
			// 本币不需要重估：它本来就是记账本位币。
			continue
		}
		outstanding := it.Amount.Sub(it.SettledAmount).Sub(it.Credited)
		if !outstanding.IsPositive() {
			continue
		}
		rate, ok := rates[it.CurrencyCode]
		if !ok {
			_, r, err := s.currencies.ToBaseDecimalFor(
				ctx, money.FromInt(1), it.CurrencyCode, asOf, model.RateTypeMiddle)
			if err != nil {
				return nil, err
			}
			rate = r
			rates[it.CurrencyCode] = rate
		}
		carrying := it.AmountBase.Sub(it.SettledAmountBase).Sub(it.CreditedBase)
		revalued := outstanding.Mul(rate).Round(baseDP)

		report.Rows = append(report.Rows, dto.RevaluationRow{
			DocNo:        it.DocNo,
			PartnerName:  it.PartnerName,
			CurrencyCode: it.CurrencyCode,
			Rate:         rate,
			Outstanding:  outstanding.Round(4),
			CarryingBase: carrying.Round(4),
			RevaluedBase: revalued,
			Difference:   revalued.Sub(carrying).Round(4),
		})
		report.TotalDifference = report.TotalDifference.Add(revalued.Sub(carrying))
	}
	report.TotalDifference = report.TotalDifference.Round(4)
	report.RowCount = int64(len(report.Rows))
	return report, nil
}
