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

// ContactService 管往来单位下的联系人。
type ContactService interface {
	Create(ctx context.Context, in dto.ContactInput) (*model.Contact, error)
	Update(ctx context.Context, id uint, in dto.ContactInput) (*model.Contact, error)
	Delete(ctx context.Context, id uint) error
	Get(ctx context.Context, id uint) (*model.Contact, error)
	List(ctx context.Context, q dto.ContactQuery) (*dto.PageResult[model.Contact], error)
}

type contactService struct {
	repo     repository.ContactRepository
	partners repository.PartnerRepository
	log      *slog.Logger
}

type ContactOption func(*contactService)

func WithContactLogger(l *slog.Logger) ContactOption {
	return func(s *contactService) { s.log = l }
}

func NewContactService(
	repo repository.ContactRepository,
	partners repository.PartnerRepository,
	opts ...ContactOption,
) ContactService {
	s := &contactService{repo: repo, partners: partners, log: slog.Default()}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *contactService) Create(ctx context.Context, in dto.ContactInput) (*model.Contact, error) {
	status, err := normEnableStatus(in.Status, model.EnableStatusOn)
	if err != nil {
		return nil, err
	}
	// 校验主体存在：联系人不挂主体就是一张通讯录，
	// 而 CRM 的整个价值在于"这个人属于哪家、跟到哪一步"。
	if _, err := s.partners.FindByID(ctx, in.PartnerID); err != nil {
		if errors.Is(err, errs.ErrNotFound) {
			return nil, errs.BadRequest("往来单位不存在")
		}
		return nil, err
	}

	m := &model.Contact{
		PartnerID: in.PartnerID,
		Name:      in.Name,
		Title:     in.Title,
		Phone:     in.Phone,
		Email:     in.Email,
		Language:  in.Language,
		Channel:   in.Channel,
		Status:    status,
		Remark:    in.Remark,
	}
	if err := s.repo.Create(ctx, m); err != nil {
		return nil, err
	}

	// 先落库再设主联系人：SetPrimary 需要 id 才能"清掉别人、置上自己"。
	if in.IsPrimary != nil && *in.IsPrimary {
		if err := s.repo.SetPrimary(ctx, m.PartnerID, m.ID); err != nil {
			return nil, err
		}
		m.IsPrimary = true
	}

	s.log.Info("新增联系人", "id", m.ID, "partner_id", m.PartnerID, "name", m.Name)
	return m, nil
}

func (s *contactService) Update(ctx context.Context, id uint, in dto.ContactInput) (*model.Contact, error) {
	existing, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if in.PartnerID != 0 && in.PartnerID != existing.PartnerID {
		// 挪主体会让这个人的历史跟进挂到别人名下，而跟进记录本身不知情。
		return nil, errs.BadRequest("联系人不能转移到另一个往来单位，请新建联系人并停用旧的")
	}

	status, err := normEnableStatus(in.Status, model.EnableStatusOn)
	if err != nil {
		return nil, err
	}

	fields := map[string]any{
		"name":     in.Name,
		"title":    in.Title,
		"phone":    in.Phone,
		"email":    in.Email,
		"language": in.Language,
		"channel":  in.Channel,
		"status":   status,
		"remark":   in.Remark,
	}

	if in.IsPrimary != nil {
		switch {
		case *in.IsPrimary && !existing.IsPrimary:
			if err := s.repo.SetPrimary(ctx, existing.PartnerID, id); err != nil {
				return nil, err
			}
		case !*in.IsPrimary && existing.IsPrimary:
			// 与币种的本币同一规则：允许"换一个主联系人"，不允许"把主联系人摘掉"。
			return nil, errs.ErrConflict.WithMsg("不能取消主联系人标记，请改把另一个联系人设为主联系人")
		}
	}

	if err := s.repo.Update(ctx, id, fields); err != nil {
		return nil, err
	}
	return s.repo.FindByID(ctx, id)
}

func (s *contactService) Delete(ctx context.Context, id uint) error {
	if _, err := s.repo.FindByID(ctx, id); err != nil {
		return err
	}
	// 主联系人被删后不自动改选：剩下的几个人里谁是对接人，
	// 是业务判断，不该由"谁先建的那条"决定。
	return s.repo.Delete(ctx, id)
}

func (s *contactService) Get(ctx context.Context, id uint) (*model.Contact, error) {
	return s.repo.FindByID(ctx, id)
}

func (s *contactService) List(ctx context.Context, q dto.ContactQuery) (*dto.PageResult[model.Contact], error) {
	q.PageQuery.Normalize()

	filter := repository.ContactFilter{
		Keyword:   q.Keyword,
		PartnerID: q.PartnerID,
		Status:    enableStatusPtr(q.Status),
	}
	page := repository.Pagination{Page: q.Page, PageSize: q.PageSize}

	list, total, err := s.repo.List(ctx, filter, page)
	if err != nil {
		return nil, err
	}
	return dto.NewPageResult(list, total, q.Page, q.PageSize), nil
}
