package service

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/edwinhuish/gowork/internal/config"
	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/infra/cache"
	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/errs"
	"github.com/edwinhuish/gowork/internal/repository"
)

func newUserTestService(t *testing.T) (UserService, repository.UserRepository) {
	t.Helper()
	db := openTestDB(t, &model.User{})
	repo := repository.NewUserRepository(db)
	c, err := cache.New(config.Cache{Driver: config.CacheMemory}, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatalf("建缓存失败: %v", err)
	}
	auth := NewAuthService(repo, c, AuthConfig{
		Secret: "test-secret", Expire: time.Hour, MaxLoginFail: 5,
	})
	return NewUserService(repo, auth), repo
}

func userActorCtx(id uint, role string) context.Context {
	return WithActor(context.Background(), &Principal{UserID: id, Role: role})
}

func hasErrCode(err error, code errs.Code) bool {
	var e *errs.Error
	return errors.As(err, &e) && e.Code == code
}

// TestUserMaintenance 覆盖 T-57 的四条业务规则：
// 用户名唯一、角色白名单、只有管理员能维护、以及"不能把自己锁在门外"。
func TestUserMaintenance(t *testing.T) {
	ctx := userActorCtx(99, model.RoleAdmin)
	svc, repo := newUserTestService(t)

	alice, err := svc.Create(ctx, dto.UserInput{
		Username: "alice", Password: "secret123", Nickname: "爱丽丝", Role: model.RoleStaff,
	})
	if err != nil {
		t.Fatalf("建号失败: %v", err)
	}
	if alice.Password == "secret123" || alice.Password == "" {
		t.Fatal("密码必须存摘要，不能存明文")
	}

	// 同名再建一次：前置查重给 409，唯一索引兜底并发。
	if _, err := svc.Create(ctx, dto.UserInput{
		Username: "alice", Password: "secret123", Role: model.RoleStaff,
	}); !hasErrCode(err, errs.CodeConflict) {
		t.Fatalf("重名应返回 409，得到 %v", err)
	}

	if _, err := svc.Create(ctx, dto.UserInput{
		Username: "bob", Password: "secret123", Role: "root",
	}); !hasErrCode(err, errs.CodeInvalidParam) {
		t.Fatalf("非法角色应返回 400，得到 %v", err)
	}

	// 业务员既不能看全部账号，也不能改账号。
	staffCtx := userActorCtx(alice.ID, model.RoleStaff)
	if _, err := svc.List(staffCtx); !errors.Is(err, errs.ErrForbidden) {
		t.Fatalf("业务员列账号应 403，得到 %v", err)
	}
	if _, err := svc.Update(staffCtx, alice.ID, dto.UserUpdateInput{Role: strPtr(model.RoleAdmin)}); !errors.Is(err, errs.ErrForbidden) {
		t.Fatalf("业务员改角色应 403，得到 %v", err)
	}

	// 提为管理员成功，且昵称不动（指针语义：没传就是不改）。
	if _, err := svc.Update(ctx, alice.ID, dto.UserUpdateInput{Role: strPtr(model.RoleAdmin)}); err != nil {
		t.Fatalf("提权失败: %v", err)
	}
	got, err := repo.FindByID(context.Background(), alice.ID)
	if err != nil {
		t.Fatalf("回查失败: %v", err)
	}
	if got.Role != model.RoleAdmin || got.Nickname != "爱丽丝" {
		t.Errorf("改角色不应动昵称，得到 role=%q nickname=%q", got.Role, got.Nickname)
	}
}

// TestUserCannotLockOut 是这一条里最容易出事的规则：系统没有"恢复出厂账号"的入口，
// 一旦停用或降级最后一个管理员，只能改数据库才能再进系统设置。
func TestUserCannotLockOut(t *testing.T) {
	ctx := userActorCtx(99, model.RoleAdmin)
	svc, _ := newUserTestService(t)

	boss, err := svc.Create(ctx, dto.UserInput{
		Username: "boss", Password: "secret123", Role: model.RoleAdmin,
	})
	if err != nil {
		t.Fatalf("建管理员失败: %v", err)
	}

	off := int8(model.EnableStatusOff)
	// boss 是唯一的可用管理员：停用与降级都要被拦下。
	if _, err := svc.Update(ctx, boss.ID, dto.UserUpdateInput{Status: &off}); !hasErrCode(err, errs.CodeConflict) {
		t.Fatalf("停用最后一个管理员应 409，得到 %v", err)
	}
	if _, err := svc.Update(ctx, boss.ID, dto.UserUpdateInput{Role: strPtr(model.RoleManager)}); !hasErrCode(err, errs.CodeConflict) {
		t.Fatalf("降级最后一个管理员应 409，得到 %v", err)
	}

	// 自己改自己的角色也被拦：这条不是防别人，是防手滑。
	selfCtx := userActorCtx(boss.ID, model.RoleAdmin)
	if _, err := svc.Update(selfCtx, boss.ID, dto.UserUpdateInput{Role: strPtr(model.RoleManager)}); !hasErrCode(err, errs.CodeInvalidParam) {
		t.Fatalf("改自己的角色应被拒绝，得到 %v", err)
	}
	if _, err := svc.Update(selfCtx, boss.ID, dto.UserUpdateInput{Status: &off}); !hasErrCode(err, errs.CodeInvalidParam) {
		t.Fatalf("停用自己应被拒绝，得到 %v", err)
	}

	// 再建一个管理员之后，前一个就可以降级了。
	if _, err := svc.Create(ctx, dto.UserInput{
		Username: "boss2", Password: "secret123", Role: model.RoleAdmin,
	}); err != nil {
		t.Fatalf("建第二个管理员失败: %v", err)
	}
	if _, err := svc.Update(ctx, boss.ID, dto.UserUpdateInput{Role: strPtr(model.RoleManager)}); err != nil {
		t.Fatalf("有第二个管理员时应可降级，得到 %v", err)
	}
}

func strPtr(s string) *string { return &s }
