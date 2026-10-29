package dto

// ClauseQuery 是条款模板列表的查询条件。
type ClauseQuery struct {
	PageQuery
	Keyword  string `form:"keyword" binding:"max=64"`
	Category string `form:"category" binding:"max=32"`
	Status   *int8  `form:"status"`
	// IncludeDeleted 把已软删的条款一并列出，这些行只能做一件事：恢复。
	IncludeDeleted bool `form:"include_deleted"`
}

// ClauseInput 是条款模板的新增 / 编辑入参。
type ClauseInput struct {
	Code     string `json:"code" binding:"required,max=32"`
	Title    string `json:"title" binding:"required,max=128"`
	Body     string `json:"body" binding:"required"`
	Category string `json:"category" binding:"max=32"`
	// Status 留空按启用处理。
	Status *int8  `json:"status"`
	Remark string `json:"remark" binding:"max=255"`
}
