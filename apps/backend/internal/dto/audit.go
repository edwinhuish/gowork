package dto

// AuditEntry 是中间件交给审计服务落库的一条记录。
//
// 字段全是"中间件手上就有的东西"：它不读库、不拼业务语义，
// 因此记录一条日志的成本是固定的，不会随业务复杂度上升。
type AuditEntry struct {
	ActorID   uint
	ActorName string
	ActorRole string

	Method string
	Action string
	Entity string
	// EntityID 是路径上的 :id；集合级写操作（如重算快照）没有，置 0。
	EntityID uint

	Path   string
	Route  string
	Status int

	RequestID string
	IP        string
	// Body 已由中间件脱敏并截断。
	Body string
}

// AuditLogQuery 是审计日志的查询条件。
type AuditLogQuery struct {
	PageQuery
	ActorID  *uint  `form:"actor_id"`
	Entity   string `form:"entity" binding:"max=64"`
	EntityID *uint  `form:"entity_id"`
	Action   string `form:"action" binding:"max=16"`
	// From / To 按发生时间筛，闭区间（To 含当天）。
	From string `form:"from" binding:"max=10"`
	To   string `form:"to" binding:"max=10"`
	// Keyword 在路径、请求体与操作人里搜：出问题时手里通常只有一个单号。
	Keyword string `form:"keyword" binding:"max=64"`
}
