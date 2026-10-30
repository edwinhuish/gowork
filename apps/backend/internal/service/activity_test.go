package service

import (
	"context"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/errs"
	"github.com/edwinhuish/gowork/internal/repository"
)

type activityFixture struct {
	svc ActivityService
	db  *gorm.DB
	now time.Time
}

func newActivityFixture(t *testing.T, now time.Time) activityFixture {
	t.Helper()
	db := openTestDB(t, &model.Activity{}, &model.Partner{}, &model.Contact{})
	return activityFixture{
		db:  db,
		now: now,
		svc: NewActivityService(
			repository.NewActivityRepository(db),
			repository.NewPartnerRepository(db),
			repository.NewContactRepository(db),
			WithActivityClock(func() time.Time { return now })),
	}
}

func (f activityFixture) addPartner(t *testing.T, code string) uint {
	t.Helper()
	p := &model.Partner{
		Code: code, Name: code, Role: model.PartnerRoleCustomer,
		EntityType: model.PartnerEntityCompany,
	}
	if err := f.db.Create(p).Error; err != nil {
		t.Fatalf("建主体失败: %v", err)
	}
	return p.ID
}

func (f activityFixture) addContact(t *testing.T, partnerID uint, name string) uint {
	t.Helper()
	c := &model.Contact{PartnerID: partnerID, Name: name}
	if err := f.db.Create(c).Error; err != nil {
		t.Fatalf("建联系人失败: %v", err)
	}
	return c.ID
}

// 跟进的三条硬约束：类型合法、不能记到未来、下次跟进不能早于本次。
func TestActivityCreateValidates(t *testing.T) {
	now := time.Date(2026, 10, 28, 10, 0, 0, 0, time.Local)
	f := newActivityFixture(t, now)
	ctx := context.Background()
	partnerID := f.addPartner(t, "CUS001")

	base := dto.ActivityInput{
		PartnerID: partnerID, Type: "call", Subject: "电话沟通", OccurredAt: "2026-10-27",
	}

	t.Run("类型必须合法", func(t *testing.T) {
		in := base
		in.Type = "wechat"
		_, err := f.svc.Create(ctx, in)
		wantCode(t, err, errs.CodeInvalidParam, "未知类型")
	})

	t.Run("不能记到未来", func(t *testing.T) {
		in := base
		in.OccurredAt = "2026-10-29" // now 是 10-28
		_, err := f.svc.Create(ctx, in)
		wantCode(t, err, errs.CodeInvalidParam, "未来日期")
	})

	t.Run("下次跟进不能早于本次", func(t *testing.T) {
		in := base
		in.NextFollowAt = "2026-10-20"
		_, err := f.svc.Create(ctx, in)
		wantCode(t, err, errs.CodeInvalidParam, "下次跟进早于本次")
	})

	t.Run("当天可以记", func(t *testing.T) {
		in := base
		in.OccurredAt = "2026-10-28"
		in.NextFollowAt = "2026-10-28"
		if _, err := f.svc.Create(ctx, in); err != nil {
			t.Errorf("当天记跟进、当天再跟，应当允许: %v", err)
		}
	})
}

// 联系人不属于这家主体时，这条跟进会同时挂在两家名下——历史里各多一条不属于自己的记录。
func TestActivityContactMustBelongToPartner(t *testing.T) {
	now := time.Date(2026, 10, 28, 10, 0, 0, 0, time.Local)
	f := newActivityFixture(t, now)
	ctx := context.Background()
	a := f.addPartner(t, "CUS001")
	b := f.addPartner(t, "CUS002")
	contactOfB := f.addContact(t, b, "别家的人")

	_, err := f.svc.Create(ctx, dto.ActivityInput{
		PartnerID: a, ContactID: &contactOfB, Type: "call", Subject: "串了", OccurredAt: "2026-10-27",
	})
	wantCode(t, err, errs.CodeInvalidParam, "联系人归属")
}

// 改动可以，但主体不能换：换主体等于把一段历史嫁接给另一家客户。
func TestActivityUpdateKeepsPartner(t *testing.T) {
	now := time.Date(2026, 10, 28, 10, 0, 0, 0, time.Local)
	f := newActivityFixture(t, now)
	ctx := context.Background()
	a := f.addPartner(t, "CUS001")
	b := f.addPartner(t, "CUS002")

	m, err := f.svc.Create(ctx, dto.ActivityInput{
		PartnerID: a, Type: "call", Subject: "初次沟通", OccurredAt: "2026-10-27",
	})
	if err != nil {
		t.Fatalf("建跟进失败: %v", err)
	}

	_, err = f.svc.Update(ctx, m.ID, dto.ActivityInput{
		PartnerID: b, Type: "call", Subject: "挪走", OccurredAt: "2026-10-27",
	})
	wantCode(t, err, errs.CodeInvalidParam, "转移主体")

	updated, err := f.svc.Update(ctx, m.ID, dto.ActivityInput{
		PartnerID: a, Type: "visit", Subject: "改成拜访", OccurredAt: "2026-10-27",
		NextFollowAt: "2026-11-05",
	})
	if err != nil {
		t.Fatalf("更新失败: %v", err)
	}
	if updated.Subject != "改成拜访" || updated.Type != model.ActivityTypeVisit {
		t.Errorf("字段没更新: %+v", updated)
	}
	if updated.NextFollowAt == nil ||
		model.DateOnly(*updated.NextFollowAt).Format("2006-01-02") != "2026-11-05" {
		t.Errorf("下次跟进应被写入: %+v", updated.NextFollowAt)
	}
}

func TestActivityDeleteAndGet(t *testing.T) {
	now := time.Date(2026, 10, 28, 10, 0, 0, 0, time.Local)
	f := newActivityFixture(t, now)
	ctx := context.Background()
	partnerID := f.addPartner(t, "CUS001")

	m, err := f.svc.Create(ctx, dto.ActivityInput{
		PartnerID: partnerID, Type: "email", Subject: "发报价", OccurredAt: "2026-10-27",
	})
	if err != nil {
		t.Fatalf("建跟进失败: %v", err)
	}
	if got, err := f.svc.Get(ctx, m.ID); err != nil || got.ID != m.ID {
		t.Fatalf("详情不对: %+v / %v", got, err)
	}

	if err := f.svc.Delete(ctx, m.ID); err != nil {
		t.Fatalf("删除失败: %v", err)
	}
	_, err = f.svc.Get(ctx, m.ID)
	wantCode(t, err, errs.CodeNotFound, "删掉之后查不到")

	if err := f.svc.Delete(ctx, m.ID); err == nil {
		t.Error("重复删除应当报找不到，而不是静默成功")
	}
}
