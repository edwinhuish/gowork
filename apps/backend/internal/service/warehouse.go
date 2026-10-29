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
	List(ctx context.Context) ([]model.Warehouse, error)
	Create(ctx context.Context, in dto.WarehouseInput) (*model.Warehouse, error)
	Update(ctx context.Context, id uint, in dto.WarehouseInput) error
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

func (s *warehouseService) List(ctx context.Context) ([]model.Warehouse, error) {
	return s.repo.List(ctx)
}

func (s *warehouseService) Create(
	ctx context.Context, in dto.WarehouseInput,
) (*model.Warehouse, error) {
	m := &model.Warehouse{
		Code:      strings.ToUpper(strings.TrimSpace(in.Code)),
		Name:      strings.TrimSpace(in.Name),
		Address:   strings.TrimSpace(in.Address),
		IsDefault: in.IsDefault,
		Status:    normalizeWarehouseStatus(in.Status),
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
		"status":     normalizeWarehouseStatus(in.Status),
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
	// 停用默认仓之后必须还有一个默认仓：否则新流水会落到 warehouse_id = 0 上，
	// 而那是一个不存在的仓库。
	if existing.IsDefault && (!in.IsDefault || normalizeWarehouseStatus(in.Status) != model.EnableStatusOn) {
		if err := s.ensureDefault(ctx, id); err != nil {
			return err
		}
	}
	s.log.Info("修改仓库", "id", id, "default", in.IsDefault)
	return nil
}

// demoteOthers 把除 id 之外的仓库取消默认。
func (s *warehouseService) demoteOthers(ctx context.Context, id uint) error {
	rows, err := s.repo.List(ctx)
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

// ensureDefault 在默认仓被取消 / 停用之后，把剩下最早的一条启用仓库顶上。
func (s *warehouseService) ensureDefault(ctx context.Context, excludeID uint) error {
	rows, err := s.repo.List(ctx)
	if err != nil {
		return err
	}
	for _, w := range rows {
		if w.ID == excludeID || w.Status != model.EnableStatusOn {
			continue
		}
		return s.repo.UpdateFields(ctx, w.ID, map[string]any{"is_default": true})
	}
	return nil
}

// normalizeWarehouseStatus 把 0 值当启用：新仓库默认就是能用的。
func normalizeWarehouseStatus(v int8) model.EnableStatus {
	if v == int8(model.EnableStatusOff) {
		return model.EnableStatusOff
	}
	return model.EnableStatusOn
}
