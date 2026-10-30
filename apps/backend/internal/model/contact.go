package model

// Contact 是往来单位下的具体自然人。
//
// 与 md_partner.contact 的分工：那个列只存"主联系人姓名"，用于列表上一眼看到，
// 不承载第二个人。真正的联系人是这里的多行，各有电话、邮箱、职位、语言。
//
// 为什么值得单独一张表：外贸的对接人会换、公司不会换。换一个人接手，
// 历史跟进要能顺着他查下去，而不是散在邮箱里。一个人一个 Phone 字段的表达
// 不了"这家公司有采购 Maria 和老板 Carlos 两个联系人，一个说西语一个说英语"。
type Contact struct {
	Base
	PartnerID uint   `gorm:"index;not null" json:"partner_id"`
	Name      string `gorm:"size:64;not null" json:"name"`
	// Title 职位（采购经理 / 老板 / 跟单）。
	Title string `gorm:"size:64" json:"title"`
	Phone string `gorm:"size:32" json:"phone"`
	Email string `gorm:"size:128" json:"email"`
	// Language 决定谁去跟这个客户。西语客户配西语业务，转述会丢信息。
	Language string `gorm:"size:32" json:"language"`
	// Channel 偏好联系渠道（WhatsApp / 微信 / 邮件 / 电话）。发错渠道等于没联系。
	Channel string `gorm:"size:32" json:"channel"`
	// IsPrimary 主联系人，一个主体最多一个，由 service 在事务里保证。
	IsPrimary bool   `gorm:"not null;default:false;index" json:"is_primary"`
	Remark    string `gorm:"size:255" json:"remark"`

	Partner *Partner `gorm:"foreignKey:PartnerID" json:"partner,omitempty"`
}

func (Contact) TableName() string { return "crm_partner_contact" }
