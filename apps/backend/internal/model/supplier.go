package model

// Supplier 供应商（1688 店铺或工厂）。
type Supplier struct {
	Base
	Code    string `gorm:"size:32;uniqueIndex;not null" json:"code"`
	Name    string `gorm:"size:128;not null" json:"name"`
	Contact string `gorm:"size:64" json:"contact"`
	Phone   string `gorm:"size:32" json:"phone"`
	Email   string `gorm:"size:128" json:"email"`
	Address string `gorm:"size:255" json:"address"`
	Status  int8   `gorm:"not null;default:1;index" json:"status"`
	Remark  string `gorm:"size:255" json:"remark"`
}

func (Supplier) TableName() string { return "md_supplier" }
