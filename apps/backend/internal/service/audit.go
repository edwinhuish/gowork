package service

import (
	"context"
	"log/slog"
	"time"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/repository"
)

// AuditService 是操作日志的写入口与查询入口（T-33）。
//
// 写由中间件统一调用一次，业务代码不需要、也不应该自己调：
// 让每个 service 记得"改完记得记一条"，结果一定是漏的，而漏的那类单据
// 恰好会是下次出事时要查的那一类。
type AuditService interface {
	// Record 落一条审计。失败只返回错误，由调用方决定怎么处理——
	// 中间件不会让它拦住业务请求。
	Record(ctx context.Context, entry dto.AuditEntry) error
	// List 分页查询，按发生时间倒序（最近的在最上面）。
	List(ctx context.Context, q dto.AuditLogQuery) (*dto.PageResult[model.AuditLog], error)
}

type auditService struct {
	repo  repository.AuditLogRepository
	clock func() time.Time
	log   *slog.Logger
}

type AuditOption func(*auditService)

func WithAuditClock(now func() time.Time) AuditOption {
	return func(s *auditService) { s.clock = now }
}

func WithAuditLogger(l *slog.Logger) AuditOption {
	return func(s *auditService) { s.log = l }
}

func NewAuditService(
	repo repository.AuditLogRepository, opts ...AuditOption,
) AuditService {
	s := &auditService{repo: repo, clock: time.Now, log: slog.Default()}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *auditService) Record(ctx context.Context, entry dto.AuditEntry) error {
	row := &model.AuditLog{
		ActorID:   entry.ActorID,
		ActorName: entry.ActorName,
		ActorRole: entry.ActorRole,
		Method:    entry.Method,
		Action:    entry.Action,
		Entity:    entry.Entity,
		EntityID:  entry.EntityID,
		Path:      entry.Path,
		Route:     entry.Route,
		Status:    entry.Status,
		RequestID: entry.RequestID,
		IP:        entry.IP,
		Body:      entry.Body,
		CreatedAt: s.clock(),
	}
	if err := s.repo.Create(ctx, row); err != nil {
		// 审计写失败不能回滚已经成功的业务：这里只报错给调用方，
		// 由中间件记一条 warn —— "日志少了一条"必须能在外层被看见。
		s.log.Warn("写审计日志失败", "entity", entry.Entity, "action", entry.Action,
			"request_id", entry.RequestID, "err", err)
		return err
	}
	return nil
}

func (s *auditService) List(
	ctx context.Context, q dto.AuditLogQuery,
) (*dto.PageResult[model.AuditLog], error) {
	q.PageQuery.Normalize()

	filter := repository.AuditFilter{
		ActorID:  q.ActorID,
		Entity:   q.Entity,
		EntityID: q.EntityID,
		Action:   q.Action,
		Keyword:  q.Keyword,
	}
	// 两个日期各解析一次：From 取当天零点，To 由仓储按"包含当天"处理。
	if q.From != "" {
		d, err := parseDateOnly(q.From)
		if err != nil {
			return nil, err
		}
		filter.From = &d
	}
	if q.To != "" {
		d, err := parseDateOnly(q.To)
		if err != nil {
			return nil, err
		}
		filter.To = &d
	}

	page := repository.Pagination{Page: q.Page, PageSize: q.PageSize}
	logs, total, err := s.repo.List(ctx, filter, page)
	if err != nil {
		return nil, err
	}
	return dto.NewPageResult(logs, total, q.Page, q.PageSize), nil
}
