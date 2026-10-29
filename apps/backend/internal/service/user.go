package service

import (
	"context"

	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/errs"
	"github.com/edwinhuish/gowork/internal/repository"
)

// UserService 是账号的只读目录。
//
// 目前只服务"把客户转给谁"这一个选择器：没有建号、改角色、重置密码——
// 那些是用户维护模块的事，等真有第二个使用者时再做，现在做等于凭空设计一套界面。
type UserService interface {
	// ListActive 列出启用中的账号，仅管理员与主管可调用（他们要分配客户）。
	ListActive(ctx context.Context) ([]model.User, error)
}

type userService struct {
	users repository.UserRepository
}

func NewUserService(users repository.UserRepository) UserService {
	return &userService{users: users}
}

func (s *userService) ListActive(ctx context.Context) ([]model.User, error) {
	actor, ok := ActorFrom(ctx)
	if !ok || !model.RoleSeesAllData(actor.Role) {
		// 业务员不需要这个列表：他既不能转移客户，也不需要知道公司有谁。
		return nil, errs.ErrForbidden
	}
	return s.users.ListActive(ctx)
}
