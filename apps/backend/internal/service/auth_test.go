package service

import (
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/edwinhuish/gowork/internal/config"
	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/infra/cache"
	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/errs"
	"github.com/edwinhuish/gowork/internal/repository"
)

type authFixture struct {
	svc AuthService
	db  *gorm.DB
}

func newAuthFixture(t *testing.T, maxFail int) authFixture {
	t.Helper()
	db := openTestDB(t, &model.User{})
	c, err := cache.New(config.Cache{Driver: config.CacheMemory},
		slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatalf("建缓存失败: %v", err)
	}
	return authFixture{
		db: db,
		svc: NewAuthService(repository.NewUserRepository(db), c, AuthConfig{
			Secret:       "test-secret",
			Expire:       time.Hour,
			MaxLoginFail: maxFail,
		}),
	}
}

func (f authFixture) addUser(t *testing.T, username, password, role string, status model.EnableStatus) *model.User {
	t.Helper()
	hash, err := f.svc.HashPassword(password)
	if err != nil {
		t.Fatalf("哈希密码失败: %v", err)
	}
	u := &model.User{Username: username, Password: hash, Nickname: username, Role: role,
		Status: model.EnableStatusOn}
	if err := f.db.Create(u).Error; err != nil {
		t.Fatalf("建账号失败: %v", err)
	}
	// Status 带 default:1，零值会被 gorm 省略并由库填 1，所以停用要单独写一次。
	if status != model.EnableStatusOn {
		if err := f.db.Model(&model.User{}).Where("id = ?", u.ID).
			Update("status", status).Error; err != nil {
			t.Fatalf("改账号状态失败: %v", err)
		}
	}
	return u
}

// 密码哈希这条路径没有别的入口，写错了没人发现——所以至少钉住"能验回去"与"每次都不同"。
func TestHashPassword(t *testing.T) {
	f := newAuthFixture(t, 5)

	first, err := f.svc.HashPassword("s3cret")
	if err != nil {
		t.Fatalf("哈希失败: %v", err)
	}
	second, err := f.svc.HashPassword("s3cret")
	if err != nil {
		t.Fatalf("哈希失败: %v", err)
	}
	if first == second {
		t.Error("同一密码两次哈希应不同（各自带盐）；相同说明没加盐")
	}
	if !strings.HasPrefix(first, "$2") {
		t.Errorf("应是 bcrypt 摘要，得到 %q", first[:min(6, len(first))])
	}
	if strings.Contains(first, "s3cret") {
		// 反向断言：摘要里不该出现明文。
		t.Error("摘要里出现了明文密码")
	}
}

// 账号不存在与密码错误必须返回**同一句话**，否则等于给了账号枚举的入口。
func TestLoginSameMessageForUnknownUserAndWrongPassword(t *testing.T) {
	f := newAuthFixture(t, 5)
	f.addUser(t, "alice", "right", model.RoleStaff, model.EnableStatusOn)
	ctx := context.Background()

	_, errUnknown := f.svc.Login(ctx, dto.LoginRequest{Username: "nobody", Password: "x"})
	_, errWrong := f.svc.Login(ctx, dto.LoginRequest{Username: "alice", Password: "wrong"})
	wantCode(t, errUnknown, errs.CodeUnauthorized, "账号不存在")
	wantCode(t, errWrong, errs.CodeUnauthorized, "密码错误")

	if errUnknown.Error() != errWrong.Error() {
		t.Errorf("两种失败必须同一文案（防账号枚举）：%q vs %q",
			errUnknown.Error(), errWrong.Error())
	}
}

// 连续失败到阈值后锁定；成功登录会清零失败计数。
func TestLoginLockoutAndReset(t *testing.T) {
	f := newAuthFixture(t, 3)
	f.addUser(t, "alice", "right", model.RoleStaff, model.EnableStatusOn)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		_, err := f.svc.Login(ctx, dto.LoginRequest{Username: "alice", Password: "wrong"})
		wantCode(t, err, errs.CodeUnauthorized, "密码错误")
	}

	// 达到阈值后，即使密码正确也被挡（403，文案说明多久后重试）。
	_, err := f.svc.Login(ctx, dto.LoginRequest{Username: "alice", Password: "right"})
	wantCode(t, err, errs.CodeForbidden, "锁定后即使密码正确")
	if !strings.Contains(err.Error(), "过多") {
		t.Errorf("锁定文案应说明原因，得到 %q", err.Error())
	}

	t.Run("另一个账号不受影响", func(t *testing.T) {
		f.addUser(t, "bob", "right", model.RoleStaff, model.EnableStatusOn)
		if _, err := f.svc.Login(ctx, dto.LoginRequest{Username: "bob", Password: "right"}); err != nil {
			t.Errorf("锁定按账号计数，不该连坐别的账号: %v", err)
		}
	})
}

func TestLoginRejectsDisabledUser(t *testing.T) {
	f := newAuthFixture(t, 5)
	f.addUser(t, "gone", "right", model.RoleStaff, model.EnableStatusOff)

	_, err := f.svc.Login(context.Background(),
		dto.LoginRequest{Username: "gone", Password: "right"})
	// 停用不是"密码错"：文案要能让人找管理员，而不是反复试密码。
	wantCode(t, err, errs.CodeForbidden, "停用账号")
}

// 登录签发的令牌要能验回身份；登出之后同一个令牌立刻失效（会话落缓存的意义）。
func TestLoginVerifyLogout(t *testing.T) {
	f := newAuthFixture(t, 5)
	user := f.addUser(t, "alice", "right", model.RoleManager, model.EnableStatusOn)
	ctx := context.Background()

	res, err := f.svc.Login(ctx, dto.LoginRequest{Username: "alice", Password: "right"})
	if err != nil {
		t.Fatalf("登录失败: %v", err)
	}
	if res.Token == "" || res.ExpiresAt == 0 {
		t.Fatalf("登录应返回令牌与过期时间: %+v", res)
	}
	if res.User == nil || res.User.ID != user.ID || res.User.Role != model.RoleManager {
		t.Errorf("返回的身份不对: %+v", res.User)
	}

	principal, err := f.svc.Verify(ctx, res.Token)
	if err != nil {
		t.Fatalf("验令牌失败: %v", err)
	}
	if principal.UserID != user.ID || principal.Username != "alice" {
		t.Errorf("解出的身份不对: %+v", principal)
	}

	if err := f.svc.Logout(ctx, principal.TokenID); err != nil {
		t.Fatalf("登出失败: %v", err)
	}
	_, err = f.svc.Verify(ctx, res.Token)
	wantCode(t, err, errs.CodeUnauthorized, "登出后令牌应失效")
}

func TestVerifyRejectsBadTokens(t *testing.T) {
	f := newAuthFixture(t, 5)
	f.addUser(t, "alice", "right", model.RoleStaff, model.EnableStatusOn)
	ctx := context.Background()

	t.Run("空令牌", func(t *testing.T) {
		_, err := f.svc.Verify(ctx, "")
		wantCode(t, err, errs.CodeUnauthorized, "空令牌")
	})

	t.Run("签名被改", func(t *testing.T) {
		res, err := f.svc.Login(ctx, dto.LoginRequest{Username: "alice", Password: "right"})
		if err != nil {
			t.Fatalf("登录失败: %v", err)
		}
		tampered := res.Token[:len(res.Token)-2] + "xy"
		_, err = f.svc.Verify(ctx, tampered)
		wantCode(t, err, errs.CodeUnauthorized, "签名被改")
	})
}

func TestLoginClearsFailCountAfterSuccess(t *testing.T) {
	f := newAuthFixture(t, 3)
	f.addUser(t, "alice", "right", model.RoleStaff, model.EnableStatusOn)
	ctx := context.Background()

	// 两次失败后成功：计数应清零，再失败两次也还不该被锁。
	for i := 0; i < 2; i++ {
		if _, err := f.svc.Login(ctx, dto.LoginRequest{Username: "alice", Password: "bad"}); err == nil {
			t.Fatal("错误密码不该登录成功")
		}
	}
	if _, err := f.svc.Login(ctx, dto.LoginRequest{Username: "alice", Password: "right"}); err != nil {
		t.Fatalf("正确密码应当成功: %v", err)
	}
	for i := 0; i < 2; i++ {
		_, _ = f.svc.Login(ctx, dto.LoginRequest{Username: "alice", Password: "bad"})
	}
	if _, err := f.svc.Login(ctx, dto.LoginRequest{Username: "alice", Password: "right"}); err != nil {
		t.Errorf("成功登录后计数应清零，两次失败不该导致锁定: %v", err)
	}
}
