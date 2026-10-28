package dto

// ReceivableQuery 是应收单的查询条件。
type ReceivableQuery struct {
	PageQuery
	PartnerID  *uint `form:"partner_id"`
	SourceType *int8 `form:"source_type"`
	SourceID   *uint `form:"source_id"`
	Status     *int8 `form:"status"`
	// Overdue=1 只看已到期且未结清的单，账龄与"今天该催谁"都从它取。
	Overdue *int8 `form:"overdue"`
}

// PayableQuery 是应付单的查询条件。
type PayableQuery struct {
	PageQuery
	PartnerID *uint `form:"partner_id"`
	Status    *int8 `form:"status"`
	Overdue   *int8 `form:"overdue"`
}

// ReceivableInput 新建与编辑共用。
//
// 没有币种、汇率、对方主体：三者一律从来源单据带出并锁定（蓝图 F3 的硬规则）。
// 允许前端传，就允许它和订单不一致——那会在对账时变成两张单各说一个数。
type ReceivableInput struct {
	// SourceType 只能是 10 销售订单。
	SourceType int8 `json:"source_type" binding:"required"`
	SourceID   uint `json:"source_id" binding:"required"`
	// Amount 为 0 表示"把来源未开票余额全开出来"。
	Amount float64 `json:"amount" binding:"gte=0"`
	// DueDate 留空表示按客户账期推算（订单日 + credit_days）。
	DueDate string `json:"due_date"`
	// Status 只在编辑时用于作废；新建恒为未结清。
	Status *int8  `json:"status"`
	Remark string `json:"remark" binding:"max=255"`
}

// PayableInput 新建与编辑共用，规则同应收，来源是采购订单。
type PayableInput struct {
	SourceType int8    `json:"source_type" binding:"required"`
	SourceID   uint    `json:"source_id" binding:"required"`
	Amount     float64 `json:"amount" binding:"gte=0"`
	DueDate    string  `json:"due_date"`
	Status     *int8   `json:"status"`
	Remark     string  `json:"remark" binding:"max=255"`
}
