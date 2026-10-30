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
	// List 列出全部仓库；includeDeleted 为真时连已软删的一起列出，供恢复入口使用。
	List(ctx context.Context, includeDeleted bool) ([]model.Warehouse, error)
	FindByID(ctx context.Context, id uint) (*model.Warehouse, error)
	// FindByIDIncludingDeleted 是恢复路径专用：常规查询看不见已删除的仓库。
	FindByIDIncludingDeleted(ctx context.Context, id uint) (*model.Warehouse, error)
	Create(ctx context.Context, m *model.Warehouse) error
	UpdateFields(ctx context.Context, id uint, fields map[string]any) error
	Delete(ctx context.Context, id uint) error
	Restore(ctx context.Context, id uint) error
	ExistsByCode(ctx context.Context, code string, excludeID uint) (bool, error)
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

func (r *warehouseRepo) List(ctx context.Context, includeDeleted bool) ([]model.Warehouse, error) {
	rows := make([]model.Warehouse, 0, 8)
	q := r.conn(ctx)
	if includeDeleted {
		q = q.Unscoped()
	}
	if err := q.Order("is_default DESC, code ASC").Find(&rows).Error; err != nil {
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

func (r *warehouseRepo) FindByIDIncludingDeleted(
	ctx context.Context, id uint,
) (*model.Warehouse, error) {
	var m model.Warehouse
	err := r.conn(ctx).Unscoped().First(&m, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errs.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("查询仓库 %d（含已删除）: %w", id, err)
	}
	return &m, nil
}

func (r *warehouseRepo) Create(ctx context.Context, m *model.Warehouse) error {
	return r.Base.Create(ctx, m)
}

func (r *warehouseRepo) UpdateFields(ctx context.Context, id uint, fields map[string]any) error {
	return r.Base.UpdateFields(ctx, id, fields)
}

func (r *warehouseRepo) Delete(ctx context.Context, id uint) error {
	return r.Base.DeleteByID(ctx, id)
}

func (r *warehouseRepo) Restore(ctx context.Context, id uint) error {
	err := r.conn(ctx).Unscoped().Model(&model.Warehouse{}).
		Where("id = ?", id).
		Update("deleted_at", 0).Error
	if err != nil {
		return fmt.Errorf("恢复仓库 %d: %w", id, err)
	}
	return nil
}

func (r *warehouseRepo) ExistsByCode(ctx context.Context, code string, excludeID uint) (bool, error) {
	return r.Exists(ctx, func(db *gorm.DB) *gorm.DB {
		q := db.Where("code = ?", code)
		if excludeID > 0 {
			q = q.Where("id <> ?", excludeID)
		}
		return q
	})
}

func (r *warehouseRepo) DefaultID(ctx context.Context) (uint, error) {
	var m model.Warehouse
	// 不再按启用状态筛：仓库停用那套已经被删除替代，默认仓就是未删除的那条。
	err := r.conn(ctx).Where("is_default = ?", true).Order("id ASC").First(&m).Error
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
		Code: "MAIN", Name: "主仓", IsDefault: true,
		Remark: "系统初始化时自动创建；改用多仓后请把真实仓库补全",
	}
	if err := r.Base.Create(ctx, m); err != nil {
		return 0, err
	}
	return m.ID, nil
}
