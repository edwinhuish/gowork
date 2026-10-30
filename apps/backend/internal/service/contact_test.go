package service

import (
	"context"
	"testing"

	"gorm.io/gorm"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/errs"
	"github.com/edwinhuish/gowork/internal/repository"
)

type contactFixture struct {
	svc ContactService
	db  *gorm.DB
}

func newContactFixture(t *testing.T) contactFixture {
	t.Helper()
	db := openTestDB(t, &model.Contact{}, &model.Partner{})
	return contactFixture{
		db: db,
		svc: NewContactService(
			repository.NewContactRepository(db), repository.NewPartnerRepository(db)),
	}
}

func (f contactFixture) addPartner(t *testing.T, code string) uint {
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

func (f contactFixture) primaries(t *testing.T, partnerID uint) []string {
	t.Helper()
	var names []string
	if err := f.db.Model(&model.Contact{}).
		Where("partner_id = ? AND is_primary = ?", partnerID, true).
		Pluck("name", &names).Error; err != nil {
		t.Fatalf("查主联系人失败: %v", err)
	}
	return names
}

// 联系人不挂主体就是一张通讯录，所以主体不存在要直接拒。
func TestContactCreateRequiresPartner(t *testing.T) {
	f := newContactFixture(t)
	_, err := f.svc.Create(context.Background(), dto.ContactInput{
		PartnerID: 9999, Name: "张三",
	})
	wantCode(t, err, errs.CodeInvalidParam, "主体不存在")
}

// 同一主体只能有一个主联系人：设新的就把旧的降级，且是同一个事务里做的。
func TestContactSetPrimaryDemotesOthers(t *testing.T) {
	f := newContactFixture(t)
	ctx := context.Background()
	partnerID := f.addPartner(t, "CUS001")
	other := f.addPartner(t, "CUS002")

	yes := true
	first, err := f.svc.Create(ctx, dto.ContactInput{
		PartnerID: partnerID, Name: "甲", IsPrimary: &yes,
	})
	if err != nil {
		t.Fatalf("建联系人失败: %v", err)
	}
	// 别家的主联系人不该被动到。
	if _, err := f.svc.Create(ctx, dto.ContactInput{
		PartnerID: other, Name: "别家甲", IsPrimary: &yes,
	}); err != nil {
		t.Fatalf("建别家联系人失败: %v", err)
	}

	second, err := f.svc.Create(ctx, dto.ContactInput{
		PartnerID: partnerID, Name: "乙", IsPrimary: &yes,
	})
	if err != nil {
		t.Fatalf("建第二个联系人失败: %v", err)
	}

	if got := f.primaries(t, partnerID); len(got) != 1 || got[0] != "乙" {
		t.Fatalf("同一主体应只剩「乙」是主联系人，得到 %v", got)
	}
	if got := f.primaries(t, other); len(got) != 1 {
		t.Errorf("清主联系人要限定在同一主体内，别家被动了：%v", got)
	}

	t.Run("主联系人不能被摘掉", func(t *testing.T) {
		no := false
		_, err := f.svc.Update(ctx, second.ID, dto.ContactInput{
			PartnerID: partnerID, Name: "乙", IsPrimary: &no,
		})
		// 与币种的本币同一规则：只能换人，不能取消。
		wantCode(t, err, errs.CodeConflict, "取消主联系人")
		if got := f.primaries(t, partnerID); len(got) != 1 {
			t.Errorf("拒绝之后状态不该变，得到 %v", got)
		}
	})

	t.Run("把别人设为主会降级原来那个", func(t *testing.T) {
		if _, err := f.svc.Update(ctx, first.ID, dto.ContactInput{
			PartnerID: partnerID, Name: "甲", IsPrimary: &yes,
		}); err != nil {
			t.Fatalf("换主联系人失败: %v", err)
		}
		if got := f.primaries(t, partnerID); len(got) != 1 || got[0] != "甲" {
			t.Errorf("应只剩「甲」是主联系人，得到 %v", got)
		}
	})
}

// 联系人不能挪主体：历史跟进挂在他身上，挪走就等于把跟进挂到别人名下。
func TestContactCannotMovePartner(t *testing.T) {
	f := newContactFixture(t)
	ctx := context.Background()
	a := f.addPartner(t, "CUS001")
	b := f.addPartner(t, "CUS002")

	c, err := f.svc.Create(ctx, dto.ContactInput{PartnerID: a, Name: "李四"})
	if err != nil {
		t.Fatalf("建联系人失败: %v", err)
	}
	_, err = f.svc.Update(ctx, c.ID, dto.ContactInput{PartnerID: b, Name: "李四"})
	wantCode(t, err, errs.CodeInvalidParam, "转移主体")
}

// 删掉主联系人不自动改选：剩下谁是对接人是业务判断。
func TestContactDeleteLeavesNoImplicitPrimary(t *testing.T) {
	f := newContactFixture(t)
	ctx := context.Background()
	partnerID := f.addPartner(t, "CUS001")

	yes := true
	primary, err := f.svc.Create(ctx, dto.ContactInput{
		PartnerID: partnerID, Name: "甲", IsPrimary: &yes,
	})
	if err != nil {
		t.Fatalf("建联系人失败: %v", err)
	}
	if _, err := f.svc.Create(ctx, dto.ContactInput{PartnerID: partnerID, Name: "乙"}); err != nil {
		t.Fatalf("建联系人失败: %v", err)
	}

	if err := f.svc.Delete(ctx, primary.ID); err != nil {
		t.Fatalf("删除联系人失败: %v", err)
	}
	if got := f.primaries(t, partnerID); len(got) != 0 {
		t.Errorf("不该自动把「乙」提为主联系人，得到 %v", got)
	}
}
