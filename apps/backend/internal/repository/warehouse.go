package repository

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/errs"
)

// WarehouseRepository 读写仓库（T-56）。
type WarehouseRepository interface {
	List(ctx context.Context) ([]model.Warehouse, error)
	FindByID(ctx context.Context, id uint) (*model.Warehouse, error)
	Create(ctx context.Context, m *model.Warehouse) error
	UpdateFields(ctx context.Context, id uint, fields map[string]any) error
	// DefaultID 返回默认仓库 id；一条都没有时返回 0（调用方按"未指定"处理）。
	DefaultID(ctx context.Context) (uint, error)
	// EnsureDefault 在表为空时建一条「主仓」。
	//
	// 不建的话，存量库存全部落在 warehouse_id = 0 上，而界面上一个仓库都选不出来，
	// 看起来像"仓库功能坏了"。有了一条默认仓，存量与新增至少指向同一个地方。
	EnsureDefault(ctx context.Context) (uint, error)
}

type warehouseRepo struct {
	*Base[model.Warehouse]
}

func NewWarehouseRepository(db *gorm.DB) WarehouseRepository {
	return &warehouseRepo{Base: NewBase[model.Warehouse](db)}
}

func (r *warehouseRepo) List(ctx context.Context) ([]model.Warehouse, error) {
	rows := make([]model.Warehouse, 0, 8)
	if err := r.conn(ctx).Order("is_default DESC, code ASC").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("查询仓库列表: %w", err)
	}
	return rows, nil
}

func (r *warehouseRepo) FindByID(ctx context.Context, id uint) (*model.Warehouse, error) {
	var m model.Warehouse
	err := r.conn(ctx).First(&m, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errs.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("查询仓库 %d: %w", id, err)
	}
	return &m, nil
}

func (r *warehouseRepo) Create(ctx context.Context, m *model.Warehouse) error {
	return r.Base.Create(ctx, m)
}

func (r *warehouseRepo) UpdateFields(ctx context.Context, id uint, fields map[string]any) error {
	return r.Base.UpdateFields(ctx, id, fields)
}

func (r *warehouseRepo) DefaultID(ctx context.Context) (uint, error) {
	var m model.Warehouse
	err := r.conn(ctx).Where("is_default = ? AND status = ?", true, model.EnableStatusOn).
		Order("id ASC").First(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("查询默认仓库: %w", err)
	}
	return m.ID, nil
}

func (r *warehouseRepo) EnsureDefault(ctx context.Context) (uint, error) {
	n, err := r.Base.Count(ctx)
	if err != nil {
		return 0, err
	}
	if n > 0 {
		return r.DefaultID(ctx)
	}
	m := &model.Warehouse{
		Code: "MAIN", Name: "主仓", IsDefault: true, Status: model.EnableStatusOn,
		Remark: "系统初始化时自动创建；改用多仓后请把真实仓库补全",
	}
	if err := r.Base.Create(ctx, m); err != nil {
		return 0, err
	}
	return m.ID, nil
}
