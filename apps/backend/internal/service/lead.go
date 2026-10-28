package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/errs"
	"github.com/edwinhuish/gowork/internal/repository"
)

var errInvalidLeadSource = errs.BadRequest(
	"source 不合法：EXHIBITION / ALIBABA / GOOGLE / REFERRAL / OTHER")

// LeadService 管线索，并负责把它们转成往来单位。
//
// 转化直接复用 PartnerService.Create，而不是自己往 partner 表里插一行：
// 编码唯一性、角色归一化、商务条件默认值这些规则都在那边，
// 绕过它等于把同一套规则实现两遍，两份迟早分叉。
type LeadService interface {
	Create(ctx context.Context, in dto.LeadInput) (*model.Lead, error)
	Update(ctx context.Context, id uint, in dto.LeadInput) (*model.Lead, error)
	Delete(ctx context.Context, id uint) error
	Get(ctx context.Context, id uint) (*model.Lead, error)
	List(ctx context.Context, q dto.LeadQuery) (*dto.PageResult[model.Lead], error)
	Convert(ctx context.Context, id uint, in dto.LeadConvertInput) (*dto.LeadConvertResult, error)
}

type leadService struct {
	repo     repository.LeadRepository
	partners PartnerService
	partnerQ repository.PartnerRepository
	clock    func() time.Time
	log      *slog.Logger
}

type LeadOption func(*leadService)

func WithLeadLogger(l *slog.Logger) LeadOption {
	return func(s *leadService) { s.log = l }
}

// WithLeadClock 注入时钟：转化时间要可断言，测试不该依赖真实时间。
func WithLeadClock(now func() time.Time) LeadOption {
	return func(s *leadService) { s.clock = now }
}

func NewLeadService(
	repo repository.LeadRepository,
	partners PartnerService,
	partnerQuery repository.PartnerRepository,
	opts ...LeadOption,
) LeadService {
	s := &leadService{
		repo:     repo,
		partners: partners,
		partnerQ: partnerQuery,
		clock:    time.Now,
		log:      slog.Default(),
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *leadService) Create(ctx context.Context, in dto.LeadInput) (*model.Lead, error) {
	source, err := normLeadSource(in.Source)
	if err != nil {
		return nil, err
	}
	status := model.LeadStatusNew
	if in.Status != nil {
		st := model.LeadStatus(*in.Status)
		if !st.IsValid() {
			return nil, errs.BadRequest("status 不合法")
		}
		// 新建的线索不可能是「已转化」——那意味着已经生成了主体，
		// 允许这么建会让线索与主体对不上。
		if st == model.LeadStatusConverted {
			return nil, errs.BadRequest("新建线索不能是「已转化」，请用转化功能")
		}
		status = st
	}

	m := &model.Lead{
		Company:     in.Company,
		ContactName: in.ContactName,
		Phone:       in.Phone,
		Email:       in.Email,
		Country:     in.Country,
		Source:      source,
		Demand:      in.Demand,
		Status:      status,
		Remark:      in.Remark,
	}
	if err := s.repo.Create(ctx, m); err != nil {
		return nil, err
	}
	s.log.Info("新增线索", "id", m.ID, "company", m.Company, "source", m.Source)
	return m, nil
}

func (s *leadService) Update(ctx context.Context, id uint, in dto.LeadInput) (*model.Lead, error) {
	existing, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	source, err := normLeadSource(in.Source)
	if err != nil {
		return nil, err
	}

	fields := map[string]any{
		"company":      in.Company,
		"contact_name": in.ContactName,
		"phone":        in.Phone,
		"email":        in.Email,
		"country":      in.Country,
		"source":       source,
		"demand":       in.Demand,
		"remark":       in.Remark,
	}
	if in.Status != nil && model.LeadStatus(*in.Status) != existing.Status {
		next := model.LeadStatus(*in.Status)
		if !next.IsValid() {
			return nil, errs.BadRequest("status 不合法")
		}
		if next == model.LeadStatusConverted {
			// 转化必须走 Convert：它要建主体、回填 partner_id、写转化时间。
			return nil, errs.BadRequest("请用转化功能把线索转为往来单位，不能直接改状态")
		}
		if !existing.Status.CanTransitTo(next) {
			return nil, errs.ErrConflict.WithMsg(fmt.Sprintf(
				"线索状态不能从「%s」变为「%s」", existing.Status.Label(), next.Label()))
		}
		fields["status"] = next
	}

	if err := s.repo.Update(ctx, id, fields); err != nil {
		return nil, err
	}
	return s.repo.FindByID(ctx, id)
}

func (s *leadService) Delete(ctx context.Context, id uint) error {
	if _, err := s.repo.FindByID(ctx, id); err != nil {
		return err
	}
	return s.repo.Delete(ctx, id)
}

func (s *leadService) Get(ctx context.Context, id uint) (*model.Lead, error) {
	return s.repo.FindByID(ctx, id)
}

func (s *leadService) List(
	ctx context.Context, q dto.LeadQuery,
) (*dto.PageResult[model.Lead], error) {
	q.PageQuery.Normalize()

	filter := repository.LeadFilter{
		Keyword:  q.Keyword,
		Country:  q.Country,
		OpenOnly: q.OpenOnly != nil && *q.OpenOnly == 1,
	}
	if q.Status != nil {
		st := model.LeadStatus(*q.Status)
		if !st.IsValid() {
			return nil, errs.BadRequest("status 不合法")
		}
		filter.Status = &st
	}
	if strings.TrimSpace(q.Source) != "" {
		src, err := normLeadSource(q.Source)
		if err != nil {
			return nil, err
		}
		filter.Source = &src
	}

	page := repository.Pagination{Page: q.Page, PageSize: q.PageSize}
	list, total, err := s.repo.List(ctx, filter, page)
	if err != nil {
		return nil, err
	}
	return dto.NewPageResult(list, total, q.Page, q.PageSize), nil
}

// Convert 把线索转成往来单位。
//
// 顺序是"先建主体、再标记线索"。反过来（先标记）一旦建主体失败，
// 线索就成了"已转化但没有主体"的死记录，比重复建主体更难收拾。
func (s *leadService) Convert(
	ctx context.Context, id uint, in dto.LeadConvertInput,
) (*dto.LeadConvertResult, error) {
	lead, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if lead.Status == model.LeadStatusConverted {
		return nil, errs.ErrConflict.WithMsg("该线索已转化，不能重复转化")
	}

	// 同名同国只提示不阻断：外贸同名公司确实存在，拦住会让人无法把询盘落库。
	warning := ""
	if dup, err := s.partnerQ.FindByNameAndCountry(ctx, lead.Company, lead.Country); err == nil {
		warning = fmt.Sprintf("已存在同名同国的往来单位：%s %s，请确认是否重复建号",
			dup.Code, dup.Name)
	} else if !errors.Is(err, errs.ErrNotFound) {
		return nil, err
	}

	role := int8(model.PartnerRoleCustomer)
	if in.Role != nil {
		role = *in.Role
	}
	var entityType *int8
	if in.EntityType != nil {
		entityType = in.EntityType
	}

	partner, err := s.partners.Create(ctx, dto.PartnerInput{
		Code:        in.Code,
		Name:        lead.Company,
		Role:        &role,
		EntityType:  entityType,
		Country:     lead.Country,
		ContactName: lead.ContactName,
		Phone:       lead.Phone,
		Email:       lead.Email,
	})
	if err != nil {
		return nil, err
	}

	if err := s.repo.MarkConverted(ctx, id, partner.ID, s.clock()); err != nil {
		// 走到这里说明并发下另一个请求先转化成功，本次刚建的主体成了孤儿。
		// 不删它：删错的风险比留一条能看见的重复记录更大。日志里留痕，让人工清理。
		s.log.Error("线索已被并发转化，本次创建的主体成为孤儿",
			"lead_id", id, "partner_id", partner.ID)
		return nil, err
	}

	updated, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	s.log.Info("线索已转化", "lead_id", id, "partner_id", partner.ID, "code", partner.Code)
	return &dto.LeadConvertResult{Lead: updated, Partner: partner, Warning: warning}, nil
}

func normLeadSource(raw string) (model.LeadSource, error) {
	src := model.LeadSource(strings.ToUpper(strings.TrimSpace(raw)))
	if !src.IsValid() {
		return "", errInvalidLeadSource
	}
	return src, nil
}
