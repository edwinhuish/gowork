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
	FindByUsername(ctx context.Context, username string) (*model.User, error)
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

func (r *userRepo) Count(ctx context.Context) (int64, error) {
	return r.Base.Count(ctx)
}
