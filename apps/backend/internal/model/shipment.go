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

	// 报关单号、ETD/ETA、提单号属于 T-19，加在这一层会把两张单据的生命周期绑死。
	Allocations []ShipmentAllocation `gorm:"foreignKey:ShipmentID" json:"allocations,omitempty"`
}

func (Shipment) TableName() string { return "biz_shipment" }

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

	// SalesOrderItem 只用于展示"这行来自哪张订单"（单号在它下面一层），
	// 不参与任何计算。
	SalesOrderItem *SalesOrderItem `gorm:"foreignKey:SalesOrderItemID" json:"sales_order_item,omitempty"`

	// SalesOrderDocNo 是把单号摊平出来给列表与详情用的只读投影。
	// 不让前端去 allocations[].sales_order_item.sales_order.doc_no 里翻：
	// ShipmentAllocation 嵌的 SalesOrderItem 里那个整单是不回吐接口的。
	SalesOrderDocNo string `gorm:"-" json:"sales_order_doc_no"`
}

func (ShipmentAllocation) TableName() string { return "biz_shipment_allocation" }
