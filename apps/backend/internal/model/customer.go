package model

// Customer 客户（进口商 / 终端门店）。
type Customer struct {
	Base
	Code    string `gorm:"size:32;uniqueIndex;not null" json:"code"`
	Name    string `gorm:"size:128;not null" json:"name"`
	Country string `gorm:"size:64" json:"country"`
	Contact string `gorm:"size:64" json:"contact"`
	Phone   string `gorm:"size:32" json:"phone"`
	Email   string `gorm:"size:128" json:"email"`
	Address string `gorm:"size:255" json:"address"`
	Status  int8   `gorm:"not null;default:1;index" json:"status"`
	Remark  string `gorm:"size:255" json:"remark"`
}

func (Customer) TableName() string { return "md_customer" }
