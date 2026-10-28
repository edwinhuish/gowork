package model

// Product 产品主数据。
//
// 编码约定参考「分类 + 流水」：SKUCode 全库唯一且永不复用，
// 价格、供应商、包装等易变信息放在独立字段里，不参与编码。
type Product struct {
	Base
	SKUCode  string `gorm:"size:32;uniqueIndex;not null" json:"sku_code"`
	NameCN   string `gorm:"size:128;not null" json:"name_cn"`
	NameEN   string `gorm:"size:128" json:"name_en"`
	Category string `gorm:"size:32;index" json:"category"`
	Spec     string `gorm:"size:128" json:"spec"`
	Unit     string `gorm:"size:16;not null;default:PCS" json:"unit"`
	// SupplierID 指向 md_partner 里承担供应商角色的主体。
	// 字段名与 JSON tag 保留 supplier 语义：产品关心的是"从谁进的货"，
	// 而不是"这个主体还兼着什么角色"。角色约束由 service 在写入时校验。
	SupplierID *uint        `gorm:"index" json:"supplier_id"`
	CostPrice  float64      `gorm:"type:decimal(12,2);not null;default:0" json:"cost_price"`
	SalePrice  float64      `gorm:"type:decimal(12,2);not null;default:0" json:"sale_price"`
	Status     EnableStatus `gorm:"not null;default:1;index" json:"status"`
	Remark     string       `gorm:"size:255" json:"remark"`

	Supplier *Partner `gorm:"foreignKey:SupplierID" json:"supplier,omitempty"`
}

func (Product) TableName() string { return "md_product" }
