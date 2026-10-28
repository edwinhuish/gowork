package dto

// PurchaseOrderQuery 是采购订单列表的查询条件。
type PurchaseOrderQuery struct {
	PageQuery
	SupplierID *uint `form:"supplier_id"`
	Status     *int8 `form:"status"`
}

// PurchaseOrderItemInput 是一条采购明细。
type PurchaseOrderItemInput struct {
	ProductID uint    `json:"product_id" binding:"required"`
	Qty       float64 `json:"qty" binding:"required,gt=0"`
	UnitPrice float64 `json:"unit_price" binding:"gte=0"`
	// SalesOrderItemID 非空表示这一行为该销售订单行采购（以销定采）。
	SalesOrderItemID *uint `json:"sales_order_item_id"`
	// SourceID 指定这行采购对应的货源，仅作追溯，不参与取价：
	// 采购价是谈出来的，不是从货源表里读出来的。
	SourceID *uint  `json:"source_id"`
	Remark   string `json:"remark" binding:"max=255"`
}

// PurchaseOrderInput 新建与编辑共用。
//
// 与销售订单同理：**没有**合计字段。前端把 amount_total 塞进来会被绑定阶段丢弃，
// 合计只从明细汇总一次。
type PurchaseOrderInput struct {
	SupplierID   uint   `json:"supplier_id" binding:"required"`
	TradeTerm    string `json:"trade_term" binding:"required,max=8"`
	CurrencyCode string `json:"currency_code" binding:"max=8"`
	// OrderDate 下单日，留空取今天。
	OrderDate string `json:"order_date"`
	// DeliveryDate 供应商承诺的到货日，可留空。
	DeliveryDate string                   `json:"delivery_date"`
	Remark       string                   `json:"remark" binding:"max=255"`
	Items        []PurchaseOrderItemInput `json:"items" binding:"required,min=1,dive"`
}

// PurchaseOrderStatusInput 走完整单据状态机，具体能不能跳由 service 判定。
type PurchaseOrderStatusInput struct {
	Status int8 `json:"status" binding:"required"`
}
