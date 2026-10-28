package dto

// SalesOrderQuery 是销售订单列表的查询条件。
type SalesOrderQuery struct {
	PageQuery
	PartnerID *uint `form:"partner_id"`
	Status    *int8 `form:"status"`
}

// SalesOrderItemInput 是一条订单明细：只给数量与单价。
type SalesOrderItemInput struct {
	ProductID uint    `json:"product_id" binding:"required"`
	Qty       float64 `json:"qty" binding:"required,gt=0"`
	UnitPrice float64 `json:"unit_price" binding:"gte=0"`
	Remark    string  `json:"remark" binding:"max=255"`
}

// SalesOrderInput 新建与编辑共用。
//
// 这里**故意没有** amount_total / amount_base_total 字段：合计只由 service 从明细汇总。
// 前端即使把总额塞进请求体，绑定阶段就会被丢弃——比"接收了再判空"更彻底，
// 因为没有任何代码路径能读到它，也就不可能有人哪天一顺手把它存进库。
type SalesOrderInput struct {
	PartnerID    uint   `json:"partner_id" binding:"required"`
	TradeTerm    string `json:"trade_term" binding:"required,max=8"`
	CurrencyCode string `json:"currency_code" binding:"max=8"`
	// OrderDate 下单日，YYYY-MM-DD；留空为今天。
	OrderDate string `json:"order_date"`
	// DeliveryDate 约定交期，可留空。
	DeliveryDate string                `json:"delivery_date"`
	Remark       string                `json:"remark" binding:"max=255"`
	Items        []SalesOrderItemInput `json:"items" binding:"required,min=1,dive"`
}

// SalesOrderStatusInput 走完整的单据状态机，具体能不能跳由 service 判定。
type SalesOrderStatusInput struct {
	Status int8 `json:"status" binding:"required"`
}
