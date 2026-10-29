package repository

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/edwinhuish/gowork/internal/model"
)

// AuditFilter 是审计日志的查询条件。全部可选，留空即不过滤。
type AuditFilter struct {
	ActorID  *uint
	Entity   string
	EntityID *uint
	Action   string
	// From / To 按**发生时间**筛，闭区间。
	From, To *time.Time
	// Keyword 在路径与请求体里搜：出问题时人手里通常只有一个单号或一个 id。
	Keyword string
}

// AuditLogRepository 是审计日志的持久化入口。
//
// **只有 Create 与 List**：没有 Update、没有 Delete、没有软删列。
// "日志不可改"不是靠权限检查守住的，而是这一层根本没有提供改的方法——
// 哪天有人要加一个 UpdateAuditLog，它必须先改这个接口，改动会出现在评审里。
// 也正因如此，它不嵌 Base：Base 自带 updated_at / deleted_at，语义上意味着"能改能删"。
type AuditLogRepository interface {
	Create(ctx context.Context, entry *model.AuditLog) error
	List(ctx context.Context, f AuditFilter, p Pagination) ([]model.AuditLog, int64, error)
}

type auditLogRepo struct {
	db *gorm.DB
}

func NewAuditLogRepository(db *gorm.DB) AuditLogRepository {
	return &auditLogRepo{db: db}
}

func (r *auditLogRepo) Create(ctx context.Context, entry *model.AuditLog) error {
	if err := dbFor(ctx, r.db).Create(entry).Error; err != nil {
		return fmt.Errorf("写入审计日志: %w", err)
	}
	return nil
}

func (r *auditLogRepo) List(
	ctx context.Context, f AuditFilter, p Pagination,
) ([]model.AuditLog, int64, error) {
	p = p.normalize()

	// 两次查询的条件必须一致（先 count 再取页），所以收成一个闭包：
	// 复制一份条件出来改，迟早出现"count 的与查出来的不是同一批"。
	base := func() *gorm.DB {
		q := dbFor(ctx, r.db).Model(&model.AuditLog{})
		if f.ActorID != nil {
			q = q.Where("actor_id = ?", *f.ActorID)
		}
		if f.Entity != "" {
			q = q.Where("entity = ?", f.Entity)
		}
		if f.EntityID != nil {
			q = q.Where("entity_id = ?", *f.EntityID)
		}
		if f.Action != "" {
			q = q.Where("action = ?", f.Action)
		}
		if f.From != nil {
			q = q.Where("created_at >= ?", *f.From)
		}
		if f.To != nil {
			// 传进来的是某天零点，闭区间要包住那一整天。
			q = q.Where("created_at < ?", f.To.Add(24*time.Hour))
		}
		if f.Keyword != "" {
			q = q.Scopes(likeAny(f.Keyword, "path", "body", "actor_name"))
		}
		return q
	}

	var total int64
	if err := base().Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("统计审计日志: %w", err)
	}

	logs := make([]model.AuditLog, 0, p.PageSize)
	if err := base().
		Order("id DESC").
		Offset(p.Offset()).
		Limit(p.PageSize).
		Find(&logs).Error; err != nil {
		return nil, 0, fmt.Errorf("查询审计日志: %w", err)
	}
	return logs, total, nil
}
