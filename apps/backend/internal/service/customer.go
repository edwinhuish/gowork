package service

import (
	"context"
	"log/slog"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/errs"
	"github.com/edwinhuish/gowork/internal/repository"
)

type CustomerService interface {
	Create(ctx context.Context, in dto.CustomerInput) (*model.Customer, error)
	Update(ctx context.Context, id uint, in dto.CustomerInput) (*model.Customer, error)
	Delete(ctx context.Context, id uint) error
	Get(ctx context.Context, id uint) (*model.Customer, error)
	List(ctx context.Context, q dto.CustomerQuery) (*dto.PageResult[model.Customer], error)
}

type customerService struct {
	repo repository.CustomerRepository
	log  *slog.Logger
}

type CustomerOption func(*customerService)

func WithCustomerLogger(l *slog.Logger) CustomerOption {
	return func(s *customerService) { s.log = l }
}

func NewCustomerService(repo repository.CustomerRepository, opts ...CustomerOption) CustomerService {
	s := &customerService{repo: repo, log: slog.Default()}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *customerService) Create(ctx context.Context, in dto.CustomerInput) (*model.Customer, error) {
	status, err := normEnableStatus(in.Status, model.EnableStatusOn)
	if err != nil {
		return nil, err
	}
	dup, err := s.repo.ExistsByCode(ctx, in.Code, 0)
	if err != nil {
		return nil, err
	}
	if dup {
		return nil, errs.ErrConflict.WithMsg("客户编码已存在：" + in.Code)
	}

	m := &model.Customer{
		Code:        in.Code,
		Name:        in.Name,
		Country:     in.Country,
		ContactName: in.ContactName,
		Phone:       in.Phone,
		Email:       in.Email,
		Address:     in.Address,
		Status:      status,
		Remark:      in.Remark,
	}
	if err := s.repo.Create(ctx, m); err != nil {
		return nil, err
	}
	s.log.Info("新增客户", "id", m.ID, "code", m.Code)
	return m, nil
}

func (s *customerService) Update(ctx context.Context, id uint, in dto.CustomerInput) (*model.Customer, error) {
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
		return nil, errs.ErrConflict.WithMsg("客户编码已存在：" + in.Code)
	}

	fields := map[string]any{
		"code":    in.Code,
		"name":    in.Name,
		"country": in.Country,
		"contact": in.ContactName,
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

func (s *customerService) Delete(ctx context.Context, id uint) error {
	if _, err := s.repo.FindByID(ctx, id); err != nil {
		return err
	}
	return s.repo.Delete(ctx, id)
}

func (s *customerService) Get(ctx context.Context, id uint) (*model.Customer, error) {
	return s.repo.FindByID(ctx, id)
}

func (s *customerService) List(ctx context.Context, q dto.CustomerQuery) (*dto.PageResult[model.Customer], error) {
	q.PageQuery.Normalize()

	filter := repository.CustomerFilter{Keyword: q.Keyword, Country: q.Country, Status: enableStatusPtr(q.Status)}
	page := repository.Pagination{Page: q.Page, PageSize: q.PageSize}

	list, total, err := s.repo.List(ctx, filter, page)
	if err != nil {
		return nil, err
	}
	return dto.NewPageResult(list, total, q.Page, q.PageSize), nil
}
