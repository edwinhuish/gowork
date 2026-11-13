package model

import "time"

// AuditLog 是一次写操作的留痕（T-33）。
//
// 不嵌 Base，是有意的：Base 带着 updated_at 与 deleted_at，那意味着"这条记录能改、
// 能删"。审计日志的价值恰恰在于改不了——仓储因此只提供 Create 与 List，
// 没有 Update / Delete 方法，也没有软删列。
//
// 它记的是**提交意图**：谁、什么时候、对哪张单、提交了什么、结果如何。
// 不是数据库的前后 diff——真 diff 要在每个 service 里读旧值，成本高且必然漏，
// 而"改成了什么"在请求体里已经有了。
type AuditLog struct {
	ID uint `gorm:"primaryKey" json:"id"`

	// ActorID 为 0 表示没认出来是谁（例如令牌在写请求中途失效）。
	ActorID   uint   `gorm:"index;not null;default:0" json:"actor_id"`
	ActorName string `gorm:"size:64;index" json:"actor_name"`
	ActorRole string `gorm:"size:16" json:"actor_role"`

	// Method 是 HTTP 方法，Action 是业务动作：create / update / delete /
	// status / merge / restore / changes……取路由里 :id 之后那一段，
	// 没有就按方法归。
	Method string `gorm:"size:8;not null" json:"method"`
	Action string `gorm:"size:16;index;not null" json:"action"`

	// Entity 是资源名（sales-orders / partners / …），EntityID 是路径上的 :id。
	// 集合级写操作（如重算快照）没有 :id，此时 EntityID 为 0。
	Entity   string `gorm:"size:64;index;not null" json:"entity"`
	EntityID uint   `gorm:"index;not null;default:0" json:"entity_id"`

	Path  string `gorm:"size:255" json:"path"`
	Route string `gorm:"size:160" json:"route"`
	// Status 是最终 HTTP 状态码：400 与 200 都记，失败的操作同样是"发生过的事"。
	Status    int    `gorm:"not null" json:"status"`
	RequestID string `gorm:"size:64;index" json:"request_id"`
	IP        string `gorm:"size:64" json:"ip"`

	// Body 是脱敏、截断后的请求体；非 JSON 或过大时给一句摘要。
	// 存原始请求体的价值在于"当时提交的是什么"，脱敏是为了别把密码抄进日志。
	Body string `gorm:"type:text" json:"body"`

	CreatedAt time.Time `gorm:"index;not null" json:"created_at"`
}

func (AuditLog) TableName() string { return "sys_audit_log" }
