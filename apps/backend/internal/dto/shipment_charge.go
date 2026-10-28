package dto

// ShipmentChargeQuery 是运费杂费的查询条件。
//
// ShipmentID 是主维度：费用总挂在某一票货下被查看与维护，
// 出货详情页的费用面板传的就是它。
type ShipmentChargeQuery struct {
	PageQuery
	ShipmentID *uint `form:"shipment_id"`
	Type       *int8 `form:"type"`
}

// ShipmentChargeInput 新建与编辑共用。
//
// 没有 amount_base 与 exchange_rate：本币金额与汇率由 service 按录入当日汇率折算后回填，
// 前端提交这两个值一律被忽略。否则一个人手填本币金额、另一个人让系统按汇率算，
// 同一票货的合计就会出现两个口径，而账单上只有一个数。
type ShipmentChargeInput struct {
	ShipmentID   uint    `json:"shipment_id" binding:"required"`
	Type         int8    `json:"type" binding:"required"`
	PartnerID    *uint   `json:"partner_id"`
	Amount       float64 `json:"amount" binding:"gte=0"`
	CurrencyCode string  `json:"currency_code" binding:"max=8"`
	Remark       string  `json:"remark" binding:"max=255"`
}

// ShipmentChargeSummary 是一票货的本币费用合计，供出货详情与单票毛利使用。
//
// 合计由 SQL 算而不是前端把列表加一遍：费用分页展示，前端手里那一页不是全部。
type ShipmentChargeSummary struct {
	ShipmentID uint    `json:"shipment_id"`
	TotalBase  float64 `json:"total_base"`
}
