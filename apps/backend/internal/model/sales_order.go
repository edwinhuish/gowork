package model

import "time"

// SalesOrder 是客户下达的订单：报价谈成之后的第一张"要出货"的凭证。
//
// 与报价单的分工：报价单是**我方给出的价**，订单是**双方认下的量**。
// 所以订单上没有成本与毛利快照——成本属于采购（T-12），毛利要等采购价与运杂费落定
// 才能算（T-22）。在这里存一份成本只会变成第二份来源不明的手填数字。
//
// 状态用 DocumentStatus 的完整状态机：草稿 → 已确认 → 备货中 → 已出货 → 已完成，
// 未出货前可取消。不在这里另搞一套订单状态，否则"同样一次取消，报价单和订单各说一套"。
type SalesOrder struct {
	Base
	DocNo string `gorm:"size:32;uniqueIndex;not null" json:"doc_no"`
	// PartnerID 是下单的客户（客户角色，由 service 校验）。
	PartnerID uint           `gorm:"index;not null" json:"partner_id"`
	TradeTerm TradeTerm      `gorm:"size:8;not null" json:"trade_term"`
	Status    DocumentStatus `gorm:"not null;index" json:"status"`

	// OrderDate 是客户下单日（业务日期），决定它落在哪个统计月份。
	OrderDate time.Time `gorm:"type:date;not null;index" json:"order_date"`
	// DeliveryDate 是约定的交期，可空：下单时还没谈定的情况很常见，
	// 而为了"字段非空"编一个日期，会让"能不能按期交"这件事从一开始就失真。
	DeliveryDate *time.Time `gorm:"type:date" json:"delivery_date"`

	CurrencyCode string  `gorm:"size:8;not null" json:"currency_code"`
	ExchangeRate float64 `gorm:"type:decimal(18,8);not null;default:1" json:"exchange_rate"`

	// 合计由明细汇总，写入时算一次并锁住：前端提交的 total 一律不采纳。
	AmountTotal     float64 `gorm:"type:decimal(16,4);not null;default:0" json:"amount_total"`
	AmountBaseTotal float64 `gorm:"type:decimal(16,4);not null;default:0" json:"amount_base_total"`

	Remark string `gorm:"size:255" json:"remark"`

	Partner *Partner         `gorm:"foreignKey:PartnerID" json:"partner,omitempty"`
	Items   []SalesOrderItem `gorm:"foreignKey:SalesOrderID" json:"items,omitempty"`
}

func (SalesOrder) TableName() string { return "biz_sales_order" }

// IsEditable 报告订单还能不能改。已确认的订单是对客户的承诺，
// 改它要留痕（T-24 的变更单），不在本任务里靠"允许编辑"糊过去。
func (o SalesOrder) IsEditable() bool { return o.Status == DocumentStatusDraft }

// SalesOrderItem 是订单明细行。
//
// 产品信息存快照：订单是要发给工厂与客户核对的文件，产品后来改名或改规格，
// 订单上仍应显示当时约定的那个说法。
type SalesOrderItem struct {
	Base
	SalesOrderID uint `gorm:"index;not null" json:"sales_order_id"`
	ProductID    uint `gorm:"index;not null" json:"product_id"`

	SKUCode string  `gorm:"size:32;not null" json:"sku_code"`
	NameCN  string  `gorm:"size:128" json:"name_cn"`
	Spec    string  `gorm:"size:128" json:"spec"`
	Unit    string  `gorm:"size:16" json:"unit"`
	Qty     float64 `gorm:"type:decimal(14,3);not null" json:"qty"`

	UnitPrice  float64 `gorm:"type:decimal(14,4);not null;default:0" json:"unit_price"`
	Amount     float64 `gorm:"type:decimal(16,4);not null;default:0" json:"amount"`
	AmountBase float64 `gorm:"type:decimal(16,4);not null;default:0" json:"amount_base"`

	Remark string `gorm:"size:255" json:"remark"`

	Product *Product `gorm:"foreignKey:ProductID" json:"product,omitempty"`
	// SalesOrder 只用于以销定采的挂接校验（采购单要知道这张销售单还能不能采），
	// 不回吐给接口：明细里再套一张整单会让 payload 变成循环。
	SalesOrder *SalesOrder `gorm:"foreignKey:SalesOrderID" json:"-"`

	// 以下两项是只读投影，不落库（gorm:"-"）：由出货分配算出来，
	// 只在按单取回时填充（详情 / 新建 / 编辑 / 改状态），列表接口不查——
	// 否则一页 20 张单要跑几十次聚合，而列表并不显示这个数。
	ShippedQty   float64 `gorm:"-" json:"shipped_qty"`
	UnshippedQty float64 `gorm:"-" json:"unshipped_qty"`
}

func (SalesOrderItem) TableName() string { return "biz_sales_order_item" }
