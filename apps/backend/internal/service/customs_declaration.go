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

var errInvalidCustomsStatus = errs.BadRequest("报关状态不合法：10=待申报，20=已申报，30=已放行，90=已退单")

// CustomsDeclarationService 管一票货的报关记录。
//
// **不做状态机。** 改单重报的先后顺序因口岸与报关行而异（有的先退单再改、
// 有的改完直接重报），硬编码一条顺序会在真实操作里挡路；
// 而报关单的状态是海关给出的，不是系统一步步推进的。
// 这里只校验取值合法，顺序由操作的人负责。
type CustomsDeclarationService interface {
	Create(ctx context.Context, in dto.CustomsDeclarationInput) (*model.CustomsDeclaration, error)
	Update(ctx context.Context, id uint, in dto.CustomsDeclarationInput) (*model.CustomsDeclaration, error)
	Delete(ctx context.Context, id uint) error
	Get(ctx context.Context, id uint) (*model.CustomsDeclaration, error)
	List(
		ctx context.Context, q dto.CustomsDeclarationQuery,
	) (*dto.PageResult[model.CustomsDeclaration], error)
}

type customsDeclarationService struct {
	repo      repository.CustomsDeclarationRepository
	shipments repository.ShipmentRepository
	log       *slog.Logger
}

type CustomsDeclarationOption func(*customsDeclarationService)

func WithCustomsDeclarationLogger(l *slog.Logger) CustomsDeclarationOption {
	return func(s *customsDeclarationService) { s.log = l }
}

func NewCustomsDeclarationService(
	repo repository.CustomsDeclarationRepository,
	shipments repository.ShipmentRepository,
	opts ...CustomsDeclarationOption,
) CustomsDeclarationService {
	s := &customsDeclarationService{repo: repo, shipments: shipments, log: slog.Default()}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *customsDeclarationService) Create(
	ctx context.Context, in dto.CustomsDeclarationInput,
) (*model.CustomsDeclaration, error) {
	if _, err := s.shipments.FindByID(ctx, in.ShipmentID); err != nil {
		if errors.Is(err, errs.ErrNotFound) {
			return nil, errs.BadRequest("出货批次不存在")
		}
		return nil, err
	}
	terms, err := buildCustomsTerms(in)
	if err != nil {
		return nil, err
	}

	m := &model.CustomsDeclaration{
		ShipmentID:      in.ShipmentID,
		Status:          terms.Status,
		DeclNo:          terms.DeclNo,
		DeclDate:        terms.DeclDate,
		SupervisionMode: terms.SupervisionMode,
		Remark:          terms.Remark,
	}
	if err := s.repo.Create(ctx, m); err != nil {
		return nil, err
	}
	s.log.Info("新增报关单", "id", m.ID, "shipment_id", m.ShipmentID,
		"decl_no", m.DeclNo, "status", m.Status.Label())
	return s.repo.FindByID(ctx, m.ID)
}

func (s *customsDeclarationService) Update(
	ctx context.Context, id uint, in dto.CustomsDeclarationInput,
) (*model.CustomsDeclaration, error) {
	existing, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	// 换一票货等于这条报关记录描述的是另一次出口，两票的单证都会因此对不上。
	// 让用户在新票下新建一条。
	if in.ShipmentID != 0 && in.ShipmentID != existing.ShipmentID {
		return nil, errs.BadRequest("报关单不能改挂到另一票货，请在新票下新建")
	}
	terms, err := buildCustomsTerms(in)
	if err != nil {
		return nil, err
	}

	fields := map[string]any{
		"status":           terms.Status,
		"decl_no":          terms.DeclNo,
		"decl_date":        terms.DeclDate,
		"supervision_mode": terms.SupervisionMode,
		"remark":           terms.Remark,
	}
	if err := s.repo.Update(ctx, id, fields); err != nil {
		return nil, err
	}
	return s.repo.FindByID(ctx, id)
}

func (s *customsDeclarationService) Delete(ctx context.Context, id uint) error {
	return s.repo.Delete(ctx, id)
}

func (s *customsDeclarationService) Get(
	ctx context.Context, id uint,
) (*model.CustomsDeclaration, error) {
	return s.repo.FindByID(ctx, id)
}

func (s *customsDeclarationService) List(
	ctx context.Context, q dto.CustomsDeclarationQuery,
) (*dto.PageResult[model.CustomsDeclaration], error) {
	q.PageQuery.Normalize()

	filter := repository.CustomsDeclarationFilter{ShipmentID: q.ShipmentID}
	if q.Status != nil {
		st := model.CustomsStatus(*q.Status)
		filter.Status = &st
	}
	page := repository.Pagination{Page: q.Page, PageSize: q.PageSize}

	list, total, err := s.repo.List(ctx, filter, page)
	if err != nil {
		return nil, err
	}
	return dto.NewPageResult(list, total, q.Page, q.PageSize), nil
}

// customsTerms 是校验之后的报关单字段。
type customsTerms struct {
	Status          model.CustomsStatus
	DeclNo          string
	DeclDate        *time.Time
	SupervisionMode string
	Remark          string
}

func buildCustomsTerms(in dto.CustomsDeclarationInput) (customsTerms, error) {
	var out customsTerms

	st := model.CustomsStatus(in.Status)
	if !st.IsValid() {
		return out, errInvalidCustomsStatus
	}
	declDate, err := parseOptionalDateOnly(in.DeclDate)
	if err != nil {
		return out, errs.BadRequest("申报日期格式应为 YYYY-MM-DD")
	}

	out = customsTerms{
		Status:          st,
		DeclNo:          strings.TrimSpace(in.DeclNo),
		DeclDate:        declDate,
		SupervisionMode: strings.TrimSpace(in.SupervisionMode),
		Remark:          in.Remark,
	}
	return out, nil
}

// parseOptionalDateOnly 解析可空的日期。空串返回 nil（表示"还没定/清空"），
// 而不是零值时间——零值会存成 0001-01-01，在列表里显示成一个荒谬的日期。
func parseOptionalDateOnly(raw string) (*time.Time, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	d, err := parseDateOnly(raw)
	if err != nil {
		return nil, err
	}
	return &d, nil
}
