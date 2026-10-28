// Package dto 定义应用层的数据传输契约（请求 / 响应 / 查询条件）。
//
// service 层只依赖 dto，不感知 gin；handler 只做绑定与转发。
package dto

const (
	DefaultPageSize = 20
	MaxPageSize     = 200
)

// PageQuery 是通用分页参数，各业务查询结构体嵌入它即可。
type PageQuery struct {
	Page     int    `form:"page"`
	PageSize int    `form:"page_size"`
	Keyword  string `form:"keyword"`
}

// Normalize 修正非法分页参数，避免越界与全表扫描。
func (q *PageQuery) Normalize() {
	if q.Page < 1 {
		q.Page = 1
	}
	if q.PageSize <= 0 {
		q.PageSize = DefaultPageSize
	}
	if q.PageSize > MaxPageSize {
		q.PageSize = MaxPageSize
	}
}

func (q PageQuery) Offset() int { return (q.Page - 1) * q.PageSize }

// PageResult 是分页响应体。
type PageResult[T any] struct {
	List     []T   `json:"list"`
	Total    int64 `json:"total"`
	Page     int   `json:"page"`
	PageSize int   `json:"page_size"`
}

func NewPageResult[T any](list []T, total int64, page, pageSize int) *PageResult[T] {
	if list == nil {
		list = make([]T, 0)
	}
	return &PageResult[T]{List: list, Total: total, Page: page, PageSize: pageSize}
}
