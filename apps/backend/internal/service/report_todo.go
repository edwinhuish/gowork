package service

import (
	"context"
	"strings"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/errs"
)

const (
	todoDefaultLimit = 20
	todoMaxLimit     = 100
)

// Todos 汇总三类"今天该处理"的事（T-34）。
//
// 口径逐条写死，不做模糊判断：
//   - 待跟进：`next_follow_at` 早于今天。到期当天不算——早上还没开始联系就先标红，
//     提醒就成了噪音，人会学会无视它。
//   - 应收待催：到期日 <= 今天且未结清。与账龄同一口径（作废不算、按未核销算），
//     但**到期当天就算**：催款是当天的事，等到明天提醒就晚了一天。
//   - 缺汇率：外币币种今天没有汇率记录。报价与开单都要锁当天汇率，缺了会在保存那一刻炸。
//
// 数据范围跟列表一致：业务员只看自己（与未分配）客户的跟进与应收；
// 汇率属于主数据，只对有全局视野的角色提示。
func (s *reportService) Todos(ctx context.Context, q dto.TodoQuery) (*dto.TodoReport, error) {
	limit, err := todoLimit(q.Limit)
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

	scope := ownerScope(ctx)

	followups, followupCount, err := s.repo.ListTodoFollowups(ctx, asOf, scope, limit)
	if err != nil {
		return nil, err
	}
	receivables, receivableCount, err := s.repo.ListTodoReceivables(ctx, asOf, scope, limit)
	if err != nil {
		return nil, err
	}

	report := &dto.TodoReport{
		AsOf:            asOf.Format(rateDateLayout),
		FollowupCount:   followupCount,
		ReceivableCount: receivableCount,
		Followups:       make([]dto.TodoFollowup, 0, len(followups)),
		Receivables:     make([]dto.TodoReceivable, 0, len(receivables)),
		MissingRates:    []dto.TodoCurrency{},
	}

	for _, f := range followups {
		report.Followups = append(report.Followups, dto.TodoFollowup{
			ID:           f.ID,
			PartnerID:    f.PartnerID,
			PartnerName:  f.PartnerName,
			Subject:      f.Subject,
			NextFollowAt: model.DateOnly(f.NextFollowAt).Format(rateDateLayout),
			OverdueDays:  agingDays(asOf, model.DateOnly(f.NextFollowAt)),
		})
	}
	for _, r := range receivables {
		report.Receivables = append(report.Receivables, dto.TodoReceivable{
			ID:              r.ID,
			DocNo:           r.DocNo,
			PartnerID:       r.PartnerID,
			PartnerName:     r.PartnerName,
			DueDate:         model.DateOnly(r.DueDate).Format(rateDateLayout),
			OutstandingBase: r.OutstandingBase.Round(4),
			OverdueDays:     max(agingDays(asOf, model.DateOnly(r.DueDate)), 0),
		})
	}

	// 汇率维护是主数据的事：业务员既改不了也不该被它打扰。
	if scope == nil {
		rates, err := s.repo.ListMissingRates(ctx, asOf)
		if err != nil {
			return nil, err
		}
		for _, c := range rates {
			report.MissingRates = append(report.MissingRates, dto.TodoCurrency{Code: c.Code, Name: c.Name})
		}
	}

	s.log.Debug("待办清单", "as_of", report.AsOf,
		"followups", followupCount, "receivables", receivableCount,
		"missing_rates", len(report.MissingRates))
	return report, nil
}

func todoLimit(raw *int) (int, error) {
	if raw == nil {
		return todoDefaultLimit, nil
	}
	if *raw < 0 {
		return 0, errs.BadRequest("limit 不能为负数")
	}
	if *raw == 0 || *raw > todoMaxLimit {
		return todoMaxLimit, nil
	}
	return *raw, nil
}
