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

// conn 取这次操作该用的连接：ctx 里带着事务就用事务，否则用池子。
//
// 所有仓储方法都走它，所以"要不要进事务"由调用方在 TxManager 处决定，
// 仓储本身仍然无状态。反过来做（让仓储持有事务副本）就要在每个调用点
// 判断该拿哪个仓储，漏一处就是一次写到事务外面。
func (r *Base[T]) conn(ctx context.Context) *gorm.DB { return dbFor(ctx, r.db) }

func (r *Base[T]) Create(ctx context.Context, m *T) error {
	if err := r.conn(ctx).Create(m).Error; err != nil {
		return fmt.Errorf("创建记录: %w", err)
	}
	return nil
}

// UpdateFields 按字段更新，避免全量覆盖时误清空未提交的字段。
func (r *Base[T]) UpdateFields(ctx context.Context, id uint, fields map[string]any) error {
	if len(fields) == 0 {
		return nil
	}
	res := r.conn(ctx).Model(new(T)).Where("id = ?", id).Updates(fields)
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
	res := r.conn(ctx).Delete(new(T), id)
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
	err := r.conn(ctx).First(&m, id).Error
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
	err := r.conn(ctx).Model(new(T)).Scopes(scopes...).Limit(1).Count(&count).Error
	if err != nil {
		return false, fmt.Errorf("统计记录: %w", err)
	}
	return count > 0, nil
}

func (r *Base[T]) Count(ctx context.Context, scopes ...Scope) (int64, error) {
	var count int64
	if err := r.conn(ctx).Model(new(T)).Scopes(scopes...).Count(&count).Error; err != nil {
		return 0, fmt.Errorf("统计记录: %w", err)
	}
	return count, nil
}

// Paginate 返回当前页数据与总数。
// 计数与取数分别构建查询：复用同一个 *gorm.DB 会让 Count 的 statement 残留到 Find 上。
func (r *Base[T]) Paginate(ctx context.Context, p Pagination, scopes ...Scope) ([]T, int64, error) {
	return r.PaginateOrdered(ctx, p, "id DESC", scopes...)
}

// PaginateOrderedWithPreload 与 PaginateOrdered 相同，额外地预加载关联。
//
// 单据类列表要带明细一起回吐（列表上要显示"几行、共多少数量"）；
// 逐条再查一次明细，会让一页 20 张单变成 21 次往返。
func (r *Base[T]) PaginateOrderedWithPreload(
	ctx context.Context, p Pagination, order string, preloads []string, scopes ...Scope,
) ([]T, int64, error) {
	p = p.normalize()

	var total int64
	if err := r.conn(ctx).Model(new(T)).Scopes(scopes...).Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("统计记录: %w", err)
	}
	if total == 0 {
		return []T{}, 0, nil
	}

	q := r.conn(ctx).Model(new(T)).Scopes(scopes...)
	for _, name := range preloads {
		q = q.Preload(name)
	}

	var list []T
	err := q.Order(order).Offset(p.Offset()).Limit(p.PageSize).Find(&list).Error
	if err != nil {
		return nil, 0, fmt.Errorf("查询记录: %w", err)
	}
	return list, total, nil
}

// PaginateOrdered 与 Paginate 相同，但由调用方指定排序。
//
// 排序必须在这里传，不能写进 scope：gorm 合并 ORDER BY 时后写的会覆盖前面的，
// 而 Paginate 内部的 Order 在 scopes 之后执行——写在 scope 里的排序会被静默吃掉，
// 表现是"列表看着没排序"，但不是每次都看得出来（id 恰好与业务顺序接近时会像是对的）。
func (r *Base[T]) PaginateOrdered(
	ctx context.Context, p Pagination, order string, scopes ...Scope,
) ([]T, int64, error) {
	p = p.normalize()

	var total int64
	if err := r.conn(ctx).Model(new(T)).Scopes(scopes...).Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("统计记录: %w", err)
	}
	if total == 0 {
		return []T{}, 0, nil
	}

	var list []T
	err := r.conn(ctx).Model(new(T)).Scopes(scopes...).
		Order(order).
		Offset(p.Offset()).
		Limit(p.PageSize).
		Find(&list).Error
	if err != nil {
		return nil, 0, fmt.Errorf("查询记录: %w", err)
	}
	return list, total, nil
}
