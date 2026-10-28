package model

// Supplier 供应商：1688 店铺或工厂。
//
// 归 ERP 域（采购侧），CRM 不引用它。与 Customer 字段高度相似是刻意的：
// 两者都是"商业伙伴"，但当前分开建模，因为客户兼供应商属于少见情况，
// 现在抽 Party 层只会让每条查询多一次关联。真出现双向往来时再收口，
// 见 docs/architecture.md 第七节。
type Supplier struct {
	Base
	Code string `gorm:"size:32;uniqueIndex;not null" json:"code"`
	Name string `gorm:"size:128;not null" json:"name"`
	// ContactName 是主联系人姓名。列名与 JSON 字段保留 contact，理由同 Customer。
	ContactName string       `gorm:"size:64;column:contact" json:"contact"`
	Phone       string       `gorm:"size:32" json:"phone"`
	Email       string       `gorm:"size:128" json:"email"`
	Address     string       `gorm:"size:255" json:"address"`
	Status      EnableStatus `gorm:"not null;default:1;index" json:"status"`
	Remark      string       `gorm:"size:255" json:"remark"`
}

func (Supplier) TableName() string { return "md_supplier" }
