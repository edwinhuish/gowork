package service

import (
	"context"
	"log/slog"
	"strings"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/errs"
	"github.com/edwinhuish/gowork/internal/repository"
)

// UserCredentialKeeper 是账号维护需要的鉴权能力：算密码摘要、吊销会话。
//
// 用窄接口而不是直接依赖 AuthService：用户维护只要这两件事，
// 依赖整个鉴权接口会让"这两个方法之外还能干什么"变得不明确。
type UserCredentialKeeper interface {
	HashPassword(plain string) (string, error)
	RevokeUser(ctx context.Context, userID uint) error
}

// UserService 管账号目录与账号维护（T-57）。
//
// 建号、改角色、停用、重置密码以前只能改数据库。团队里出现第二个人的那天，
// 「离职账号停不掉」就从"不方便"变成"事故"——而角色写在 JWT 里，
// 改完还要等令牌过期才生效，所以每一次变更都要吊销该账号的会话。
type UserService interface {
	// ListActive 列出启用中的账号，供"把客户转给谁"这类选择器使用。
	ListActive(ctx context.Context) ([]model.User, error)
	// List 列出全部账号（含停用），供维护页使用。
	List(ctx context.Context) ([]model.User, error)
	Create(ctx context.Context, in dto.UserInput) (*model.User, error)
	Update(ctx context.Context, id uint, in dto.UserUpdateInput) (*model.User, error)
}

type userService struct {
	users repository.UserRepository
	auth  UserCredentialKeeper
	log   *slog.Logger
}

// UserOption 供 bootstrap 注入日志器。
type UserOption func(*userService)

func WithUserLogger(l *slog.Logger) UserOption {
	return func(s *userService) { s.log = l }
}

func NewUserService(
	users repository.UserRepository, auth UserCredentialKeeper, opts ...UserOption,
) UserService {
	s := &userService{users: users, auth: auth, log: slog.Default()}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *userService) ListActive(ctx context.Context) ([]model.User, error) {
	actor, ok := ActorFrom(ctx)
	if !ok || !model.RoleSeesAllData(actor.Role) {
		// 业务员不需要这个列表：他既不能转移客户，也不需要知道公司有谁。
		return nil, errs.ErrForbidden
	}
	return s.users.ListActive(ctx)
}

func (s *userService) List(ctx context.Context) ([]model.User, error) {
	if err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	return s.users.List(ctx)
}

func (s *userService) Create(ctx context.Context, in dto.UserInput) (*model.User, error) {
	if err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	role := strings.TrimSpace(in.Role)
	if !model.RoleIsValid(role) {
		return nil, errs.BadRequest("角色只能是 admin / manager / staff")
	}
	username := strings.TrimSpace(in.Username)
	if username == "" {
		return nil, errs.BadRequest("用户名不能为空")
	}
	// 先查一次给出 409；唯一索引仍然兜底，两者都要——
	// 前置查重是为了给一句能看懂的话，索引是为了并发下不出现两个同名账号。
	if _, err := s.users.FindByUsername(ctx, username); err == nil {
		return nil, errs.ErrConflict.WithMsg("用户名已存在：" + username)
	}

	hashed, err := s.auth.HashPassword(in.Password)
	if err != nil {
		return nil, err
	}
	m := &model.User{
		Username: username,
		Password: hashed,
		Nickname: strings.TrimSpace(in.Nickname),
		Role:     role,
		Status:   model.EnableStatusOn,
	}
	if err := s.users.Create(ctx, m); err != nil {
		return nil, err
	}
	s.log.Info("新建账号", "user_id", m.ID, "username", m.Username, "role", m.Role)
	return m, nil
}

func (s *userService) Update(ctx context.Context, id uint, in dto.UserUpdateInput) (*model.User, error) {
	actor, err := requireAdminActor(ctx)
	if err != nil {
		return nil, err
	}
	existing, err := s.users.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}

	fields := make(map[string]any, 4)
	// 这三件事都会让已签发的令牌与新状态不一致：角色变了令牌里还是旧角色，
	// 停用了令牌却还有效，密码换了旧令牌仍能进来。任一发生都要吊销。
	revoke := false

	if in.Nickname != nil {
		fields["nickname"] = strings.TrimSpace(*in.Nickname)
	}
	if in.Role != nil {
		role := strings.TrimSpace(*in.Role)
		if !model.RoleIsValid(role) {
			return nil, errs.BadRequest("角色只能是 admin / manager / staff")
		}
		if role != existing.Role {
			// 先判"是不是自己"，再判"是不是最后一个管理员"：
			// 两件事同时成立时，前者才是操作人需要看到的提示——
			// 他真正要做的是找另一位管理员，而不是去数还剩几个 admin。
			if actor.UserID == existing.ID && role != model.RoleAdmin {
				return nil, errs.BadRequest("不能修改自己的角色，请让另一位管理员操作")
			}
			if existing.Role == model.RoleAdmin {
				if err := s.guardLastAdmin(ctx, existing, "降级"); err != nil {
					return nil, err
				}
			}
			fields["role"] = role
			revoke = true
		}
	}
	if in.Status != nil {
		status := model.EnableStatus(*in.Status)
		if status != existing.Status {
			if status == model.EnableStatusOff {
				if actor.UserID == existing.ID {
					return nil, errs.BadRequest("不能停用自己的账号")
				}
				if err := s.guardLastAdmin(ctx, existing, "停用"); err != nil {
					return nil, err
				}
			}
			fields["status"] = status
			revoke = true
		}
	}
	if in.Password != nil && *in.Password != "" {
		hashed, err := s.auth.HashPassword(*in.Password)
		if err != nil {
			return nil, err
		}
		fields["password"] = hashed
		revoke = true
	}

	if len(fields) == 0 {
		return existing, nil
	}
	if err := s.users.Update(ctx, id, fields); err != nil {
		return nil, err
	}
	if revoke {
		// 吊销放在改库之后：改失败了不该把人踢下线。
		if err := s.auth.RevokeUser(ctx, id); err != nil {
			return nil, err
		}
	}
	s.log.Info("更新账号", "user_id", id, "username", existing.Username, "fields", len(fields), "revoked", revoke)
	return s.users.FindByID(ctx, id)
}

// guardLastAdmin 拦住「停用或降级最后一个可用管理员」。
//
// 系统里没有"恢复出厂账号"的入口，进了这个状态只能改数据库；
// 而发现它的时候通常正是最需要进系统设置的那一刻。
func (s *userService) guardLastAdmin(ctx context.Context, existing *model.User, action string) error {
	if existing.Role != model.RoleAdmin || existing.Status != model.EnableStatusOn {
		return nil
	}
	n, err := s.users.CountActiveAdmins(ctx)
	if err != nil {
		return err
	}
	if n <= 1 {
		return errs.ErrConflict.WithMsg("这是最后一个可用的管理员，不能" + action +
			"；请先把另一个账号提为管理员")
	}
	return nil
}

func requireAdmin(ctx context.Context) error {
	_, err := requireAdminActor(ctx)
	return err
}

func requireAdminActor(ctx context.Context) (*Principal, error) {
	actor, ok := ActorFrom(ctx)
	if !ok || !actor.IsAdmin() {
		return nil, errs.ErrForbidden
	}
	return actor, nil
}
