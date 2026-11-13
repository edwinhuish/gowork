package model

import "time"

// ActivityType 跟进方式。
//
// 分类型不是为了好看：外贸里"发过邮件"与"打过电话"的说服力完全不同，
// 报价后又沉默了三天，和报价后客户回了"价格太高"，是两种完全不同的处境。
type ActivityType string

const (
	ActivityTypeCall  ActivityType = "CALL"  // 电话
	ActivityTypeEmail ActivityType = "EMAIL" // 邮件
	ActivityTypeVisit ActivityType = "VISIT" // 拜访 / 来访
	ActivityTypeQuote ActivityType = "QUOTE" // 报价（与报价单相互印证）
	ActivityTypeOther ActivityType = "OTHER" // 其它
)

func (t ActivityType) IsValid() bool {
	switch t {
	case ActivityTypeCall, ActivityTypeEmail, ActivityTypeVisit, ActivityTypeQuote, ActivityTypeOther:
		return true
	}
	return false
}

// Activity 是一次跟进记录：电话、邮件、拜访、报价。
//
// 它挂在主体上，可选地指向具体联系人（"跟 Maria 确认了包装"比"跟客户确认了包装"有用得多）。
//
// 为什么 OccurredAt 与 CreatedAt 必须分开：补录上周的电话时，录入时间是今天，
// 但"最近一次跟进"是上周。混用会让"三十天未跟进"的筛选漏掉该跟的客户，
// 也会让刚补录完的客户看起来今天刚联系过。
type Activity struct {
	Base
	PartnerID uint  `gorm:"index;not null" json:"partner_id"`
	ContactID *uint `gorm:"index" json:"contact_id"`
	// OpportunityID 是这条跟进谈的是哪个商机（T-62）。可空：
	// 打给客户的例行回访不属于任何一次询价，硬要求挂商机只会逼人随便选一个。
	OpportunityID *uint        `gorm:"index" json:"opportunity_id"`
	Type          ActivityType `gorm:"size:16;not null;index" json:"type"`
	Subject       string       `gorm:"size:128;not null" json:"subject"`
	Content       string       `gorm:"type:text" json:"content"`
	// OccurredAt 业务发生时间（按日），由录入人指定，默认今天。
	OccurredAt time.Time `gorm:"index;not null" json:"occurred_at"`
	// NextFollowAt 下次跟进日期。为空表示这条线索暂时不需要跟进（例如已成交转售后）。
	// 「超期未跟进」= NextFollowAt 早于今天且记录未删除。
	NextFollowAt *time.Time `gorm:"index" json:"next_follow_at"`

	Partner *Partner `gorm:"foreignKey:PartnerID" json:"partner,omitempty"`
	Contact *Contact `gorm:"foreignKey:ContactID" json:"contact,omitempty"`
}

func (Activity) TableName() string { return "crm_activity" }
