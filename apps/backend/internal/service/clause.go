package service

import (
	"context"
	"log/slog"
	"strings"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/errs"
	"github.com/edwinhuish/gowork/internal/repository"
)

// ClauseService 管理可复用的合同条款模板（T-27）。
//
// 只管模板本身。合同上的条款是**生成时抄下来的快照**，
// 所以"改模板会不会影响已确认的合同"这个问题的答案是：不会，结构上就不可能。
type ClauseService interface {
	List(ctx context.Context, q dto.ClauseQuery) (*dto.PageResult[model.Clause], error)
	Create(ctx context.Context, in dto.ClauseInput) (*model.Clause, error)
	Update(ctx context.Context, id uint, in dto.ClauseInput) (*model.Clause, error)
	Delete(ctx context.Context, id uint) error
	Restore(ctx context.Context, id uint) (*model.Clause, error)
}

type clauseService struct {
	repo repository.ClauseRepository
	log  *slog.Logger
}

type ClauseOption func(*clauseService)

func WithClauseLogger(l *slog.Logger) ClauseOption {
	return func(s *clauseService) { s.log = l }
}

func NewClauseService(repo repository.ClauseRepository, opts ...ClauseOption) ClauseService {
	s := &clauseService{repo: repo, log: slog.Default()}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *clauseService) List(
	ctx context.Context, q dto.ClauseQuery,
) (*dto.PageResult[model.Clause], error) {
	q.PageQuery.Normalize()

	filter := repository.ClauseFilter{
		Keyword:        q.Keyword,
		Category:       q.Category,
		IncludeDeleted: q.IncludeDeleted,
	}

	list, total, err := s.repo.List(ctx, filter,
		repository.Pagination{Page: q.Page, PageSize: q.PageSize})
	if err != nil {
		return nil, err
	}
	return dto.NewPageResult(list, total, q.Page, q.PageSize), nil
}

func (s *clauseService) Create(ctx context.Context, in dto.ClauseInput) (*model.Clause, error) {
	code := strings.TrimSpace(in.Code)
	title := strings.TrimSpace(in.Title)
	body := strings.TrimSpace(in.Body)
	if code == "" || title == "" || body == "" {
		return nil, errs.BadRequest("编码、标题与正文都不能为空")
	}

	dup, err := s.repo.ExistsByCode(ctx, code, 0)
	if err != nil {
		return nil, err
	}
	if dup {
		return nil, errs.ErrConflict.WithMsg("条款编码已存在：" + code)
	}

	m := &model.Clause{
		Code: code, Title: title, Body: body,
		Category: strings.TrimSpace(in.Category),
		Remark:   in.Remark,
	}
	if err := s.repo.Create(ctx, m); err != nil {
		return nil, err
	}
	s.log.Info("新增条款模板", "id", m.ID, "code", m.Code)
	return m, nil
}

func (s *clauseService) Update(
	ctx context.Context, id uint, in dto.ClauseInput,
) (*model.Clause, error) {
	if _, err := s.repo.FindByID(ctx, id); err != nil {
		return nil, err
	}

	code := strings.TrimSpace(in.Code)
	title := strings.TrimSpace(in.Title)
	body := strings.TrimSpace(in.Body)
	if code == "" || title == "" || body == "" {
		return nil, errs.BadRequest("编码、标题与正文都不能为空")
	}

	dup, err := s.repo.ExistsByCode(ctx, code, id)
	if err != nil {
		return nil, err
	}
	if dup {
		return nil, errs.ErrConflict.WithMsg("条款编码已存在：" + code)
	}

	// 改模板只影响之后生成的合同：已经生成的合同存的是快照。
	if err := s.repo.Update(ctx, id, map[string]any{
		"code": code, "title": title, "body": body,
		"category": strings.TrimSpace(in.Category),
		"remark":   in.Remark,
	}); err != nil {
		return nil, err
	}
	s.log.Info("修改条款模板", "id", id, "code", code)
	return s.repo.FindByID(ctx, id)
}

func (s *clauseService) Delete(ctx context.Context, id uint) error {
	if _, err := s.repo.FindByID(ctx, id); err != nil {
		return err
	}
	// 软删：已经生成的合同不依赖这条模板（存的是快照），但"这条当初从哪来"还要查得到。
	if err := s.repo.Delete(ctx, id); err != nil {
		return err
	}
	s.log.Info("删除条款模板", "id", id)
	return nil
}

func (s *clauseService) Restore(ctx context.Context, id uint) (*model.Clause, error) {
	if err := s.repo.Restore(ctx, id); err != nil {
		return nil, err
	}
	return s.repo.FindByID(ctx, id)
}
