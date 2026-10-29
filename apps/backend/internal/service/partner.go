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
	// Duplicates 扫描全库，按三条规则给出可能重复的主体分组，供合并前清点。
	Duplicates(ctx context.Context) ([]model.PartnerDuplicateGroup, error)
	// DuplicateCandidates 拿一组四要素去比对已有主体，供新建 / 编辑时提示。
	// 只提示不阻断：外贸同名公司确实存在。
	DuplicateCandidates(ctx context.Context, q dto.PartnerDuplicateQuery) ([]model.PartnerDuplicateCandidate, error)
	// Merge 把 in.SourceID 并入 id（主记录）：引用整体迁过去、角色取并集、
	// 被合并方软删。不可逆，所以每一步都要能解释。
	Merge(ctx context.Context, id uint, in dto.PartnerMergeInput) (*dto.PartnerMergeResult, error)
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

	if in.UpdatedAt != nil {
		fields[repository.FieldUpdatedAt] = *in.UpdatedAt
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

// Duplicates 是全库查重：同一批主体按三条规则各分一遍组，命中任意一条的分组
// 都会返回。分组只说明"像"，合不合要人看单据——误合并比漏提示难收拾得多。
func (s *partnerService) Duplicates(ctx context.Context) ([]model.PartnerDuplicateGroup, error) {
	all, err := s.repo.ListAll(ctx)
	if err != nil {
		return nil, err
	}
	return model.GroupPartnerDuplicates(all), nil
}

// DuplicateCandidates 是录入时查重。
//
// 三个要素全空时直接返回空列表，不去拉全表：没有可比的输入，扫一遍既没用，
// 又会让人误以为"系统认为到处都重复"。
func (s *partnerService) DuplicateCandidates(
	ctx context.Context, q dto.PartnerDuplicateQuery,
) ([]model.PartnerDuplicateCandidate, error) {
	probe := model.PartnerProbe{
		Name:      q.Name,
		Country:   q.Country,
		Email:     q.Email,
		Phone:     q.Phone,
		ExcludeID: q.ExcludeID,
	}
	if model.PartnerNameKey(probe.Name, probe.Country) == "" &&
		model.PartnerPhoneKey(probe.Phone) == "" &&
		model.PartnerEmailDomainKey(probe.Email) == "" {
		return []model.PartnerDuplicateCandidate{}, nil
	}

	all, err := s.repo.ListAll(ctx)
	if err != nil {
		return nil, err
	}
	return model.FindPartnerDuplicates(all, probe), nil
}

// Merge 把一条主体并进另一条。
//
// 为什么不是"删掉多余那条、手工把单据改过去"：引用散在十几张表里，
// 手工改必然漏，漏掉的那张单据在界面上表现为客户一片空白，事后没人查得出来。
//
// 方向固定为「并进主记录」：编码、联系人、主记录自己的非空字段都保留，
// 被合并方只在主记录空着的地方补位——合并的目的是"同一家的信息合到一处"，
// 不是"用新的覆盖旧的"。
func (s *partnerService) Merge(
	ctx context.Context, id uint, in dto.PartnerMergeInput,
) (*dto.PartnerMergeResult, error) {
	if in.SourceID == 0 {
		return nil, errs.BadRequest("请指定被合并的主体")
	}
	if in.SourceID == id {
		// 自己并自己会把角色并集之外的字段补成一套，且随后把自己软删——
		// 一次误点的代价是这条主体从列表里消失。
		return nil, errs.BadRequest("不能把主体合并到自己")
	}

	survivor, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	source, err := s.repo.FindByID(ctx, in.SourceID)
	if err != nil {
		if errors.Is(err, errs.ErrNotFound) {
			return nil, errs.BadRequest("被合并的主体不存在或已删除")
		}
		return nil, err
	}

	// 货源冲突先拦：同一产品同一货号在两家各有一条，合并后就是重复录入，
	// 而自动丢掉其中一条等于悄悄改了某张已发出报价的成本依据。
	conflicts, err := s.repo.ProductSourceConflicts(ctx, survivor.ID, source.ID)
	if err != nil {
		return nil, err
	}
	if len(conflicts) > 0 {
		return nil, errs.ErrConflict.WithMsg(fmt.Sprintf(
			"两家在这些产品上都有同一货号的货源，合并后会重复，请先处理其中一条：%s",
			strings.Join(conflicts, "；")))
	}

	fields := mergeFields(survivor, source)
	if in.UpdatedAt != nil {
		fields[repository.FieldUpdatedAt] = *in.UpdatedAt
	}

	moved, err := s.repo.Merge(ctx, repository.PartnerMergePlan{
		SurvivorID: survivor.ID,
		SourceID:   source.ID,
		Fields:     fields,
	})
	if err != nil {
		return nil, err
	}

	merged, err := s.repo.FindByID(ctx, survivor.ID)
	if err != nil {
		return nil, err
	}

	// 留痕：T-33 的审计表还没落地，先落应用日志（与信用放行的处理一致）。
	operator := ""
	if actor, ok := ActorFrom(ctx); ok {
		operator = actor.Username
	}
	s.log.Warn("合并主体",
		"survivor", survivor.Code, "absorbed", source.Code,
		"survivor_role", merged.Role.String(), "operator", operator,
		"moved", moved)

	return &dto.PartnerMergeResult{Partner: merged, Absorbed: source, Moved: moved}, nil
}

// mergeFields 决定合并后主记录长什么样。
//
// 角色永远取并集：合并的意义之一就是"这家既是客户又是供应商"只留一行。
// 字符串字段只在主记录为空时用被合并方的值补上；
// 账期与额度**不动**——它们的零值有业务含义（0 = 款到发货 / 0 = 不限），
// 用被合并方的数填进去，等于悄悄收紧了这家客户的结算条件。
func mergeFields(survivor, source *model.Partner) map[string]any {
	fields := map[string]any{"role": survivor.Role | source.Role}

	fill := func(key, current, incoming string) {
		if strings.TrimSpace(current) == "" && strings.TrimSpace(incoming) != "" {
			fields[key] = incoming
		}
	}
	fill("country", survivor.Country, source.Country)
	fill("contact", survivor.ContactName, source.ContactName)
	fill("phone", survivor.Phone, source.Phone)
	fill("email", survivor.Email, source.Email)
	fill("address", survivor.Address, source.Address)
	fill("remark", survivor.Remark, source.Remark)
	fill("trade_term", string(survivor.TradeTerm), string(source.TradeTerm))
	fill("settlement_method", string(survivor.SettlementMethod), string(source.SettlementMethod))
	fill("currency_code", survivor.CurrencyCode, source.CurrencyCode)
	return fields
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
