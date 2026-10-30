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

// WarehouseService 维护仓库主数据（T-56）。
//
// 在此之前仓库只有一条启动时自动建的「主仓」，界面上一处都改不了——
// 而调拨、按仓盘点、按仓看库存都要先有两个仓可选。
type WarehouseService interface {
	// List 列出全部未删除的仓库；includeDeleted 为真时连已软删的一起列出。
	List(ctx context.Context, includeDeleted bool) ([]model.Warehouse, error)
	Create(ctx context.Context, in dto.WarehouseInput) (*model.Warehouse, error)
	Update(ctx context.Context, id uint, in dto.WarehouseInput) error
	Delete(ctx context.Context, id uint) error
	Restore(ctx context.Context, id uint) (*model.Warehouse, error)
}

type warehouseService struct {
	repo repository.WarehouseRepository
	log  *slog.Logger
}

type WarehouseOption func(*warehouseService)

func WithWarehouseLogger(l *slog.Logger) WarehouseOption {
	return func(s *warehouseService) { s.log = l }
}

func NewWarehouseService(repo repository.WarehouseRepository, opts ...WarehouseOption) WarehouseService {
	s := &warehouseService{repo: repo, log: slog.Default()}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *warehouseService) List(ctx context.Context, includeDeleted bool) ([]model.Warehouse, error) {
	return s.repo.List(ctx, includeDeleted)
}

func (s *warehouseService) Create(
	ctx context.Context, in dto.WarehouseInput,
) (*model.Warehouse, error) {
	m := &model.Warehouse{
		Code:      strings.ToUpper(strings.TrimSpace(in.Code)),
		Name:      strings.TrimSpace(in.Name),
		Address:   strings.TrimSpace(in.Address),
		IsDefault: in.IsDefault,
		Remark:    strings.TrimSpace(in.Remark),
	}
	if m.Code == "" || m.Name == "" {
		return nil, errs.BadRequest("仓库编码与名称不能为空")
	}
	if err := s.repo.Create(ctx, m); err != nil {
		return nil, err
	}
	// 设成默认就把别的降级：允许多条为 true 的话，"默认仓"就成了"随便挑一个"。
	if m.IsDefault {
		if err := s.demoteOthers(ctx, m.ID); err != nil {
			return nil, err
		}
	}
	s.log.Info("新增仓库", "code", m.Code, "name", m.Name, "default", m.IsDefault)
	return m, nil
}

func (s *warehouseService) Update(
	ctx context.Context, id uint, in dto.WarehouseInput,
) error {
	existing, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return err
	}
	fields := map[string]any{
		"name":       strings.TrimSpace(in.Name),
		"address":    strings.TrimSpace(in.Address),
		"is_default": in.IsDefault,
		"remark":     strings.TrimSpace(in.Remark),
	}
	// 编码不在这里改：它是出入库流水与报表里认的那个短标识，
	// 改了之后历史数据仍指着旧值，两边就对不上。
	if err := s.repo.UpdateFields(ctx, id, fields); err != nil {
		return err
	}
	if in.IsDefault && !existing.IsDefault {
		if err := s.demoteOthers(ctx, id); err != nil {
			return err
		}
	}
	// 取消默认仓之后必须还有一个默认仓：否则新流水会落到 warehouse_id = 0 上，
	// 而那是一个不存在的仓库。
	if existing.IsDefault && !in.IsDefault {
		if err := s.ensureDefault(ctx, id); err != nil {
			return err
		}
	}
	s.log.Info("修改仓库", "id", id, "default", in.IsDefault)
	return nil
}

// Delete 软删一个仓库。
//
// 默认仓不让删：它不是"更常用的一条"，而是新流水不指定仓库时唯一能落的位置，
// 删掉它等价于把默认仓变成"没有"，届时落下去的 warehouse_id = 0 查不出任何仓库。
func (s *warehouseService) Delete(ctx context.Context, id uint) error {
	m, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return err
	}
	if m.IsDefault {
		return errs.ErrConflict.WithMsg("默认仓不能删除，请先把另一个仓库设为默认仓")
	}
	return s.repo.Delete(ctx, id)
}

func (s *warehouseService) Restore(ctx context.Context, id uint) (*model.Warehouse, error) {
	m, err := s.repo.FindByIDIncludingDeleted(ctx, id)
	if err != nil {
		return nil, err
	}
	if m.DeletedAt == 0 {
		return nil, errs.BadRequest("该仓库未被删除，无需恢复")
	}
	// 编码只在未删的记录里唯一，删掉之后可以重录同一个码；
	// 那时恢复会让两条活着的记录共用编码，而流水里存的是 id 与编码两个口径。
	dup, err := s.repo.ExistsByCode(ctx, m.Code, id)
	if err != nil {
		return nil, err
	}
	if dup {
		return nil, errs.ErrConflict.WithMsg("仓库编码已被占用：" + m.Code)
	}
	if err := s.repo.Restore(ctx, id); err != nil {
		return nil, err
	}
	s.log.Info("恢复仓库", "id", id, "code", m.Code)
	return s.repo.FindByID(ctx, id)
}

// demoteOthers 把除 id 之外的仓库取消默认。
func (s *warehouseService) demoteOthers(ctx context.Context, id uint) error {
	rows, err := s.repo.List(ctx, false)
	if err != nil {
		return err
	}
	for _, w := range rows {
		if w.ID == id || !w.IsDefault {
			continue
		}
		if err := s.repo.UpdateFields(ctx, w.ID, map[string]any{"is_default": false}); err != nil {
			return err
		}
	}
	return nil
}

// ensureDefault 在默认仓被取消之后，把剩下最早的一条仓库顶上。
func (s *warehouseService) ensureDefault(ctx context.Context, excludeID uint) error {
	rows, err := s.repo.List(ctx, false)
	if err != nil {
		return err
	}
	for _, w := range rows {
		if w.ID == excludeID {
			continue
		}
		return s.repo.UpdateFields(ctx, w.ID, map[string]any{"is_default": true})
	}
	return nil
}
