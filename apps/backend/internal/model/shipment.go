package model

import "time"

// Shipment 是一次出货（一票）。
//
// 为什么单独建表而不是在订单上加"出货日期"：一张订单可以分三次走（先空运两箱样品、
// 再拼柜、最后尾货），三次各有自己的日期与单据；反过来，一票也可以装三张订单的货
// ——外贸拼柜是常态。所以出货与订单是多对多，"一次出货"必须能独立存在。
type Shipment struct {
	Base
	DocNo  string         `gorm:"size:32;uniqueIndex;not null" json:"doc_no"`
	Status DocumentStatus `gorm:"not null;index" json:"status"`
	// ShipDate 是实际（或计划）出货日，决定它落在哪个统计月份。
	ShipDate time.Time `gorm:"type:date;not null;index" json:"ship_date"`
	Remark   string    `gorm:"size:255" json:"remark"`

	// InvoiceRevision 是这一票商票的改证次数：0 是首版，改一次加一。
	//
	// 改证**不换套号**：报关、提单与客户账上都认这个号，换号等于换了一票货。
	// 版本只体现在文件名与打印件上（`-CI-R1`），这样"客户手上是哪一版"能说清楚。
	InvoiceRevision int `gorm:"not null;default:0" json:"invoice_revision"`

	// 物流节点。这四个值是**订舱之后**才知道的，而出货单在草稿阶段就把数量定死了，
	// 所以它们不跟 ShipmentInput 走：走 `PUT /shipments/:id/logistics`，
	// 任何状态都能改。这与"数量只能在草稿改"是两条相反的规则，理由不同：
	// 数量一变，仓库与货代手里的单就白做了；船期一变，只是把实际情况记下来。
	//
	// BLNo 提单号、Vessel 船名航次由承运人给，是**外部号**：只记录，不参与发号与唯一性。
	BLNo   string     `gorm:"size:64" json:"bl_no"`
	Vessel string     `gorm:"size:128" json:"vessel"`
	ETD    *time.Time `gorm:"type:date" json:"etd"`
	ETA    *time.Time `gorm:"type:date" json:"eta"`

	Allocations  []ShipmentAllocation `gorm:"foreignKey:ShipmentID" json:"allocations,omitempty"`
	Declarations []CustomsDeclaration `gorm:"foreignKey:ShipmentID" json:"declarations,omitempty"`
}

func (Shipment) TableName() string { return "biz_shipment" }

// PackingTotals 汇总整票的箱数与毛净重。
//
// 汇总只由分配行相加得出，不在票头上另存一份：两处数字一旦分家，
// 装箱单的合计与明细就对不上，而客户是按明细逐行点数、按合计签收的。
func (s Shipment) PackingTotals() (cartons int, gross, net float64) {
	for _, a := range s.Allocations {
		cartons += a.Cartons
		gross += a.GrossWeight
		net += a.NetWeight
	}
	return cartons, gross, net
}

// IsEditable 报告这票出货还能不能改。只有草稿能改：
// 已确认的一票已经被仓库与货代按它备货、订舱，改数量等于让他们白干。
func (s Shipment) IsEditable() bool { return s.Status == DocumentStatusDraft }

// ShipmentAllocation 是"这票货里有哪些销售订单行、各多少"。
//
// 它是订单与出货之间的桥：一条销售订单行可以分到多票，一票也可以装多张订单的行。
// 数量校验（不得超过未出数量）只能落在这一层——订单行上的 qty 是"答应客户多少"，
// 而"还能出多少"要看它已经被多少票分走了。
type ShipmentAllocation struct {
	Base
	ShipmentID uint `gorm:"index;not null" json:"shipment_id"`
	// SalesOrderItemID 指向具体那一行销售订单，不是订单头：
	// 一张订单里两行是同一产品的不同包装，按订单头分配就分不清出的是哪一种。
	SalesOrderItemID uint `gorm:"index;not null" json:"sales_order_item_id"`

	// 快照：出货单要发给仓库与货代，产品改名后单上仍是当时那个说法。
	SKUCode string `gorm:"size:32;not null" json:"sku_code"`
	NameCN  string `gorm:"size:128" json:"name_cn"`
	Spec    string `gorm:"size:128" json:"spec"`
	Unit    string `gorm:"size:16" json:"unit"`

	Qty    float64 `gorm:"type:decimal(14,3);not null" json:"qty"`
	Remark string  `gorm:"size:255" json:"remark"`

	// 装箱数据：装箱单靠这几项才印得出来。
	//
	// 挂在分配行而不是票头：一票装多种货时，票头上写着"12 箱 / 240kg"
	// 拆不到任何一行，装箱单每行只能留空，客户也就无法按行核对。
	//
	// 重量是**这一行的合计**，不是单箱重量：合计直接相加就对得上，
	// 若按单箱记，改一次箱数就要把所有合计重算一遍，迟早对不上。
	Cartons     int     `gorm:"not null;default:0" json:"cartons"`
	GrossWeight float64 `gorm:"type:decimal(12,3);not null;default:0" json:"gross_weight"`
	NetWeight   float64 `gorm:"type:decimal(12,3);not null;default:0" json:"net_weight"`
	// Measurement 是外箱尺寸，人工填成 "60×40×30CM"。
	//
	// 存文本而不是长宽高三个数字：装箱单只把它原样印出来，系统不算体积、
	// 也不校验格式——分箱方案要到实际装箱时才定（蓝图 §五.4.4）。
	Measurement string `gorm:"size:64" json:"measurement"`

	// SalesOrderItem 只用于展示"这行来自哪张订单"（单号在它下面一层），
	// 不参与任何计算。
	SalesOrderItem *SalesOrderItem `gorm:"foreignKey:SalesOrderItemID" json:"sales_order_item,omitempty"`

	// SalesOrderDocNo 是把单号摊平出来给列表与详情用的只读投影。
	// 不让前端去 allocations[].sales_order_item.sales_order.doc_no 里翻：
	// ShipmentAllocation 嵌的 SalesOrderItem 里那个整单是不回吐接口的。
	SalesOrderDocNo string `gorm:"-" json:"sales_order_doc_no"`
}

func (ShipmentAllocation) TableName() string { return "biz_shipment_allocation" }
