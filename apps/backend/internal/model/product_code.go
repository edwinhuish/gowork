package model

// 产品编号的来源。用字符串而非数字：这些值会出现在列表的提示里，
// 「source=20」谁也看不出那是供应商货号。
const (
	// ProductCodeSourceSKU 是改过 SKU 之后沉淀下来的旧 SKU。
	ProductCodeSourceSKU = "sku"
	// ProductCodeSourceCustomer 是客户自己的物料号（客户下单时写的那串）。
	ProductCodeSourceCustomer = "customer"
	// ProductCodeSourceSupplier 是供应商货号（与 md_product_source.supplier_sku 同源）。
	ProductCodeSourceSupplier = "supplier"
	// ProductCodeSourceFactory 是工厂内部号。
	ProductCodeSourceFactory = "factory"
	// ProductCodeSourceOther 是其余一切：展会号、旧系统号。
	ProductCodeSourceOther = "other"
)

// ProductCode 是产品在 SKU 之外的「别名」：客户物料号、供应商货号、工厂号、旧 SKU。
//
// 为什么单独一张表而不是给产品加几个字段：同一件货可能被三家客户各叫一个号，
// 工厂还有一个内部号；固定几列会在第四个客户出现时立刻不够用。
//
// 为什么 SKU 不放进这张表：SKU 是身份，全库唯一且永不复用；这里是「同一个身份的
// 其他说法」，允许多个产品共用（两家客户从同一个工厂拿货，工厂号相同是常态）。
type ProductCode struct {
	Base
	ProductID uint `gorm:"not null;index;uniqueIndex:uni_product_code,priority:1" json:"product_id"`
	// Code 原样保存，用于显示：客户认的是他写的那串，包括大小写与连字符。
	Code string `gorm:"size:64;not null" json:"code"`
	// CodeNorm 是检索用的归一化形态（见 pkg/code）。
	//
	// 归一化值落库而不是查询时现算：现算意味着 WHERE 里套函数，
	// 索引失效、三库的函数名还不一样。
	CodeNorm string `gorm:"size:64;not null;index;uniqueIndex:uni_product_code,priority:2" json:"code_norm"`
	Source   string `gorm:"size:16;not null;default:other;index" json:"source"`
	Remark   string `gorm:"size:255" json:"remark"`
}

func (ProductCode) TableName() string { return "md_product_code" }
