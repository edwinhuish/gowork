package model

// Product 产品主数据。
//
// 编码约定参考「分类 + 流水」：SkuCode 全库唯一且永不复用，
// 价格、供应商、包装等易变信息放在独立字段里，不参与编码。
type Product struct {
	Base
	SkuCode    string       `gorm:"size:32;uniqueIndex;not null" json:"sku_code"`
	NameCN     string       `gorm:"size:128;not null" json:"name_cn"`
	NameEN     string       `gorm:"size:128" json:"name_en"`
	Category   string       `gorm:"size:32;index" json:"category"`
	Spec       string       `gorm:"size:128" json:"spec"`
	Unit       string       `gorm:"size:16;not null;default:PCS" json:"unit"`
	SupplierID *uint        `gorm:"index" json:"supplier_id"`
	CostPrice  float64      `gorm:"type:decimal(12,2);not null;default:0" json:"cost_price"`
	SalePrice  float64      `gorm:"type:decimal(12,2);not null;default:0" json:"sale_price"`
	Status     EnableStatus `gorm:"not null;default:1;index" json:"status"`
	Remark     string       `gorm:"size:255" json:"remark"`

	Supplier *Supplier `gorm:"foreignKey:SupplierID" json:"supplier,omitempty"`
}

func (Product) TableName() string { return "md_product" }
