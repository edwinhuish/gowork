package service

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/edwinhuish/gowork/internal/config"
	"github.com/edwinhuish/gowork/internal/infra/cache"
	"github.com/edwinhuish/gowork/internal/pkg/errs"
)

const (
	testSecret = "test-secret-at-least-32-chars-long-abcdef"
	testExpire = time.Hour
)

// newTestAuth 构造一个只依赖内存缓存的鉴权服务，并返回缓存实例用于摆放会话。
func newTestAuth(t *testing.T) (AuthService, cache.Cache) {
	t.Helper()

	discard := slog.New(slog.NewTextHandler(io.Discard, nil))
	sessions, err := cache.New(config.Cache{Driver: config.CacheMemory}, discard)
	if err != nil {
		t.Fatalf("创建内存缓存: %v", err)
	}
	t.Cleanup(func() { _ = sessions.Close() })

	return NewAuthService(nil, sessions, AuthConfig{Secret: testSecret, Expire: testExpire},
		WithAuthLogger(discard)), sessions
}

// signTestToken 按指定剩余有效期签发令牌，ttl 传负值即已过期。
func signTestToken(t *testing.T, tokenID string, ttl time.Duration) string {
	t.Helper()

	now := time.Now()
	claims := tokenClaims{
		UserID:   7,
		Username: "edwin",
		Nickname: "Edwin",
		Role:     "admin",
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        tokenID,
			Subject:   "7",
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(testSecret))
	if err != nil {
		t.Fatalf("签发测试令牌: %v", err)
	}
	return signed
}

func TestVerifyNoRenewWhenFresh(t *testing.T) {
	auth, sessions := newTestAuth(t)
	ctx := context.Background()

	// 剩余 1h，超过 Expire/2 的阈值，不该续期。
	token := signTestToken(t, "tok-fresh", testExpire)
	if err := sessions.Set(ctx, sessionKeyPrefix+"tok-fresh", "7", testExpire); err != nil {
		t.Fatalf("写入会话: %v", err)
	}

	p, err := auth.Verify(ctx, token)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if p.RenewedToken != "" {
		t.Fatalf("剩余有效期充足却触发了续期")
	}
	if p.TokenID != "tok-fresh" {
		t.Fatalf("TokenID = %q, 期望 tok-fresh", p.TokenID)
	}
}

func TestVerifyRenewsWhenNearlyExpired(t *testing.T) {
	auth, sessions := newTestAuth(t)
	ctx := context.Background()

	// 只剩 5 分钟，低于 Expire/2，应续期。
	token := signTestToken(t, "tok-renew", 5*time.Minute)
	if err := sessions.Set(ctx, sessionKeyPrefix+"tok-renew", "7", 5*time.Minute); err != nil {
		t.Fatalf("写入会话: %v", err)
	}

	p, err := auth.Verify(ctx, token)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if p.RenewedToken == "" {
		t.Fatal("临近过期却未续期")
	}
	if p.TokenID != "tok-renew" {
		t.Fatalf("续期后 TokenID 应保持不变，实际 = %q", p.TokenID)
	}
	// 续期后的有效期应接近完整的 testExpire，而不是原令牌剩余的 5 分钟。
	if p.ExpiresAt.Before(time.Now().Add(testExpire - time.Minute)) {
		t.Fatalf("续期后有效期只到 %v，不足 %v", p.ExpiresAt, testExpire)
	}

	// 新令牌必须自洽：能通过校验、jti 不变、不再触发第二次续期。
	p2, err := auth.Verify(ctx, p.RenewedToken)
	if err != nil {
		t.Fatalf("用续期后的令牌再校验失败: %v", err)
	}
	if p2.TokenID != "tok-renew" {
		t.Fatalf("续期令牌的 jti 变了: %q", p2.TokenID)
	}
	if p2.RenewedToken != "" {
		t.Fatal("新令牌有效期充足，不应再次续期")
	}
}

func TestVerifyRejectsRevokedSession(t *testing.T) {
	auth, _ := newTestAuth(t)
	ctx := context.Background()

	// 签名有效但会话不存在（等价于已登出），必须拒绝。
	token := signTestToken(t, "tok-logged-out", testExpire)
	if _, err := auth.Verify(ctx, token); !errors.Is(err, errs.ErrUnauthorized) {
		t.Fatalf("会话缺失应返回 ErrUnauthorized，实际: %v", err)
	}
}

func TestVerifyRejectsExpiredToken(t *testing.T) {
	auth, sessions := newTestAuth(t)
	ctx := context.Background()

	token := signTestToken(t, "tok-expired", -time.Minute)
	if err := sessions.Set(ctx, sessionKeyPrefix+"tok-expired", "7", testExpire); err != nil {
		t.Fatalf("写入会话: %v", err)
	}

	if _, err := auth.Verify(ctx, token); !errors.Is(err, errs.ErrTokenExpired) {
		t.Fatalf("过期令牌应返回 ErrTokenExpired，实际: %v", err)
	}
}
