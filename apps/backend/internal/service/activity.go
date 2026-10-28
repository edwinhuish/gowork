package service

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/errs"
	"github.com/edwinhuish/gowork/internal/repository"
)

var errInvalidActivityType = errs.BadRequest("type 不合法：CALL / EMAIL / VISIT / QUOTE / OTHER")

// ActivityService 管跟进记录。
//
// 它不维护"最近跟进时间"这类冗余列：那个值按需从明细聚合
// （见 docs/trade-erp-crm-blueprint.md 第五节第 3 条）。
// 一旦加列，每次写跟进都要记得更新它，而"记得"不是一种机制。
type ActivityService interface {
	Create(ctx context.Context, in dto.ActivityInput) (*model.Activity, error)
	Update(ctx context.Context, id uint, in dto.ActivityInput) (*model.Activity, error)
	Delete(ctx context.Context, id uint) error
	Get(ctx context.Context, id uint) (*model.Activity, error)
	List(ctx context.Context, q dto.ActivityQuery) (*dto.PageResult[model.Activity], error)
}

type activityService struct {
	repo     repository.ActivityRepository
	partners repository.PartnerRepository
	contacts repository.ContactRepository
	clock    func() time.Time
	log      *slog.Logger
}

type ActivityOption func(*activityService)

func WithActivityLogger(l *slog.Logger) ActivityOption {
	return func(s *activityService) { s.log = l }
}

// WithActivityClock 注入时钟：「已到期」判定用得到，测试也不必等到明天。
func WithActivityClock(now func() time.Time) ActivityOption {
	return func(s *activityService) { s.clock = now }
}

func NewActivityService(
	repo repository.ActivityRepository,
	partners repository.PartnerRepository,
	contacts repository.ContactRepository,
	opts ...ActivityOption,
) ActivityService {
	s := &activityService{
		repo:     repo,
		partners: partners,
		contacts: contacts,
		clock:    time.Now,
		log:      slog.Default(),
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *activityService) Create(ctx context.Context, in dto.ActivityInput) (*model.Activity, error) {
	occurredAt, nextFollowAt, err := s.validate(ctx, in)
	if err != nil {
		return nil, err
	}

	m := &model.Activity{
		PartnerID:    in.PartnerID,
		ContactID:    in.ContactID,
		Type:         model.ActivityType(strings.ToUpper(strings.TrimSpace(in.Type))),
		Subject:      in.Subject,
		Content:      in.Content,
		OccurredAt:   occurredAt,
		NextFollowAt: nextFollowAt,
	}
	if err := s.repo.Create(ctx, m); err != nil {
		return nil, err
	}
	s.log.Info("新增跟进", "id", m.ID, "partner_id", m.PartnerID, "type", m.Type,
		"occurred_at", occurredAt.Format(rateDateLayout))
	return m, nil
}

func (s *activityService) Update(ctx context.Context, id uint, in dto.ActivityInput) (*model.Activity, error) {
	existing, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if in.PartnerID != 0 && in.PartnerID != existing.PartnerID {
		// 跟进记录换了主体，等于把一段历史嫁接给另一家客户。
		return nil, errs.BadRequest("跟进记录不能转移到另一个往来单位，请新建一条")
	}
	occurredAt, nextFollowAt, err := s.validate(ctx, in)
	if err != nil {
		return nil, err
	}

	fields := map[string]any{
		"type":           model.ActivityType(strings.ToUpper(strings.TrimSpace(in.Type))),
		"subject":        in.Subject,
		"content":        in.Content,
		"contact_id":     in.ContactID,
		"occurred_at":    occurredAt,
		"next_follow_at": nextFollowAt,
	}
	if err := s.repo.Update(ctx, id, fields); err != nil {
		return nil, err
	}
	return s.repo.FindByID(ctx, id)
}

func (s *activityService) Delete(ctx context.Context, id uint) error {
	if _, err := s.repo.FindByID(ctx, id); err != nil {
		return err
	}
	return s.repo.Delete(ctx, id)
}

func (s *activityService) Get(ctx context.Context, id uint) (*model.Activity, error) {
	return s.repo.FindByID(ctx, id)
}

func (s *activityService) List(
	ctx context.Context, q dto.ActivityQuery,
) (*dto.PageResult[model.Activity], error) {
	q.PageQuery.Normalize()

	filter := repository.ActivityFilter{
		Keyword:   q.Keyword,
		PartnerID: q.PartnerID,
	}
	if t := strings.ToUpper(strings.TrimSpace(q.Type)); t != "" {
		at := model.ActivityType(t)
		if !at.IsValid() {
			return nil, errInvalidActivityType
		}
		filter.Type = &at
	}
	if q.Due != nil && *q.Due == 1 {
		// 「已到期」按整天比：今天到期的还没算超期，明天才算。
		// 拿"今天"做上界（严格小于），查询发生在上午还是晚上结论都一样。
		filter.NextFollowBefore = model.DateOnly(s.clock()).Format(rateDateLayout)
	}

	page := repository.Pagination{Page: q.Page, PageSize: q.PageSize}
	list, total, err := s.repo.List(ctx, filter, page)
	if err != nil {
		return nil, err
	}
	return dto.NewPageResult(list, total, q.Page, q.PageSize), nil
}

// validate 校验主体、联系人归属、跟进方式与两个日期，返回归一化后的日期。
func (s *activityService) validate(
	ctx context.Context, in dto.ActivityInput,
) (occurredAt time.Time, nextFollowAt *time.Time, err error) {
	at := model.ActivityType(strings.ToUpper(strings.TrimSpace(in.Type)))
	if !at.IsValid() {
		return time.Time{}, nil, errInvalidActivityType
	}

	if _, err := s.partners.FindByID(ctx, in.PartnerID); err != nil {
		if errors.Is(err, errs.ErrNotFound) {
			return time.Time{}, nil, errs.BadRequest("往来单位不存在")
		}
		return time.Time{}, nil, err
	}

	if in.ContactID != nil && *in.ContactID != 0 {
		contact, err := s.contacts.FindByID(ctx, *in.ContactID)
		if err != nil {
			if errors.Is(err, errs.ErrNotFound) {
				return time.Time{}, nil, errs.BadRequest("联系人不存在")
			}
			return time.Time{}, nil, err
		}
		// 联系人不属于这家主体时，这条跟进会同时挂在两家客户名下，
		// 而两家客户的跟进历史都会因此多出一条不属于自己的记录。
		if contact.PartnerID != in.PartnerID {
			return time.Time{}, nil, errs.BadRequest("该联系人不属于所选往来单位")
		}
	}

	occurredAt, err = parseDateOnly(in.OccurredAt)
	if err != nil {
		return time.Time{}, nil, err
	}
	// 不能把跟进记到未来：那会让「最近跟进」显示成还没发生的事。
	if occurredAt.After(model.DateOnly(s.clock())) {
		return time.Time{}, nil, errs.BadRequest("跟进日期不能晚于今天")
	}

	if strings.TrimSpace(in.NextFollowAt) != "" {
		next, err := parseDateOnly(in.NextFollowAt)
		if err != nil {
			return time.Time{}, nil, err
		}
		if next.Before(occurredAt) {
			return time.Time{}, nil, errs.BadRequest("下次跟进日期不能早于本次跟进日期")
		}
		nextFollowAt = &next
	}

	return occurredAt, nextFollowAt, nil
}
