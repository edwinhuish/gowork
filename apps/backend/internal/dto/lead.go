package dto

import "github.com/edwinhuish/gowork/internal/model"

// LeadQuery 是线索列表的查询条件。
type LeadQuery struct {
	PageQuery
	Status  *int8  `form:"status"`
	Source  string `form:"source"`
	Country string `form:"country"`
	// OpenOnly=1 只看还在跟的（新建与跟进中）。
	OpenOnly *int8 `form:"open_only"`
}

// LeadInput 新建与编辑共用。
//
// Status 用指针：新建时留空即"新建"，编辑时留空表示不改状态。
// 状态变更仍要走状态机（见 model.LeadStatus.CanTransitTo）。
type LeadInput struct {
	Company     string `json:"company" binding:"required,max=128"`
	ContactName string `json:"contact_name" binding:"max=64"`
	Phone       string `json:"phone" binding:"max=32"`
	Email       string `json:"email" binding:"omitempty,email,max=128"`
	Country     string `json:"country" binding:"max=64"`
	Source      string `json:"source" binding:"required,max=16"`
	Demand      string `json:"demand"`
	Status      *int8  `json:"status"`
	Remark      string `json:"remark" binding:"max=255"`
}

// LeadConvertInput 是线索转为主体时的入参。
//
// 编码必须由人来定：主体编码会印在报价单与发票上，自动生成的 `CUS0007`
// 三个月后没人能看出是哪家客户。其余字段从线索带过去，不让人重填一遍。
type LeadConvertInput struct {
	Code string `json:"code" binding:"required,max=32"`
	// Role 默认 1（客户）。极少见的情况是供应商询盘，允许指定 2。
	Role       *int8 `json:"role"`
	EntityType *int8 `json:"entity_type"`
}

// LeadConvertResult 是转化结果。两个实体都返回：
// 前端要刷新线索行（状态变了），也要能跳到刚建好的主体。
type LeadConvertResult struct {
	Lead    *model.Lead    `json:"lead"`
	Partner *model.Partner `json:"partner"`
	// Warning 是同名同国主体的提示。存在也照样建——外贸同名公司确实有，
	// 而且拦住会让人无法把询盘落库；提示出来让人自己判断。
	Warning string `json:"warning,omitempty"`
}
