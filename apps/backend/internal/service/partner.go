package service

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/errs"
	"github.com/edwinhuish/gowork/internal/repository"
)

var (
	errInvalidPartnerRole = errs.BadRequest("role 不合法：1=客户，2=供应商，3=两者")
	errInvalidEntityType  = errs.BadRequest("entity_type 不合法：0=公司，1=个人")
)

// PartnerService 管理商业伙伴（客户与供应商的统一主体）。
//
// 它同时依赖产品仓储，因为"能不能删这个供应商"不能只看主体表：
// 角色与引用是两处状态，摘角色和删主体都必须先确认没有产品还挂着它。
type PartnerService interface {
	Create(ctx context.Context, in dto.PartnerInput) (*model.Partner, error)
	Update(ctx context.Context, id uint, in dto.PartnerInput) (*model.Partner, error)
	Delete(ctx context.Context, id uint) error
	Get(ctx context.Context, id uint) (*model.Partner, error)
	List(ctx context.Context, q dto.PartnerQuery) (*dto.PageResult[model.Partner], error)
}

type partnerService struct {
	repo     repository.PartnerRepository
	products repository.ProductRepository
	log      *slog.Logger
}

type PartnerOption func(*partnerService)

func WithPartnerLogger(l *slog.Logger) PartnerOption {
	return func(s *partnerService) { s.log = l }
}

func NewPartnerService(
	repo repository.PartnerRepository,
	products repository.ProductRepository,
	opts ...PartnerOption,
) PartnerService {
	s := &partnerService{repo: repo, products: products, log: slog.Default()}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *partnerService) Create(ctx context.Context, in dto.PartnerInput) (*model.Partner, error) {
	role, hasRole, err := normPartnerRole(in.Role)
	if err != nil {
		return nil, err
	}
	if !hasRole {
		// 角色决定这个主体出现在哪个业务的候选列表里，没角色的记录建出来就找不到了。
		return nil, errs.BadRequest("新建主体必须指定 role：1=客户，2=供应商，3=两者")
	}
	entityType, hasEntityType, err := normEntityType(in.EntityType)
	if err != nil {
		return nil, err
	}
	if !hasEntityType {
		entityType = model.PartnerEntityCompany
	}
	status, err := normEnableStatus(in.Status, model.EnableStatusOn)
	if err != nil {
		return nil, err
	}
	dup, err := s.repo.ExistsByCode(ctx, in.Code, 0)
	if err != nil {
		return nil, err
	}
	if dup {
		return nil, errs.ErrConflict.WithMsg("主体编码已存在：" + in.Code)
	}

	m := &model.Partner{
		Code:        in.Code,
		Name:        in.Name,
		Role:        role,
		EntityType:  entityType,
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
	s.log.Info("新增主体", "id", m.ID, "code", m.Code, "role", m.Role.String())
	return m, nil
}

func (s *partnerService) Update(ctx context.Context, id uint, in dto.PartnerInput) (*model.Partner, error) {
	existing, err := s.repo.FindByID(ctx, id)
	if err != nil {
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
		return nil, errs.ErrConflict.WithMsg("主体编码已存在：" + in.Code)
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

	role, hasRole, err := normPartnerRole(in.Role)
	if err != nil {
		return nil, err
	}
	if hasRole {
		// 摘掉 supplier 角色等同于"这个主体不再是供应商"，而产品还挂着它时
		// 就会留下一条指向非供应商产品的引用。先拦下来，让人去改产品。
		if existing.Role.Has(model.PartnerRoleSupplier) && !role.Has(model.PartnerRoleSupplier) {
			used, err := s.products.CountBySupplierID(ctx, id)
			if err != nil {
				return nil, err
			}
			if used > 0 {
				return nil, errs.ErrConflict.WithMsg(fmt.Sprintf(
					"该主体仍被 %d 个产品引用为供应商，请先调整这些产品的供应商", used))
			}
		}
		fields["role"] = role
	}

	entityType, hasEntityType, err := normEntityType(in.EntityType)
	if err != nil {
		return nil, err
	}
	if hasEntityType {
		fields["entity_type"] = entityType
	}

	if err := s.repo.Update(ctx, id, fields); err != nil {
		return nil, err
	}
	return s.repo.FindByID(ctx, id)
}

func (s *partnerService) Delete(ctx context.Context, id uint) error {
	existing, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return err
	}
	if existing.Role.Has(model.PartnerRoleSupplier) {
		used, err := s.products.CountBySupplierID(ctx, id)
		if err != nil {
			return err
		}
		if used > 0 {
			return errs.ErrConflict.WithMsg(fmt.Sprintf(
				"该供应商仍被 %d 个产品引用，请先调整这些产品的供应商", used))
		}
	}
	return s.repo.Delete(ctx, id)
}

func (s *partnerService) Get(ctx context.Context, id uint) (*model.Partner, error) {
	return s.repo.FindByID(ctx, id)
}

func (s *partnerService) List(ctx context.Context, q dto.PartnerQuery) (*dto.PageResult[model.Partner], error) {
	q.PageQuery.Normalize()

	filter := repository.PartnerFilter{
		Keyword: q.Keyword,
		Role:    partnerRolePtr(q.Role),
		Status:  enableStatusPtr(q.Status),
		Country: q.Country,
	}
	if q.EntityType != nil {
		t := model.PartnerEntityType(*q.EntityType)
		filter.EntityType = &t
	}
	page := repository.Pagination{Page: q.Page, PageSize: q.PageSize}

	list, total, err := s.repo.List(ctx, filter, page)
	if err != nil {
		return nil, err
	}
	return dto.NewPageResult(list, total, q.Page, q.PageSize), nil
}

// normPartnerRole 校验角色位。has 为 false 表示调用方没提交这个字段。
func normPartnerRole(p dto.PartnerRoleInput) (role model.PartnerRole, has bool, err error) {
	if p == nil {
		return 0, false, nil
	}
	role = model.PartnerRole(*p)
	if !role.IsValid() {
		return 0, false, errInvalidPartnerRole
	}
	return role, true, nil
}

// normEntityType 校验主体类型。has 为 false 表示调用方没提交这个字段。
func normEntityType(p dto.PartnerEntityTypeInput) (t model.PartnerEntityType, has bool, err error) {
	if p == nil {
		return 0, false, nil
	}
	t = model.PartnerEntityType(*p)
	if !t.IsValid() {
		return 0, false, errInvalidEntityType
	}
	return t, true, nil
}

// partnerRolePtr 把 dto 的可选角色转成仓储层用的枚举指针。
//
// 列表筛选不校验取值：传一个未定义的角色位，展开出来的候选集合为空，
// 结果是空列表 —— 这比报错更符合"筛选"的语义（没有这种记录）。
func partnerRolePtr(p dto.PartnerRoleInput) *model.PartnerRole {
	if p == nil {
		return nil
	}
	r := model.PartnerRole(*p)
	return &r
}
