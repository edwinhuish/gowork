package service

import (
	"context"
	"testing"
	"time"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/errs"
	"github.com/edwinhuish/gowork/internal/repository"
)

func newActivityTestService(
	t *testing.T, now time.Time,
) (ActivityService, repository.PartnerRepository, repository.ContactRepository) {
	t.Helper()
	db := openTestDB(t, &model.Partner{}, &model.Contact{}, &model.Activity{})
	partners := repository.NewPartnerRepository(db)
	contacts := repository.NewContactRepository(db)
	svc := NewActivityService(
		repository.NewActivityRepository(db), partners, contacts,
		WithActivityClock(func() time.Time { return now }),
	)
	return svc, partners, contacts
}

func TestActivityRules(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 10, 28, 15, 0, 0, 0, time.Local)
	svc, partners, contacts := newActivityTestService(t, now)

	p1 := &model.Partner{
		Code: "C001", Name: "Ferreteria El Sol", Role: model.PartnerRoleCustomer,
		EntityType: model.PartnerEntityCompany, Status: model.EnableStatusOn,
	}
	p2 := &model.Partner{
		Code: "C002", Name: "Wang Hai", Role: model.PartnerRoleCustomer,
		EntityType: model.PartnerEntityPerson, Status: model.EnableStatusOn,
	}
	for _, p := range []*model.Partner{p1, p2} {
		if err := partners.Create(ctx, p); err != nil {
			t.Fatalf("建主体失败: %v", err)
		}
	}
	c1 := &model.Contact{PartnerID: p1.ID, Name: "Maria", Status: model.EnableStatusOn}
	if err := contacts.Create(ctx, c1); err != nil {
		t.Fatalf("建联系人失败: %v", err)
	}

	created, err := svc.Create(ctx, dto.ActivityInput{
		PartnerID: p1.ID, ContactID: &c1.ID, Type: "call",
		Subject: "电话确认包装", OccurredAt: "2026-10-27", NextFollowAt: "2026-10-29",
	})
	if err != nil {
		t.Fatalf("新建跟进失败: %v", err)
	}
	// 小写入参被归一化，否则列表按 type 过滤会同时存在 CALL 与 call。
	if created.Type != model.ActivityTypeCall {
		t.Errorf("type = %s，期望 CALL", created.Type)
	}

	// 已到期：下次跟进日在今天之前。
	if _, err := svc.Create(ctx, dto.ActivityInput{
		PartnerID: p1.ID, Type: "EMAIL", Subject: "报价已发",
		OccurredAt: "2026-10-20", NextFollowAt: "2026-10-25",
	}); err != nil {
		t.Fatalf("新建跟进失败: %v", err)
	}

	due := int8(1)
	overdue, err := svc.List(ctx, dto.ActivityQuery{Due: &due})
	if err != nil {
		t.Fatalf("查已到期失败: %v", err)
	}
	if len(overdue.List) != 1 || overdue.List[0].Subject != "报价已发" {
		t.Fatalf("已到期应为 1 条（报价已发），实际 %d 条", len(overdue.List))
	}

	// 今天到期的还不算超期：判定按整天，不按查询时刻。
	if _, err := svc.Create(ctx, dto.ActivityInput{
		PartnerID: p1.ID, Type: "VISIT", Subject: "今天到期",
		OccurredAt: "2026-10-28", NextFollowAt: "2026-10-28",
	}); err != nil {
		t.Fatalf("新建跟进失败: %v", err)
	}
	overdue, err = svc.List(ctx, dto.ActivityQuery{Due: &due})
	if err != nil {
		t.Fatalf("查已到期失败: %v", err)
	}
	if len(overdue.List) != 1 {
		t.Fatalf("今天到期不应算超期，实际 %d 条", len(overdue.List))
	}

	// 联系人必须属于所选主体，否则这条跟进会同时挂在两家客户名下。
	_, err = svc.Create(ctx, dto.ActivityInput{
		PartnerID: p2.ID, ContactID: &c1.ID, Type: "CALL",
		Subject: "串到别家", OccurredAt: "2026-10-27",
	})
	if e, ok := errs.From(err); !ok || e.Code != errs.CodeInvalidParam {
		t.Fatalf("联系人串主体应报 40001，实际 %v", err)
	}

	_, err = svc.Create(ctx, dto.ActivityInput{
		PartnerID: p1.ID, Type: "CALL", Subject: "未来", OccurredAt: "2026-10-30",
	})
	if e, ok := errs.From(err); !ok || e.Code != errs.CodeInvalidParam {
		t.Fatalf("未来日期应报 40001，实际 %v", err)
	}

	_, err = svc.Create(ctx, dto.ActivityInput{
		PartnerID: p1.ID, Type: "CALL", Subject: "顺序反了",
		OccurredAt: "2026-10-27", NextFollowAt: "2026-10-26",
	})
	if e, ok := errs.From(err); !ok || e.Code != errs.CodeInvalidParam {
		t.Fatalf("下次跟进早于本次应报 40001，实际 %v", err)
	}

	// 时间线按发生时间倒序。
	all, err := svc.List(ctx, dto.ActivityQuery{PartnerID: &p1.ID})
	if err != nil {
		t.Fatalf("查列表失败: %v", err)
	}
	if len(all.List) < 3 {
		t.Fatalf("应有 3 条跟进，实际 %d 条", len(all.List))
	}
	for i := 1; i < len(all.List); i++ {
		if all.List[i].OccurredAt.After(all.List[i-1].OccurredAt) {
			t.Fatalf("时间线未按 occurred_at 倒序：第 %d 条晚于第 %d 条", i, i-1)
		}
	}
}
