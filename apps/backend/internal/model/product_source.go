package model

import "github.com/edwinhuish/gowork/internal/pkg/money"

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
	// SupplierSKUNorm 是 SupplierSKU 的检索形态（见 pkg/code），与 md_product_code.code_norm
	// 用同一个函数。供应商报货号时写的可能是 `A-100` 也可能是 `a 100`，产品列表的关键词
	// 要一次命中，所以两边必须按同一口径归一化。
	//
	// 归一化值落库而不是查询时现算：现算意味着 WHERE 里套函数，索引失效，
	// 三库的函数名还不一样。录入与改货号时由 service 计算。
	SupplierSKUNorm string `gorm:"size:64;index" json:"-"`

	// 采购价按「原币价 + 币种 + 本币价 + 汇率」四件套存（蓝图第五节第 1 条）。
	// 只存原币，比价时得先手工算汇率，而汇率每天都在变——比价结果过一天就不成立。
	// 三个金额字段是 money.Decimal（定点）：货源本币价是报价成本的直接来源，
	// 报价按它比价、毛利按它算，任何末位误差都会被放大到每一张报价上。
	PurchasePrice     money.Decimal `gorm:"type:decimal(14,4);not null;default:0" json:"purchase_price"`
	CurrencyCode      string        `gorm:"size:8;not null" json:"currency_code"`
	PurchasePriceBase money.Decimal `gorm:"type:decimal(14,4);not null;default:0" json:"purchase_price_base"`
	ExchangeRate      money.Decimal `gorm:"type:decimal(18,8);not null;default:1" json:"exchange_rate"`

	// MOQ 最小起订量。0 表示对方没提要求，而不是"不能订"。
	MOQ int `gorm:"not null;default:0" json:"moq"`
	// DeliveryDays 交期天数，从下单起算。0 表示未确认。
	//
	// 叫 delivery 而不是 lead_time：本库里 Lead 已经指 CRM 的线索，
	// lead_time 在搜索与讨论里会被读成"线索时间"。中文仍叫交期。
	DeliveryDays int `gorm:"not null;default:0" json:"delivery_days"`

	Remark string `gorm:"size:255" json:"remark"`

	Product *Product `gorm:"foreignKey:ProductID" json:"product,omitempty"`
	Partner *Partner `gorm:"foreignKey:PartnerID" json:"partner,omitempty"`
}

func (ProductSource) TableName() string { return "md_product_source" }
