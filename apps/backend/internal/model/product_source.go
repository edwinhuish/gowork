package model

// ProductSource 产品货源：某产品在某供应商处的采购来源。
//
// 为什么不在产品表上加 supplier_id2 / supplier_id3：一个产品对多个供应商是常态
// （比价、备选、旺季分单），列数一旦按供应商数量定死，第四个供应商就没有位置。
// md_product.supplier_id 保留为**默认货源**，不承担"唯一货源"的语义。
//
// 这张表是报价单的成本来源：报价时按本币价比较，才知道该从谁家进、毛利有多少。
type ProductSource struct {
	Base
	ProductID uint `gorm:"index;not null" json:"product_id"`
	// PartnerID 指向 md_partner 里承担供应商角色的主体。
	// 与 Product.SupplierID 同理，字段名保留 supplier 语义，角色约束由 service 校验。
	PartnerID uint `gorm:"index;not null" json:"partner_id"`
	// SupplierSKU 供应商侧的货号（对方目录里叫什么）。
	//
	// 允许为空：打样阶段对方还没给号，而"先记下这家的报价"本身就有价值。
	// 但同一产品在同一供应商下只能有一条空货号的行——再挂一条就是同一件事录了两次。
	SupplierSKU string `gorm:"size:64;index" json:"supplier_sku"`

	// 采购价按「原币价 + 币种 + 本币价 + 汇率」四件套存（蓝图第五节第 1 条）。
	// 只存原币，比价时得先手工算汇率，而汇率每天都在变——比价结果过一天就不成立。
	PurchasePrice     float64 `gorm:"type:decimal(14,4);not null;default:0" json:"purchase_price"`
	CurrencyCode      string  `gorm:"size:8;not null" json:"currency_code"`
	PurchasePriceBase float64 `gorm:"type:decimal(14,4);not null;default:0" json:"purchase_price_base"`
	ExchangeRate      float64 `gorm:"type:decimal(18,8);not null;default:1" json:"exchange_rate"`

	// MOQ 最小起订量。0 表示对方没提要求，而不是"不能订"。
	MOQ int `gorm:"not null;default:0" json:"moq"`
	// DeliveryDays 交期天数，从下单起算。0 表示未确认。
	//
	// 叫 delivery 而不是 lead_time：本库里 Lead 已经指 CRM 的线索，
	// lead_time 在搜索与讨论里会被读成"线索时间"。中文仍叫交期。
	DeliveryDays int `gorm:"not null;default:0" json:"delivery_days"`

	// Status 刻意不写 default tag：写了之后显式传 0（停用）会被静默存成 1，
	// 停用的货源会重新出现在比价列表里。值由 service 统一归一化。
	Status EnableStatus `gorm:"not null;index" json:"status"`
	Remark string       `gorm:"size:255" json:"remark"`

	Product *Product `gorm:"foreignKey:ProductID" json:"product,omitempty"`
	Partner *Partner `gorm:"foreignKey:PartnerID" json:"partner,omitempty"`
}

func (ProductSource) TableName() string { return "md_product_source" }
