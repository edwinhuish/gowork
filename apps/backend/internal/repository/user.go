package repository

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/errs"
)

type UserRepository interface {
	Create(ctx context.Context, m *model.User) error
	FindByID(ctx context.Context, id uint) (*model.User, error)
	FindByUsername(ctx context.Context, username string) (*model.User, error)
	// ListActive 列出**启用中**的账号，供"把客户转给谁"这类选择器使用。
	// 停用账号不该出现在下拉里：转给一个看不到的人等于把客户弄丢了。
	ListActive(ctx context.Context) ([]model.User, error)
	// List 列出全部账号（含停用与已软删之外的），供用户维护页使用。
	// 与 ListActive 分开而不是加一个参数：选择器要的是"能派给谁"，
	// 维护页要的是"公司里有哪些账号"，两个集合的含义不同。
	List(ctx context.Context) ([]model.User, error)
	Update(ctx context.Context, id uint, fields map[string]any) error
	// CountActiveAdmins 数一数还有几个能用的管理员：停用或降级最后一个 admin，
	// 会让系统进入"谁都进不了系统设置"的状态，且只能改数据库才能恢复。
	CountActiveAdmins(ctx context.Context) (int64, error)
	Count(ctx context.Context) (int64, error)
}

type userRepo struct {
	*Base[model.User]
}

func NewUserRepository(db *gorm.DB) UserRepository {
	return &userRepo{Base: NewBase[model.User](db)}
}

func (r *userRepo) Create(ctx context.Context, m *model.User) error {
	return r.Base.Create(ctx, m)
}

func (r *userRepo) FindByID(ctx context.Context, id uint) (*model.User, error) {
	return r.Base.FindByID(ctx, id)
}

func (r *userRepo) FindByUsername(ctx context.Context, username string) (*model.User, error) {
	var u model.User
	err := r.conn(ctx).Where("username = ?", username).First(&u).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errs.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("按用户名查询: %w", err)
	}
	return &u, nil
}

func (r *userRepo) ListActive(ctx context.Context) ([]model.User, error) {
	users := make([]model.User, 0, 16)
	err := r.conn(ctx).
		Where("status = ?", model.EnableStatusOn).
		Order("username ASC").
		Find(&users).Error
	if err != nil {
		return nil, fmt.Errorf("查询账号列表: %w", err)
	}
	return users, nil
}

func (r *userRepo) List(ctx context.Context) ([]model.User, error) {
	users := make([]model.User, 0, 16)
	if err := r.conn(ctx).Order("username ASC").Find(&users).Error; err != nil {
		return nil, fmt.Errorf("查询全部账号: %w", err)
	}
	return users, nil
}

func (r *userRepo) Update(ctx context.Context, id uint, fields map[string]any) error {
	return r.Base.UpdateFields(ctx, id, fields)
}

func (r *userRepo) CountActiveAdmins(ctx context.Context) (int64, error) {
	return r.Base.Count(ctx, func(db *gorm.DB) *gorm.DB {
		return db.Where("role = ? AND status = ?", model.RoleAdmin, model.EnableStatusOn)
	})
}

func (r *userRepo) Count(ctx context.Context) (int64, error) {
	return r.Base.Count(ctx)
}
