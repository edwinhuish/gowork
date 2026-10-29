package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/errs"
	"github.com/edwinhuish/gowork/internal/pkg/money"
	"github.com/edwinhuish/gowork/internal/repository"
)

var (
	errInvalidPartnerRole = errs.BadRequest("role 不合法：按位组合，1=客户，2=供应商，4=货代")
	errInvalidEntityType  = errs.BadRequest("entity_type 不合法：0=公司，1=个人")
	errInvalidTradeTerm   = errs.BadRequest("trade_term 不合法：EXW / FOB / CFR / CIF / DDP")
	errInvalidSettlement  = errs.BadRequest("settlement_method 不合法：TT / LC / DP / DA / OA")
)

// PartnerService 管理商业伙伴（客户、供应商与货代的统一主体）。
//
// 它同时依赖产品仓储，因为"能不能删这个供应商"不能只看主体表：
// 角色与引用是两处状态，摘角色和删主体都必须先确认没有产品还挂着它。
type PartnerService interface {
	Create(ctx context.Context, in dto.PartnerInput) (*model.Partner, error)
	Update(ctx context.Context, id uint, in dto.PartnerInput) (*model.Partner, error)
	Delete(ctx context.Context, id uint) error
	Get(ctx context.Context, id uint) (*model.Partner, error)
	List(ctx context.Context, q dto.PartnerQuery) (*dto.PageResult[model.Partner], error)
	// Restore 把已软删的往来单位恢复回来，与产品的恢复同口径。
	Restore(ctx context.Context, id uint) (*model.Partner, error)
}

type partnerService struct {
	repo     repository.PartnerRepository
	products repository.ProductRepository
	// currencies 用来校验默认结算币种确实存在。
	// 不校验的话，一个手误的 USD1 会安静地存进去，直到开单时才在折算处报错，
	// 而那时已经在报价环节了。
	currencies repository.CurrencyRepository
	log        *slog.Logger
}

type PartnerOption func(*partnerService)

func WithPartnerLogger(l *slog.Logger) PartnerOption {
	return func(s *partnerService) { s.log = l }
}

func NewPartnerService(
	repo repository.PartnerRepository,
	products repository.ProductRepository,
	currencies repository.CurrencyRepository,
	opts ...PartnerOption,
) PartnerService {
	s := &partnerService{
		repo:       repo,
		products:   products,
		currencies: currencies,
		log:        slog.Default(),
	}
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
		return nil, errs.BadRequest("新建主体必须指定 role：1=客户，2=供应商，4=货代，可叠加")
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
	terms, err := s.normCommercialTerms(ctx, in)
	if err != nil {
		return nil, err
	}

	m := &model.Partner{
		Code:             in.Code,
		Name:             in.Name,
		Role:             role,
		EntityType:       entityType,
		Country:          in.Country,
		ContactName:      in.ContactName,
		Phone:            in.Phone,
		Email:            in.Email,
		Address:          in.Address,
		Status:           status,
		Remark:           in.Remark,
		TradeTerm:        terms.TradeTerm,
		SettlementMethod: terms.SettlementMethod,
		CreditDays:       terms.CreditDays,
		CreditLimit:      terms.CreditLimit,
		CurrencyCode:     terms.CurrencyCode,
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

	// 商务条件只在提交了对应字段时才写库（nil = 这次不改），
	// 否则供应商页面保存一次就会把客户侧的账期清空。
	terms, err := s.normCommercialTerms(ctx, in)
	if err != nil {
		return nil, err
	}
	if in.TradeTerm != nil {
		fields["trade_term"] = string(terms.TradeTerm)
	}
	if in.SettlementMethod != nil {
		fields["settlement_method"] = string(terms.SettlementMethod)
	}
	if in.CreditDays != nil {
		fields["credit_days"] = terms.CreditDays
	}
	if in.CreditLimit != nil {
		fields["credit_limit"] = terms.CreditLimit
	}
	if in.CurrencyCode != nil {
		fields["currency_code"] = terms.CurrencyCode
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

// Restore 把一条已软删的往来单位恢复回来。
//
// 与产品一样只写回标记：恢复不做任何"补数据"的动作，
// 主体删掉期间被改过的产品引用也不会因为它回来而重新指过去。
func (s *partnerService) Restore(ctx context.Context, id uint) (*model.Partner, error) {
	m, err := s.repo.FindByIDIncludingDeleted(ctx, id)
	if err != nil {
		return nil, err
	}
	if m.DeletedAt == 0 {
		return nil, errs.BadRequest("该往来单位未被删除，无需恢复")
	}

	// 先挡一次编码冲突：交给唯一索引只能给出通用的「数据已存在」，
	// 用户看不出是哪个编码被占了。
	occupied, err := s.repo.ExistsByCode(ctx, m.Code, id)
	if err != nil {
		return nil, err
	}
	if occupied {
		return nil, errs.ErrConflict.WithMsg(fmt.Sprintf(
			"编码 %s 已被另一条往来单位占用，先处理掉那条再恢复", m.Code))
	}

	if err := s.repo.Restore(ctx, id); err != nil {
		return nil, err
	}
	return s.repo.FindByID(ctx, id)
}

func (s *partnerService) List(ctx context.Context, q dto.PartnerQuery) (*dto.PageResult[model.Partner], error) {
	q.PageQuery.Normalize()

	filter := repository.PartnerFilter{
		Keyword:        q.Keyword,
		Role:           partnerRolePtr(q.Role),
		Status:         enableStatusPtr(q.Status),
		Country:        q.Country,
		IncludeDeleted: q.IncludeDeleted,
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

// commercialTerms 是校验并归一化之后的商务条件。
//
// nil 入参在这里统一变成零值，由调用方决定"零值要不要写库"：
// 新建时全部写入（零值就是默认值），编辑时只写真正提交过的字段。
type commercialTerms struct {
	TradeTerm        model.TradeTerm
	SettlementMethod model.SettlementMethod
	CreditDays       int
	CreditLimit      money.Decimal
	CurrencyCode     string
}

func (s *partnerService) normCommercialTerms(
	ctx context.Context, in dto.PartnerInput,
) (commercialTerms, error) {
	var out commercialTerms

	if in.TradeTerm != nil {
		term := strings.ToUpper(strings.TrimSpace(*in.TradeTerm))
		if term != "" && !model.TradeTerm(term).IsValid() {
			return out, errInvalidTradeTerm
		}
		out.TradeTerm = model.TradeTerm(term)
	}

	if in.SettlementMethod != nil {
		method := strings.ToUpper(strings.TrimSpace(*in.SettlementMethod))
		if method != "" && !model.SettlementMethod(method).IsValid() {
			return out, errInvalidSettlement
		}
		out.SettlementMethod = model.SettlementMethod(method)
	}

	if in.CreditDays != nil {
		if *in.CreditDays < 0 {
			return out, errs.BadRequest("账期天数不能为负")
		}
		out.CreditDays = *in.CreditDays
	}

	if in.CreditLimit != nil {
		if in.CreditLimit.IsNegative() {
			return out, errs.BadRequest("信用额度不能为负")
		}
		out.CreditLimit = *in.CreditLimit
	}

	if in.CurrencyCode != nil {
		code := strings.ToUpper(strings.TrimSpace(*in.CurrencyCode))
		if code != "" {
			if _, err := s.currencies.FindByCode(ctx, code); err != nil {
				if errors.Is(err, errs.ErrNotFound) {
					return out, errs.BadRequest("默认结算币种不存在：" + code)
				}
				return out, err
			}
		}
		out.CurrencyCode = code
	}

	return out, nil
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
