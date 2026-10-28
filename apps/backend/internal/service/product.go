package service

import (
	"context"
	"errors"
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
	// partners 用来校验 supplier_id 指向的主体确实具备供应商角色。
	// 合并后的主表什么角色都可能，光有外键约束不住"选了个客户当供应商"。
	partners repository.PartnerRepository
	log      *slog.Logger
}

// ProductOption 用于注入可选依赖，保持构造函数签名稳定。
type ProductOption func(*productService)

func WithProductLogger(l *slog.Logger) ProductOption {
	return func(s *productService) { s.log = l }
}

func NewProductService(
	repo repository.ProductRepository,
	partners repository.PartnerRepository,
	opts ...ProductOption,
) ProductService {
	s := &productService{repo: repo, partners: partners, log: slog.Default()}
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
	if err := s.checkSupplier(ctx, in.SupplierID); err != nil {
		return nil, err
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
	if err := s.checkSupplier(ctx, in.SupplierID); err != nil {
		return nil, err
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

// checkSupplier 确认 supplier_id 指向的主体存在且具备供应商角色。
//
// 不校验启用状态：供应商停用后，已经指向它的产品仍然要能被编辑
// （改个售价、改个规格），一刀切拦住会让运营卡在"先改供应商再改产品"。
// 停用的供应商在下拉里已经选不到，新引用进不来，这就够了。
func (s *productService) checkSupplier(ctx context.Context, id *uint) error {
	if id == nil {
		return nil
	}
	p, err := s.partners.FindByID(ctx, *id)
	if err != nil {
		if errors.Is(err, errs.ErrNotFound) {
			return errs.BadRequest("指定的供应商不存在")
		}
		return err
	}
	if !p.Role.Has(model.PartnerRoleSupplier) {
		return errs.BadRequest("所选主体不具备供应商角色：" + p.Name)
	}
	return nil
}
