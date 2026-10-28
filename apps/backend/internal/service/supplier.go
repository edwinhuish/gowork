package service

import (
	"context"
	"log/slog"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/errs"
	"github.com/edwinhuish/gowork/internal/repository"
)

type SupplierService interface {
	Create(ctx context.Context, in dto.SupplierInput) (*model.Supplier, error)
	Update(ctx context.Context, id uint, in dto.SupplierInput) (*model.Supplier, error)
	Delete(ctx context.Context, id uint) error
	Get(ctx context.Context, id uint) (*model.Supplier, error)
	List(ctx context.Context, q dto.SupplierQuery) (*dto.PageResult[model.Supplier], error)
}

type supplierService struct {
	repo repository.SupplierRepository
	log  *slog.Logger
}

type SupplierOption func(*supplierService)

func WithSupplierLogger(l *slog.Logger) SupplierOption {
	return func(s *supplierService) { s.log = l }
}

func NewSupplierService(repo repository.SupplierRepository, opts ...SupplierOption) SupplierService {
	s := &supplierService{repo: repo, log: slog.Default()}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *supplierService) Create(ctx context.Context, in dto.SupplierInput) (*model.Supplier, error) {
	status, err := normEnableStatus(in.Status, model.EnableStatusOn)
	if err != nil {
		return nil, err
	}
	dup, err := s.repo.ExistsByCode(ctx, in.Code, 0)
	if err != nil {
		return nil, err
	}
	if dup {
		return nil, errs.ErrConflict.WithMsg("供应商编码已存在：" + in.Code)
	}

	m := &model.Supplier{
		Code:    in.Code,
		Name:    in.Name,
		Contact: in.Contact,
		Phone:   in.Phone,
		Email:   in.Email,
		Address: in.Address,
		Status:  status,
		Remark:  in.Remark,
	}
	if err := s.repo.Create(ctx, m); err != nil {
		return nil, err
	}
	s.log.Info("新增供应商", "id", m.ID, "code", m.Code)
	return m, nil
}

func (s *supplierService) Update(ctx context.Context, id uint, in dto.SupplierInput) (*model.Supplier, error) {
	if _, err := s.repo.FindByID(ctx, id); err != nil {
		return nil, err
	}
	status, err := normEnableStatus(in.Status, model.EnableStatusOn)
	if err != nil {
		return nil, err
	}
	dup, err := s.repo.ExistsByCode(ctx, in.Code, id)
	if err != nil {
		return nil, err
	}
	if dup {
		return nil, errs.ErrConflict.WithMsg("供应商编码已存在：" + in.Code)
	}

	fields := map[string]any{
		"code":    in.Code,
		"name":    in.Name,
		"contact": in.Contact,
		"phone":   in.Phone,
		"email":   in.Email,
		"address": in.Address,
		"status":  status,
		"remark":  in.Remark,
	}
	if err := s.repo.Update(ctx, id, fields); err != nil {
		return nil, err
	}
	return s.repo.FindByID(ctx, id)
}

func (s *supplierService) Delete(ctx context.Context, id uint) error {
	if _, err := s.repo.FindByID(ctx, id); err != nil {
		return err
	}
	return s.repo.Delete(ctx, id)
}

func (s *supplierService) Get(ctx context.Context, id uint) (*model.Supplier, error) {
	return s.repo.FindByID(ctx, id)
}

func (s *supplierService) List(ctx context.Context, q dto.SupplierQuery) (*dto.PageResult[model.Supplier], error) {
	q.PageQuery.Normalize()

	filter := repository.SupplierFilter{Keyword: q.Keyword, Status: enableStatusPtr(q.Status)}
	page := repository.Pagination{Page: q.Page, PageSize: q.PageSize}

	list, total, err := s.repo.List(ctx, filter, page)
	if err != nil {
		return nil, err
	}
	return dto.NewPageResult(list, total, q.Page, q.PageSize), nil
}
