package repository

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/errs"
)

// InspectionRepository 读写验货记录（T-51b）。
type InspectionRepository interface {
	ListByOrder(ctx context.Context, orderID uint) ([]model.Inspection, error)
	FindByID(ctx context.Context, id uint) (*model.Inspection, error)
	Create(ctx context.Context, m *model.Inspection) error
	DeleteByID(ctx context.Context, id uint) error
}

type inspectionRepo struct {
	*Base[model.Inspection]
}

func NewInspectionRepository(db *gorm.DB) InspectionRepository {
	return &inspectionRepo{Base: NewBase[model.Inspection](db)}
}

func (r *inspectionRepo) ListByOrder(
	ctx context.Context, orderID uint,
) ([]model.Inspection, error) {
	rows := make([]model.Inspection, 0, 8)
	err := r.conn(ctx).
		Where("purchase_order_id = ?", orderID).
		// 新的在前：最近一次验货是回答"现在能不能出"的那一条。
		Order("inspected_at DESC, id DESC").
		Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("查询采购单 %d 的验货记录: %w", orderID, err)
	}
	return rows, nil
}

func (r *inspectionRepo) FindByID(ctx context.Context, id uint) (*model.Inspection, error) {
	var m model.Inspection
	err := r.conn(ctx).First(&m, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errs.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("查询验货记录 %d: %w", id, err)
	}
	return &m, nil
}

func (r *inspectionRepo) Create(ctx context.Context, m *model.Inspection) error {
	return r.Base.Create(ctx, m)
}

func (r *inspectionRepo) DeleteByID(ctx context.Context, id uint) error {
	return r.Base.DeleteByID(ctx, id)
}
