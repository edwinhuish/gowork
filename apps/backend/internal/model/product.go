package model

import "github.com/edwinhuish/gowork/internal/pkg/money"

// Product 产品主数据。
//
// 编码约定参考「分类 + 流水」：SKUCode 全库唯一且永不复用，
// 价格、供应商、包装等易变信息放在独立字段里，不参与编码。
type Product struct {
	Base
	// 与 Base.DeletedAt 同名字段（外层遮蔽内层），只为把它排进复合唯一索引：
	// 嵌入式字段没法按表定制索引。未删除的行 deleted_at 恒为 0，
	// 所以 (sku_code, deleted_at) 表达的就是「未删除的产品里 SKU 唯一」——
	// 删掉一个产品后，同 SKU 可以立刻重建。
	DeletedAt DeletedAt `gorm:"softDelete:milli;not null;default:0;uniqueIndex:uni_product_sku,priority:2" json:"deleted_at,omitempty"`
	SKUCode   string    `gorm:"size:32;uniqueIndex:uni_product_sku,priority:1;not null" json:"sku_code"`
	NameCN   string `gorm:"size:128;not null" json:"name_cn"`
	NameEN   string `gorm:"size:128" json:"name_en"`
	Category string `gorm:"size:32;index" json:"category"`
	Spec     string `gorm:"size:128" json:"spec"`
	Unit     string `gorm:"size:16;not null;default:PCS" json:"unit"`
	// SupplierID 指向 md_partner 里承担供应商角色的主体。
	// 字段名与 JSON tag 保留 supplier 语义：产品关心的是"从谁进的货"，
	// 而不是"这个主体还兼着什么角色"。角色约束由 service 在写入时校验。
	SupplierID *uint `gorm:"index" json:"supplier_id"`
	// 成本价与售价用 money.Decimal：它们是参考价，但会被复制进报价与订单的
	// 成本快照，一旦带了 float 的尾差，两边永远对不上。
	CostPrice money.Decimal `gorm:"type:decimal(12,2);not null;default:0" json:"cost_price"`
	SalePrice money.Decimal `gorm:"type:decimal(12,2);not null;default:0" json:"sale_price"`

	// 以下六项是报关要素的**默认值**，全部可选。
	//
	// HS 编码随目的国与用途变：同一个产品出口到欧盟和美国可能归不同税号，
	// 同一批货里用途不同还要分票申报。所以真实报关以出货批次上填的为准，
	// 产品上这组字段只负责"带出来"——没有它，每票货都要重新查一遍编码，
	// 有它之后只在例外时才改。
	HSCode         string `gorm:"size:16" json:"hs_code"`
	DeclaredNameCN string `gorm:"size:128" json:"declared_name_cn"`
	DeclaredNameEN string `gorm:"size:128" json:"declared_name_en"`
	// Material 记材质与用途。两者合成一个字段而不是两个：报关时它们总是
	// 一起写在同一栏（"不锈钢，厨房用"），拆开反而每次都要判断该填哪个。
	Material      string `gorm:"size:255" json:"material"`
	OriginCountry string `gorm:"size:64" json:"origin_country"`
	// LegalUnit 是法定第一计量单位（千克、台、米），与业务用的 Unit 不同：
	// 报价按「箱」、报关按「千克」是常态，所以不能复用 Unit。
	LegalUnit string `gorm:"size:16" json:"legal_unit"`

	Status EnableStatus `gorm:"not null;default:1;index" json:"status"`
	Remark string       `gorm:"size:255" json:"remark"`

	Supplier *Partner `gorm:"foreignKey:SupplierID" json:"supplier,omitempty"`

	// MatchedCodes 回填「这次搜索命中了哪些编号」，供列表提示用。
	//
	// 不落库也不进详情：它是某一次查询的结果，不是产品的一个字段。
	// 没有它，搜旧编号虽然能搜到，但用户看不出这条为什么会出现在结果里。
	MatchedCodes []string `gorm:"-" json:"matched_codes,omitempty"`
}

func (Product) TableName() string { return "md_product" }
