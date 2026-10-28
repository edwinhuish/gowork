// Package repository 是数据访问层。
//
// 上层（service）只依赖本包导出的接口，不接触 gorm；
// 具体实现可以随时换成 ent / sqlc 而不影响业务代码。
package repository

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"github.com/edwinhuish/gowork/internal/pkg/errs"
)

// Pagination 是数据访问层的分页入参，与 HTTP 参数解耦。
type Pagination struct {
	Page     int
	PageSize int
}

const (
	defaultPageSize = 20
	maxPageSize     = 200
)

func (p Pagination) normalize() Pagination {
	if p.Page < 1 {
		p.Page = 1
	}
	if p.PageSize <= 0 {
		p.PageSize = defaultPageSize
	}
	if p.PageSize > maxPageSize {
		p.PageSize = maxPageSize
	}
	return p
}

func (p Pagination) Offset() int { return (p.Page - 1) * p.PageSize }

// Scope 是 gorm 条件构造器，把"查询条件"从泛型基类里解耦出去。
// 用类型别名而非命名类型，才能直接展开传给 gorm 的 Scopes(...)。
type Scope = func(*gorm.DB) *gorm.DB

// Base 是泛型仓储基类，承载所有实体的通用 CRUD。
// 具体仓储通过嵌入它复用代码，只实现自己独有的查询方法。
type Base[T any] struct {
	db *gorm.DB
}

func NewBase[T any](db *gorm.DB) *Base[T] { return &Base[T]{db: db} }

// WithTx 返回绑定事务的副本，让仓储保持无状态。
func (r *Base[T]) WithTx(tx *gorm.DB) *Base[T] { return &Base[T]{db: tx} }

// DB 暴露底层连接，仅供本包内的具体仓储做复杂查询。
func (r *Base[T]) DB() *gorm.DB { return r.db }

func (r *Base[T]) Create(ctx context.Context, m *T) error {
	if err := r.db.WithContext(ctx).Create(m).Error; err != nil {
		return fmt.Errorf("创建记录: %w", err)
	}
	return nil
}

// UpdateFields 按字段更新，避免全量覆盖时误清空未提交的字段。
func (r *Base[T]) UpdateFields(ctx context.Context, id uint, fields map[string]any) error {
	if len(fields) == 0 {
		return nil
	}
	res := r.db.WithContext(ctx).Model(new(T)).Where("id = ?", id).Updates(fields)
	if res.Error != nil {
		return fmt.Errorf("更新记录 id=%d: %w", id, res.Error)
	}
	if res.RowsAffected == 0 {
		return errs.ErrNotFound
	}
	return nil
}

// DeleteByID 执行软删除，历史单据与追溯链不受影响。
func (r *Base[T]) DeleteByID(ctx context.Context, id uint) error {
	res := r.db.WithContext(ctx).Delete(new(T), id)
	if res.Error != nil {
		return fmt.Errorf("删除记录 id=%d: %w", id, res.Error)
	}
	if res.RowsAffected == 0 {
		return errs.ErrNotFound
	}
	return nil
}

func (r *Base[T]) FindByID(ctx context.Context, id uint) (*T, error) {
	var m T
	err := r.db.WithContext(ctx).First(&m, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errs.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("查询记录 id=%d: %w", id, err)
	}
	return &m, nil
}

func (r *Base[T]) Exists(ctx context.Context, scopes ...Scope) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(new(T)).Scopes(scopes...).Limit(1).Count(&count).Error
	if err != nil {
		return false, fmt.Errorf("统计记录: %w", err)
	}
	return count > 0, nil
}

func (r *Base[T]) Count(ctx context.Context, scopes ...Scope) (int64, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(new(T)).Scopes(scopes...).Count(&count).Error; err != nil {
		return 0, fmt.Errorf("统计记录: %w", err)
	}
	return count, nil
}

// Paginate 返回当前页数据与总数。
// 计数与取数分别构建查询：复用同一个 *gorm.DB 会让 Count 的 statement 残留到 Find 上。
func (r *Base[T]) Paginate(ctx context.Context, p Pagination, scopes ...Scope) ([]T, int64, error) {
	p = p.normalize()

	var total int64
	if err := r.db.WithContext(ctx).Model(new(T)).Scopes(scopes...).Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("统计记录: %w", err)
	}
	if total == 0 {
		return []T{}, 0, nil
	}

	var list []T
	err := r.db.WithContext(ctx).Model(new(T)).Scopes(scopes...).
		Order("id DESC").
		Offset(p.Offset()).
		Limit(p.PageSize).
		Find(&list).Error
	if err != nil {
		return nil, 0, fmt.Errorf("查询记录: %w", err)
	}
	return list, total, nil
}
