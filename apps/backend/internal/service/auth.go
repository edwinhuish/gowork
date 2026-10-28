package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/infra/cache"
	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/errs"
	"github.com/edwinhuish/gowork/internal/repository"
)

const (
	sessionKeyPrefix = "session:"
	loginFailPrefix  = "login_fail:"
)

// AuthConfig 是鉴权相关的可调参数。
type AuthConfig struct {
	Secret       string
	Expire       time.Duration
	MaxLoginFail int
}

// Principal 是鉴权通过后的身份信息，由中间件注入请求上下文。
type Principal struct {
	UserID    uint
	Username  string
	Nickname  string
	Role      string
	TokenID   string
	ExpiresAt time.Time
}

func (p *Principal) IsAdmin() bool { return p.Role == model.RoleAdmin }

type AuthService interface {
	Login(ctx context.Context, req dto.LoginRequest) (*dto.LoginResponse, error)
	Logout(ctx context.Context, tokenID string) error
	Verify(ctx context.Context, token string) (*Principal, error)
	HashPassword(plain string) (string, error)
}

type authService struct {
	users    repository.UserRepository
	sessions cache.Cache
	cfg      AuthConfig
	log      *slog.Logger
}

type AuthOption func(*authService)

func WithAuthLogger(l *slog.Logger) AuthOption {
	return func(s *authService) { s.log = l }
}

func NewAuthService(users repository.UserRepository, sessions cache.Cache, cfg AuthConfig, opts ...AuthOption) AuthService {
	if cfg.Expire <= 0 {
		cfg.Expire = 24 * time.Hour
	}
	if cfg.MaxLoginFail <= 0 {
		cfg.MaxLoginFail = 5
	}

	s := &authService{users: users, sessions: sessions, cfg: cfg, log: slog.Default()}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *authService) HashPassword(plain string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("生成密码摘要: %w", err)
	}
	return string(h), nil
}

func (s *authService) Login(ctx context.Context, req dto.LoginRequest) (*dto.LoginResponse, error) {
	failKey := loginFailPrefix + req.Username
	if n, err := s.failCount(ctx, failKey); err == nil && n >= int64(s.cfg.MaxLoginFail) {
		return nil, errs.New(errs.CodeForbidden, "登录失败次数过多，请 15 分钟后再试")
	}

	user, err := s.users.FindByUsername(ctx, req.Username)
	if err != nil {
		s.markLoginFail(ctx, failKey)
		// 用户不存在与密码错误返回同一文案，避免账号枚举。
		return nil, errs.New(errs.CodeUnauthorized, "用户名或密码错误")
	}
	if !user.IsActive() {
		return nil, errs.New(errs.CodeForbidden, "账号已被禁用")
	}
	if bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password)) != nil {
		s.markLoginFail(ctx, failKey)
		return nil, errs.New(errs.CodeUnauthorized, "用户名或密码错误")
	}

	tokenID, err := newTokenID()
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, "生成会话标识失败")
	}

	now := time.Now()
	expireAt := now.Add(s.cfg.Expire)

	claims := tokenClaims{
		UserID:   user.ID,
		Username: user.Username,
		Nickname: user.Nickname,
		Role:     user.Role,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        tokenID,
			Subject:   strconv.FormatUint(uint64(user.ID), 10),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(expireAt),
		},
	}

	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(s.cfg.Secret))
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, "签发令牌失败")
	}

	// 会话落缓存：登出即失效，也支持后续接入强制下线。
	if err := s.sessions.Set(ctx, sessionKeyPrefix+tokenID, strconv.FormatUint(uint64(user.ID), 10), s.cfg.Expire); err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, "写入会话失败")
	}
	_ = s.sessions.Del(ctx, failKey)

	s.log.Info("用户登录", "user_id", user.ID, "username", user.Username)

	return &dto.LoginResponse{
		Token:     signed,
		ExpiresAt: expireAt.Unix(),
		User: &dto.UserInfo{
			ID:       user.ID,
			Username: user.Username,
			Nickname: user.Nickname,
			Role:     user.Role,
		},
	}, nil
}

func (s *authService) Logout(ctx context.Context, tokenID string) error {
	if tokenID == "" {
		return nil
	}
	return s.sessions.Del(ctx, sessionKeyPrefix+tokenID)
}

func (s *authService) Verify(ctx context.Context, token string) (*Principal, error) {
	var claims tokenClaims
	parsed, err := jwt.ParseWithClaims(token, &claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("不支持的签名算法: %v", t.Header["alg"])
		}
		return []byte(s.cfg.Secret), nil
	})
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, errs.ErrTokenExpired
		}
		return nil, errs.ErrUnauthorized
	}
	if !parsed.Valid {
		return nil, errs.ErrUnauthorized
	}

	// 令牌签名有效还不够：会话必须仍在缓存中，登出后立即失效。
	if _, err := s.sessions.Get(ctx, sessionKeyPrefix+claims.ID); err != nil {
		if errors.Is(err, cache.ErrMiss) {
			return nil, errs.ErrUnauthorized
		}
		return nil, errs.Wrap(err, errs.CodeInternal, "校验会话失败")
	}

	p := &Principal{
		UserID:   claims.UserID,
		Username: claims.Username,
		Nickname: claims.Nickname,
		Role:     claims.Role,
		TokenID:  claims.ID,
	}
	if claims.ExpiresAt != nil {
		p.ExpiresAt = claims.ExpiresAt.Time
	}
	return p, nil
}

func (s *authService) failCount(ctx context.Context, key string) (int64, error) {
	v, err := s.sessions.Get(ctx, key)
	if err != nil {
		return 0, err
	}
	return strconv.ParseInt(v, 10, 64)
}

func (s *authService) markLoginFail(ctx context.Context, key string) {
	if _, err := s.sessions.Incr(ctx, key, 15*time.Minute); err != nil {
		s.log.Warn("记录登录失败次数出错", "key", key, "err", err)
	}
}

type tokenClaims struct {
	UserID   uint   `json:"uid"`
	Username string `json:"usr"`
	Nickname string `json:"nick"`
	Role     string `json:"role"`
	jwt.RegisteredClaims
}

func newTokenID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
