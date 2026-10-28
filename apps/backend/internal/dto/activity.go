package dto

// ActivityQuery 是跟进记录的查询条件。
type ActivityQuery struct {
	PageQuery
	PartnerID *uint  `form:"partner_id"`
	Type      string `form:"type"`
	// Due=1 时只取「已到期未跟进」：NextFollowAt 早于今天。
	Due *int8 `form:"due"`
}

// ActivityInput 新建与编辑共用。
//
// 日期一律用 YYYY-MM-DD 字符串：跟进是"哪天跟的"，不是"几点几分跟的"，
// 传时间戳会让时区把它挪一天。
type ActivityInput struct {
	PartnerID uint   `json:"partner_id" binding:"required"`
	ContactID *uint  `json:"contact_id"`
	Type      string `json:"type" binding:"required,max=16"`
	Subject   string `json:"subject" binding:"required,max=128"`
	Content   string `json:"content"`
	// OccurredAt 业务发生日期。补录时必须填实际那天，不能默认今天。
	OccurredAt string `json:"occurred_at" binding:"required,max=32"`
	// NextFollowAt 下次跟进日期，留空表示不需要再跟。
	NextFollowAt string `json:"next_follow_at" binding:"max=32"`
}
