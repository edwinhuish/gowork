package model

// Customer 客户主体：外贸链条里的进口商或终端门店。
//
// 它同时被 ERP 与 CRM 引用——ERP 拿它做交易对手（订单、出货、应收），
// CRM 拿它做关系主体（联系人、跟进、商机）。所以本表属于**共享主数据**，
// 归在 md_ 前缀下，不要因为 CRM 相关就搬到 crm_ 去。
type Customer struct {
	Base
	Code    string `gorm:"size:32;uniqueIndex;not null" json:"code"`
	Name    string `gorm:"size:128;not null" json:"name"`
	Country string `gorm:"size:64" json:"country"`
	// ContactName 是主联系人姓名，只有一个人。
	// 完整的联系人（多个、各有电话邮箱）在 CRM 里是独立实体，见 docs/glossary.md。
	// 列名与 JSON 字段都保留 contact：前者为了不破坏既有数据，后者为了不破坏前端契约。
	ContactName string       `gorm:"size:64;column:contact" json:"contact"`
	Phone       string       `gorm:"size:32" json:"phone"`
	Email       string       `gorm:"size:128" json:"email"`
	Address     string       `gorm:"size:255" json:"address"`
	Status      EnableStatus `gorm:"not null;default:1;index" json:"status"`
	Remark      string       `gorm:"size:255" json:"remark"`
}

func (Customer) TableName() string { return "md_customer" }
