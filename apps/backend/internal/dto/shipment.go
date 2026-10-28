package dto

// ShipmentQuery 是出货列表的查询条件。
type ShipmentQuery struct {
	PageQuery
	Status *int8 `form:"status"`
	// SalesOrderID 查"这张订单出了几次货"。
	SalesOrderID *uint `form:"sales_order_id"`
}

// ShipmentAllocationInput 是"这票要出哪一行、出多少"。
//
// 挂到销售订单**行**而不是订单头：一张订单里两行可能是同一产品的不同包装，
// 按订单头分配就分不清出的到底是哪一种。
type ShipmentAllocationInput struct {
	SalesOrderItemID uint    `json:"sales_order_item_id" binding:"required"`
	Qty              float64 `json:"qty" binding:"required,gt=0"`

	// 装箱数据，只有印装箱单时才需要；留空就是"还没装箱"，不拦。
	//
	// 重量是这一行的合计（不是单箱），与装箱单上每行的 G.W. / N.W. 对应。
	Cartons     int     `json:"cartons" binding:"gte=0,lte=100000"`
	GrossWeight float64 `json:"gross_weight" binding:"gte=0,lte=1000000"`
	NetWeight   float64 `json:"net_weight" binding:"gte=0,lte=1000000"`
	// Measurement 是外箱尺寸，如 "60×40×30CM"，原样印在装箱单上。
	Measurement string `json:"measurement" binding:"max=64"`

	Remark string `json:"remark" binding:"max=255"`
}

// ShipmentInput 新建与编辑共用。
//
// 没有金额字段：出货本身不收钱也不付钱，费用归集在 T-18 的出货杂费上。
type ShipmentInput struct {
	// ShipDate 出货日，YYYY-MM-DD，留空取今天。
	ShipDate    string                    `json:"ship_date"`
	Remark      string                    `json:"remark" binding:"max=255"`
	Allocations []ShipmentAllocationInput `json:"allocations" binding:"required,min=1,dive"`
}

// ShipmentStatusInput 走完整单据状态机。
type ShipmentStatusInput struct {
	Status int8 `json:"status" binding:"required"`
}

// ShipmentLogisticsInput 只改物流节点，与出货单本身的编辑解耦。
//
// 单独一个入口而不是塞进 ShipmentInput：出货单只有草稿能改，
// 而船名航次与 ETD / ETA 是订舱之后才知道的——那时单子早已确认。
// 把它们塞进 ShipmentInput 的代价是"想记一个提单号，得先有权限改整张单"。
type ShipmentLogisticsInput struct {
	// BLNo 提单号，承运人给的外部号，只记录、不参与发号与唯一性。
	BLNo   string `json:"bl_no" binding:"max=64"`
	Vessel string `json:"vessel" binding:"max=128"`
	// ETD / ETA 用 YYYY-MM-DD，留空表示未定（会把已填的值清掉）。
	ETD string `json:"etd"`
	ETA string `json:"eta"`
}

// ShipmentCandidate 是"还能出货的销售订单行"，出货单选行时用。
//
// 只回状态为已确认 / 备货中、且还有未出数量的行：把已出完的、草稿的、已取消的
// 也都塞进下拉，等于让用户在界面上挑一个必然被 409 拒掉的选项。
type ShipmentCandidate struct {
	SalesOrderID     uint    `json:"sales_order_id"`
	SalesOrderDocNo  string  `json:"sales_order_doc_no"`
	PartnerName      string  `json:"partner_name"`
	SalesOrderItemID uint    `json:"sales_order_item_id"`
	SKUCode          string  `json:"sku_code"`
	NameCN           string  `json:"name_cn"`
	Spec             string  `json:"spec"`
	Unit             string  `json:"unit"`
	Qty              float64 `json:"qty"`
	ShippedQty       float64 `json:"shipped_qty"`
	UnshippedQty     float64 `json:"unshipped_qty"`
	// DeliveryDate 是订单上的约定交期，YYYY-MM-DD；没约定则空。
	DeliveryDate string `json:"delivery_date,omitempty"`
}
