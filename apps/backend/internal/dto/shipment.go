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
	Remark           string  `json:"remark" binding:"max=255"`
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
