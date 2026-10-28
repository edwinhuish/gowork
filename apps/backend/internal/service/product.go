package service

import (
	"context"
	"log/slog"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/errs"
	"github.com/edwinhuish/gowork/internal/repository"
)

type ProductService interface {
	Create(ctx context.Context, in dto.ProductInput) (*model.Product, error)
	Update(ctx context.Context, id uint, in dto.ProductInput) (*model.Product, error)
	Delete(ctx context.Context, id uint) error
	Get(ctx context.Context, id uint) (*model.Product, error)
	List(ctx context.Context, q dto.ProductQuery) (*dto.PageResult[model.Product], error)
}

type productService struct {
	repo repository.ProductRepository
	log  *slog.Logger
}

// ProductOption 用于注入可选依赖，保持构造函数签名稳定。
type ProductOption func(*productService)

func WithProductLogger(l *slog.Logger) ProductOption {
	return func(s *productService) { s.log = l }
}

func NewProductService(repo repository.ProductRepository, opts ...ProductOption) ProductService {
	s := &productService{repo: repo, log: slog.Default()}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *productService) Create(ctx context.Context, in dto.ProductInput) (*model.Product, error) {
	status, err := normEnableStatus(in.Status, model.EnableStatusOn)
	if err != nil {
		return nil, err
	}

	dup, err := s.repo.ExistsBySKUCode(ctx, in.SKUCode, 0)
	if err != nil {
		return nil, err
	}
	if dup {
		return nil, errs.ErrConflict.WithMsg("产品编码已存在：" + in.SKUCode)
	}

	m := &model.Product{
		SKUCode:    in.SKUCode,
		NameCN:     in.NameCN,
		NameEN:     in.NameEN,
		Category:   in.Category,
		Spec:       in.Spec,
		Unit:       defaultUnit(in.Unit),
		SupplierID: in.SupplierID,
		CostPrice:  in.CostPrice,
		SalePrice:  in.SalePrice,
		Status:     status,
		Remark:     in.Remark,
	}
	if err := s.repo.Create(ctx, m); err != nil {
		return nil, err
	}

	s.log.Info("新增产品", "id", m.ID, "sku", m.SKUCode)
	return m, nil
}

func (s *productService) Update(ctx context.Context, id uint, in dto.ProductInput) (*model.Product, error) {
	if _, err := s.repo.FindByID(ctx, id); err != nil {
		return nil, err
	}
	status, err := normEnableStatus(in.Status, model.EnableStatusOn)
	if err != nil {
		return nil, err
	}

	dup, err := s.repo.ExistsBySKUCode(ctx, in.SKUCode, id)
	if err != nil {
		return nil, err
	}
	if dup {
		return nil, errs.ErrConflict.WithMsg("产品编码已存在：" + in.SKUCode)
	}

	fields := map[string]any{
		"sku_code":    in.SKUCode,
		"name_cn":     in.NameCN,
		"name_en":     in.NameEN,
		"category":    in.Category,
		"spec":        in.Spec,
		"unit":        defaultUnit(in.Unit),
		"supplier_id": in.SupplierID,
		"cost_price":  in.CostPrice,
		"sale_price":  in.SalePrice,
		"status":      status,
		"remark":      in.Remark,
	}
	if err := s.repo.Update(ctx, id, fields); err != nil {
		return nil, err
	}
	return s.repo.FindByID(ctx, id)
}

func (s *productService) Delete(ctx context.Context, id uint) error {
	if _, err := s.repo.FindByID(ctx, id); err != nil {
		return err
	}
	return s.repo.Delete(ctx, id)
}

func (s *productService) Get(ctx context.Context, id uint) (*model.Product, error) {
	return s.repo.FindByID(ctx, id)
}

func (s *productService) List(ctx context.Context, q dto.ProductQuery) (*dto.PageResult[model.Product], error) {
	q.PageQuery.Normalize()

	filter := repository.ProductFilter{
		Keyword:    q.Keyword,
		Category:   q.Category,
		SupplierID: q.SupplierID,
		Status:     enableStatusPtr(q.Status),
	}
	page := repository.Pagination{Page: q.Page, PageSize: q.PageSize}

	list, total, err := s.repo.List(ctx, filter, page)
	if err != nil {
		return nil, err
	}
	return dto.NewPageResult(list, total, q.Page, q.PageSize), nil
}

func defaultUnit(unit string) string {
	if unit == "" {
		return "PCS"
	}
	return unit
}
