package model

import (
	"fmt"
	"time"
)

// LeadStatus 是线索状态。
//
// 与商机阶段的区别：线索只回答"这条询盘还在不在跟"，不回答"卖到哪一步了"。
// 转化成功之后线索就结束使命，后续的销售过程在商机里继续。
type LeadStatus int8

const (
	LeadStatusNew       LeadStatus = 10 // 新建，还没联系
	LeadStatusFollowing LeadStatus = 20 // 跟进中
	LeadStatusConverted LeadStatus = 30 // 已转化，指向一个新建的往来单位（硬终态）
	LeadStatusDropped   LeadStatus = 40 // 已废弃，跟不动了
)

func (s LeadStatus) IsValid() bool {
	switch s {
	case LeadStatusNew, LeadStatusFollowing, LeadStatusConverted, LeadStatusDropped:
		return true
	}
	return false
}

// IsFinal 报告是否为硬终态。只有"已转化"是：
// 它已经在 md_partner 里生成了记录，再把它拉回来会让一个主体对应两条线索。
func (s LeadStatus) IsFinal() bool {
	return s == LeadStatusConverted
}

// CanTransitTo 校验状态跳转。
//
// 允许"已废弃"重新激活：客户今年没预算、明年又联系上，是线索阶段最常见的情形；
// 强制新建线索会把来源渠道与最初的询盘内容丢掉。
func (s LeadStatus) CanTransitTo(next LeadStatus) bool {
	if !next.IsValid() || next == s || s.IsFinal() {
		return false
	}
	switch s {
	case LeadStatusNew:
		return next == LeadStatusFollowing || next == LeadStatusDropped || next == LeadStatusConverted
	case LeadStatusFollowing, LeadStatusDropped:
		return next == LeadStatusFollowing || next == LeadStatusDropped || next == LeadStatusConverted
	default:
		return false
	}
}

func (s LeadStatus) Label() string {
	switch s {
	case LeadStatusNew:
		return "新建"
	case LeadStatusFollowing:
		return "跟进中"
	case LeadStatusConverted:
		return "已转化"
	case LeadStatusDropped:
		return "已废弃"
	default:
		return fmt.Sprintf("未知状态(%d)", int8(s))
	}
}

// LeadSource 是询盘来源渠道。
//
// 做成枚举而不是自由文本：来源是投放决策的唯一依据（"展会来的客户成了几单"），
// 自由文本三个月后会同时存在"展会""广交会""Canton Fair"三种写法。
type LeadSource string

const (
	LeadSourceExhibition LeadSource = "EXHIBITION" // 展会
	LeadSourceAlibaba    LeadSource = "ALIBABA"    // 阿里国际站
	LeadSourceGoogle     LeadSource = "GOOGLE"     // 搜索引擎
	LeadSourceReferral   LeadSource = "REFERRAL"   // 老客户推荐
	LeadSourceOther      LeadSource = "OTHER"      // 其它
)

func (s LeadSource) IsValid() bool {
	switch s {
	case LeadSourceExhibition, LeadSourceAlibaba, LeadSourceGoogle, LeadSourceReferral, LeadSourceOther:
		return true
	}
	return false
}

// Lead 是尚未建档的询盘主体。
//
// 为什么不直接建成往来单位：大多数询盘不会成交，全部建进 md_partner 会让
// 客户列表变成"联系人通讯录"，选客户时要从几十个没成交的名字里挑。
// 线索与主体的分界就是"有没有开始正式做生意"。
type Lead struct {
	Base
	Company     string     `gorm:"size:128;not null;index" json:"company"`
	ContactName string     `gorm:"size:64" json:"contact_name"`
	Phone       string     `gorm:"size:32" json:"phone"`
	Email       string     `gorm:"size:128" json:"email"`
	Country     string     `gorm:"size:64" json:"country"`
	Source      LeadSource `gorm:"size:16;not null;index" json:"source"`
	// Demand 客户要什么。转化后不回填到主体上，留在线索里备查。
	Demand string     `gorm:"type:text" json:"demand"`
	Status LeadStatus `gorm:"not null;index" json:"status"`
	// PartnerID 转化后指向新建的往来单位；未转化时为空。
	PartnerID   *uint      `gorm:"index" json:"partner_id"`
	ConvertedAt *time.Time `json:"converted_at"`
	Remark      string     `gorm:"size:255" json:"remark"`

	Partner *Partner `gorm:"foreignKey:PartnerID" json:"partner,omitempty"`
}

func (Lead) TableName() string { return "crm_lead" }
