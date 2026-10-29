package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
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
	// revokedKeyPrefix 记「这个账号在某时刻之前的令牌全部作废」（T-57）。
	//
	// 用时间戳而不是遍历删除会话：缓存没有"按前缀列出 key"的能力，
	// 而会话 key 里带的是令牌 ID、不含用户 ID，从用户反查不到令牌。
	// 时间戳方案只写一个 key，校验时比一次大小。
	revokedKeyPrefix = "revoked:"
)

// errLoginTooManyFails 是"猜错太多次"的统一出口。
//
// 已知账号与不存在的账号必须回同一句：否则连续尝试几下就能从
// "403 过多"与"401 密码错误"的差别里看出账号是否存在。
var errLoginTooManyFails = errs.New(errs.CodeForbidden, "登录失败次数过多，请 15 分钟后再试")

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

	// RenewedToken 非空表示本次请求触发了滑动续期。
	// 中间件应把它写进 X-Renewed-Token 响应头，由客户端替换本地令牌。
	RenewedToken string
}

func (p *Principal) IsAdmin() bool { return p.Role == model.RoleAdmin }

type AuthService interface {
	Login(ctx context.Context, req dto.LoginRequest) (*dto.LoginResponse, error)
	Logout(ctx context.Context, tokenID string) error
	// RevokeUser 让该账号已签发的全部令牌失效：停用、改角色、重置密码后必须调它。
	RevokeUser(ctx context.Context, userID uint) error
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
	// 失败计数只决定"还能不能继续猜"，**不预先拒绝请求**。
	//
	// 早先的写法是先看计数、超了直接 403，于是任何人连错 5 次就能把某个账号
	// 锁在门外 15 分钟——被锁的人手里握着正确密码，却什么也做不了。
	// 现在把密码校验放在前面：密码对就放行并清计数，密码错才计数、超了才 403。
	// 攻击者仍然无法继续猜（错就到 403），但锁不住知道密码的人。
	//
	// 代价：锁定期内每次错误尝试仍要算一次 bcrypt（约为一次正常登录的开销）。
	// 这是为了让"手里有正确密码的人"能进来——省掉这次计算就得先拒请求，
	// 而那正是把账号锁死的写法。
	over, err := s.loginLocked(ctx, failKey)
	if err != nil {
		// 计数读不出来不该把登录整体拦死（缓存抖动时用户还是要能进来），
		// 但要在日志里留下痕迹。
		s.log.Warn("读取登录失败次数出错", "key", failKey, "err", err)
	}

	user, err := s.users.FindByUsername(ctx, req.Username)
	if err != nil {
		s.markLoginFail(ctx, failKey)
		// 用户不存在与密码错误返回同一文案，避免账号枚举。
		if over {
			return nil, errLoginTooManyFails
		}
		return nil, errs.New(errs.CodeUnauthorized, "用户名或密码错误")
	}
	if !user.IsEnabled() {
		return nil, errs.New(errs.CodeForbidden, "账号已被禁用")
	}
	if bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password)) != nil {
		n := s.markLoginFail(ctx, failKey)
		// 这一次让计数跨过阈值时也报"过多"：否则第 6 次仍是"密码错误"，
		// 而用户已经无法再试了，提示与实际状态不一致。
		if over || n >= int64(s.cfg.MaxLoginFail) {
			return nil, errLoginTooManyFails
		}
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

	// 会话落缓存：登出即失效，也支持强制下线（T-57）。
	//
	// 值里带签发时间：改角色或停用账号时只需记下"此刻起旧令牌作废"，
	// 校验时比较签发时间与作废时刻即可，不必遍历该用户的会话。
	if err := s.sessions.Set(ctx, sessionKeyPrefix+tokenID,
		fmt.Sprintf("%d:%d", user.ID, time.Now().Unix()), s.cfg.Expire); err != nil {
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

// RevokeUser 让某个账号在此刻之前签发的令牌全部失效（T-57）。
//
// 停用账号、改角色、重置密码之后都要调它：角色写在 JWT claims 里，
// 不吊销会话的话，降权与停用要等到令牌自然过期才生效——而"离职当天还能登进来"
// 是这类系统最不该有的行为。
func (s *authService) RevokeUser(ctx context.Context, userID uint) error {
	key := revokedKeyPrefix + strconv.FormatUint(uint64(userID), 10)
	if err := s.sessions.Set(ctx, key, strconv.FormatInt(time.Now().Unix(), 10), s.cfg.Expire); err != nil {
		return errs.Wrap(err, errs.CodeInternal, "吊销会话失败")
	}
	return nil
}

// sessionRevoked 判断这条会话是否签发自"账号信息已变更"之前。
//
// 会话值是 `uid:签发时间`；老格式（只存 uid）认不出签发时间，一律放行——
// 那种会话最多活到自然过期，而把所有存量会话一次性踢下线，
// 换来的是一次全员重新登录，不值得。
func (s *authService) sessionRevoked(ctx context.Context, userID uint, sessionValue string) (bool, error) {
	idx := strings.LastIndex(sessionValue, ":")
	if idx < 0 {
		return false, nil
	}
	issuedAt, err := strconv.ParseInt(sessionValue[idx+1:], 10, 64)
	if err != nil {
		return false, nil
	}
	v, err := s.sessions.Get(ctx, revokedKeyPrefix+strconv.FormatUint(uint64(userID), 10))
	if errors.Is(err, cache.ErrMiss) {
		return false, nil
	}
	if err != nil {
		return false, errs.Wrap(err, errs.CodeInternal, "校验会话吊销状态失败")
	}
	revokedAt, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return false, nil
	}
	return issuedAt <= revokedAt, nil
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
	sessionValue, err := s.sessions.Get(ctx, sessionKeyPrefix+claims.ID)
	if err != nil {
		if errors.Is(err, cache.ErrMiss) {
			return nil, errs.ErrUnauthorized
		}
		return nil, errs.Wrap(err, errs.CodeInternal, "校验会话失败")
	}
	// 账号被停用或改了角色之后，这一刻之前签发的令牌全部作废（T-57）。
	// 角色写在 JWT 里，不作废的话降权要等到令牌自然过期才生效。
	if revoked, err := s.sessionRevoked(ctx, claims.UserID, sessionValue); err != nil {
		return nil, err
	} else if revoked {
		return nil, errs.ErrUnauthorized.WithMsg("账号信息已变更，请重新登录")
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
	s.renew(ctx, &claims, p)
	return p, nil
}

// renew 实现滑动续期：剩余有效期不足一半时重签令牌并重置会话 TTL。
//
// 关键点是复用原 jti。并发请求各自换出的令牌字符串不同，但都指向同一条 session，
// 所以续期天然幂等，不会因为一次页面加载的多个并发请求把会话记录写膨胀。
// 如果每次续期都换新 jti，就得额外做去重，否则 session 条数随请求数增长。
//
// 续期失败只放弃本次续期，不影响当前请求：此刻手上的令牌还没过期。
// 顺序上必须先重置 session 成功再下发新令牌，否则新令牌的 exp 会超出会话寿命，
// 客户端拿着一个看似有效、实际已被会话吊销的令牌反复重试。
func (s *authService) renew(ctx context.Context, claims *tokenClaims, p *Principal) {
	if claims.ExpiresAt == nil || time.Until(claims.ExpiresAt.Time) > s.cfg.Expire/2 {
		return
	}

	expireAt := time.Now().Add(s.cfg.Expire)
	claims.ExpiresAt = jwt.NewNumericDate(expireAt)

	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(s.cfg.Secret))
	if err != nil {
		s.log.Warn("续签令牌失败", "token_id", claims.ID, "err", err)
		return
	}
	uid := strconv.FormatUint(uint64(claims.UserID), 10)
	if err := s.sessions.Set(ctx, sessionKeyPrefix+claims.ID, uid, s.cfg.Expire); err != nil {
		s.log.Warn("重置会话有效期失败", "token_id", claims.ID, "err", err)
		return
	}

	p.RenewedToken = signed
	p.ExpiresAt = expireAt
}

func (s *authService) failCount(ctx context.Context, key string) (int64, error) {
	v, err := s.sessions.Get(ctx, key)
	if err != nil {
		return 0, err
	}
	return strconv.ParseInt(v, 10, 64)
}

// loginLocked 报告这个账号是否已经用完猜密码的次数。
func (s *authService) loginLocked(ctx context.Context, key string) (bool, error) {
	n, err := s.failCount(ctx, key)
	if err != nil {
		return false, err
	}
	return n >= int64(s.cfg.MaxLoginFail), nil
}

// markLoginFail 记一次失败并返回累计次数（读不出累计值时返回 0）。
func (s *authService) markLoginFail(ctx context.Context, key string) int64 {
	n, err := s.sessions.Incr(ctx, key, 15*time.Minute)
	if err != nil {
		s.log.Warn("记录登录失败次数出错", "key", key, "err", err)
		return 0
	}
	return n
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
