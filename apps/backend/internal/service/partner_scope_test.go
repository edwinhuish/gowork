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

type partnerScopeFixture struct {
	svc PartnerService
	db  *gorm.DB
}

func newPartnerScopeFixture(t *testing.T) partnerScopeFixture {
	t.Helper()
	db := openTestDB(t, &model.Partner{}, &model.Product{}, &model.Currency{}, &model.User{})
	return partnerScopeFixture{
		svc: NewPartnerService(
			repository.NewPartnerRepository(db),
			repository.NewProductRepository(db),
			repository.NewCurrencyRepository(db),
			repository.NewUserRepository(db),
		),
		db: db,
	}
}

func (f partnerScopeFixture) addUser(t *testing.T, username, role string, status model.EnableStatus) uint {
	t.Helper()
	u := &model.User{Username: username, Password: "x", Role: role, Status: status}
	if err := f.db.Create(u).Error; err != nil {
		t.Fatalf("建账号失败: %v", err)
	}
	// Status 带 `default:1`：零值会被 gorm 省略、由数据库填 1，
	// "显式停用"因此被静默存成启用。建完再单独写一次才可靠。
	if status != model.EnableStatusOn {
		if err := f.db.Model(&model.User{}).Where("id = ?", u.ID).
			Update("status", status).Error; err != nil {
			t.Fatalf("改账号状态失败: %v", err)
		}
	}
	return u.ID
}

func (f partnerScopeFixture) addPartner(t *testing.T, code string, ownerID uint) uint {
	t.Helper()
	role := int8(model.PartnerRoleCustomer)
	ctx := context.Background()
	if ownerID != 0 {
		// 走 Create 会把 owner 设成当前身份，所以直接写库再补 owner。
		p, err := f.svc.Create(ctx, dto.PartnerInput{Code: code, Name: code, Role: &role})
		if err != nil {
			t.Fatalf("建主体失败: %v", err)
		}
		if err := f.db.Model(&model.Partner{}).Where("id = ?", p.ID).
			Update("owner_id", ownerID).Error; err != nil {
			t.Fatalf("写负责人失败: %v", err)
		}
		return p.ID
	}
	p, err := f.svc.Create(ctx, dto.PartnerInput{Code: code, Name: code, Role: &role})
	if err != nil {
		t.Fatalf("建主体失败: %v", err)
	}
	return p.ID
}

func scopeActor(userID uint, role string) *Principal {
	return &Principal{UserID: userID, Username: "u" + string(rune('0'+userID)), Role: role}
}

// TestPartnerListScope 是 T-32 的验收点：业务员只看自己负责的与未分配的，
// 主管与管理员看全部。
func TestPartnerListScope(t *testing.T) {
	f := newPartnerScopeFixture(t)
	alice := f.addUser(t, "alice", model.RoleStaff, model.EnableStatusOn)
	bob := f.addUser(t, "bob", model.RoleStaff, model.EnableStatusOn)

	mine := f.addPartner(t, "CUS001", alice)
	others := f.addPartner(t, "CUS002", bob)
	public := f.addPartner(t, "CUS003", 0)

	codes := func(ctx context.Context) []string {
		res, err := f.svc.List(ctx, dto.PartnerQuery{})
		if err != nil {
			t.Fatalf("查列表失败: %v", err)
		}
		out := make([]string, 0, len(res.List))
		for _, p := range res.List {
			out = append(out, p.Code)
		}
		return out
	}

	ctxAlice := WithActor(context.Background(), scopeActor(alice, model.RoleStaff))
	got := codes(ctxAlice)
	if len(got) != 2 {
		t.Fatalf("业务员应看到自己的 + 未分配的共 2 条，得到 %v", got)
	}
	for _, code := range got {
		if code == "CUS002" {
			t.Errorf("业务员不该看到别人的客户: %v", got)
		}
	}

	// 业务员传别人的 owner_id 也没用：范围由身份决定。
	other := bob
	res, err := f.svc.List(ctxAlice, dto.PartnerQuery{OwnerID: &other})
	if err != nil {
		t.Fatalf("查列表失败: %v", err)
	}
	for _, p := range res.List {
		if p.Code == "CUS002" {
			t.Errorf("按参数越权拿到了别人的客户: %v", p.Code)
		}
	}

	for _, role := range []string{model.RoleManager, model.RoleAdmin} {
		ctx := WithActor(context.Background(), scopeActor(bob, role))
		if n := len(codes(ctx)); n != 3 {
			t.Errorf("%s 应看到全部 3 条，得到 %d", role, n)
		}
	}

	// 认不出的角色按"只看自己的"处理：拼错一个字母不等于提权。
	ctxTypo := WithActor(context.Background(), scopeActor(alice, "staf"))
	if n := len(codes(ctxTypo)); n != 2 {
		t.Errorf("未知角色应按业务员处理（2 条），得到 %d", n)
	}

	_ = mine
	_ = others
	_ = public
}

func TestPartnerGetForbiddenAcrossOwners(t *testing.T) {
	f := newPartnerScopeFixture(t)
	alice := f.addUser(t, "alice", model.RoleStaff, model.EnableStatusOn)
	bob := f.addUser(t, "bob", model.RoleStaff, model.EnableStatusOn)

	mine := f.addPartner(t, "CUS001", alice)
	others := f.addPartner(t, "CUS002", bob)
	public := f.addPartner(t, "CUS003", 0)

	ctx := WithActor(context.Background(), scopeActor(alice, model.RoleStaff))

	if _, err := f.svc.Get(ctx, mine); err != nil {
		t.Fatalf("自己的客户应当看得到: %v", err)
	}
	if _, err := f.svc.Get(ctx, public); err != nil {
		t.Fatalf("未分配的客户对所有人可见: %v", err)
	}
	// 越权要 403 而不是 404：记录确实存在，只是不归你看。
	_, err := f.svc.Get(ctx, others)
	wantCode(t, err, errs.CodeForbidden, "看别人的客户")

	// 主管不受限制。
	ctxBoss := WithActor(context.Background(), scopeActor(bob, model.RoleManager))
	if _, err := f.svc.Get(ctxBoss, others); err != nil {
		t.Fatalf("主管应能看任何客户: %v", err)
	}
}

func TestPartnerCreateRecordsOwner(t *testing.T) {
	f := newPartnerScopeFixture(t)
	alice := f.addUser(t, "alice", model.RoleStaff, model.EnableStatusOn)

	role := int8(model.PartnerRoleCustomer)
	ctx := WithActor(context.Background(), scopeActor(alice, model.RoleStaff))
	p, err := f.svc.Create(ctx, dto.PartnerInput{Code: "CUS001", Name: "甲", Role: &role})
	if err != nil {
		t.Fatalf("建主体失败: %v", err)
	}
	if p.OwnerID != alice {
		t.Errorf("谁建的该归谁：期望 owner=%d，得到 %d", alice, p.OwnerID)
	}
}

func TestPartnerTransferOwner(t *testing.T) {
	f := newPartnerScopeFixture(t)
	alice := f.addUser(t, "alice", model.RoleStaff, model.EnableStatusOn)
	bob := f.addUser(t, "bob", model.RoleStaff, model.EnableStatusOn)
	boss := f.addUser(t, "boss", model.RoleManager, model.EnableStatusOn)
	off := f.addUser(t, "gone", model.RoleStaff, model.EnableStatusOff)
	target := f.addPartner(t, "CUS001", alice)

	bossCtx := WithActor(context.Background(), scopeActor(boss, model.RoleManager))

	t.Run("业务员不能自己转移", func(t *testing.T) {
		ctx := WithActor(context.Background(), scopeActor(alice, model.RoleStaff))
		_, err := f.svc.TransferOwner(ctx, target, dto.PartnerOwnerInput{OwnerID: bob})
		wantCode(t, err, errs.CodeForbidden, "业务员转移")
	})

	t.Run("没有身份一律拒绝", func(t *testing.T) {
		_, err := f.svc.TransferOwner(context.Background(), target, dto.PartnerOwnerInput{OwnerID: bob})
		wantCode(t, err, errs.CodeUnauthorized, "无身份转移")
	})

	t.Run("接手的账号必须存在且可用", func(t *testing.T) {
		_, err := f.svc.TransferOwner(bossCtx, target, dto.PartnerOwnerInput{OwnerID: 9999})
		wantCode(t, err, errs.CodeInvalidParam, "账号不存在")

		// 转给停用账号等于把这家的单据交给一个看不到的人。
		_, err = f.svc.TransferOwner(bossCtx, target, dto.PartnerOwnerInput{OwnerID: off})
		wantCode(t, err, errs.CodeInvalidParam, "账号已停用")
	})

	t.Run("主管转移成功，退回未分配也行", func(t *testing.T) {
		got, err := f.svc.TransferOwner(bossCtx, target, dto.PartnerOwnerInput{OwnerID: bob})
		if err != nil {
			t.Fatalf("转移失败: %v", err)
		}
		if got.OwnerID != bob {
			t.Errorf("期望 owner=%d，得到 %d", bob, got.OwnerID)
		}

		got, err = f.svc.TransferOwner(bossCtx, target, dto.PartnerOwnerInput{OwnerID: 0})
		if err != nil {
			t.Fatalf("退回未分配失败: %v", err)
		}
		if got.OwnerID != 0 {
			t.Errorf("期望退回 0（公共池），得到 %d", got.OwnerID)
		}
	})

	t.Run("过期版本被拒", func(t *testing.T) {
		p, err := f.svc.Get(bossCtx, target)
		if err != nil {
			t.Fatalf("读主体失败: %v", err)
		}
		stale := p.UpdatedAt
		if _, err := f.svc.TransferOwner(bossCtx, target, dto.PartnerOwnerInput{OwnerID: bob}); err != nil {
			t.Fatalf("第一次转移失败: %v", err)
		}
		_, err = f.svc.TransferOwner(bossCtx, target,
			dto.PartnerOwnerInput{OwnerID: alice, UpdatedAt: &stale})
		wantCode(t, err, errs.CodeConflict, "用过期的版本转移")
	})
}

// 列表要带上负责人姓名：只给 id 的话，界面上那一列是个数字。
func TestPartnerListProjectsOwnerName(t *testing.T) {
	f := newPartnerScopeFixture(t)
	alice := f.addUser(t, "alice", model.RoleStaff, model.EnableStatusOn)
	f.addPartner(t, "CUS001", alice)

	boss := f.addUser(t, "boss", model.RoleManager, model.EnableStatusOn)
	ctx := WithActor(context.Background(), scopeActor(boss, model.RoleManager))
	res, err := f.svc.List(ctx, dto.PartnerQuery{})
	if err != nil {
		t.Fatalf("查列表失败: %v", err)
	}
	if len(res.List) != 1 || res.List[0].OwnerName != "alice" {
		t.Fatalf("应投影出负责人登录名，得到 %+v", res.List)
	}
}
