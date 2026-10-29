package dto

// InspectionInput 是新增验货记录的入参。
//
// 不做 PUT：验货记录是"某天某人验了这批货"的事实，改它等于改历史。
// 录错了删掉重录，审计日志里两次都有记录。
type InspectionInput struct {
	// PurchaseOrderItemID 可空：整单验货时留空。
	PurchaseOrderItemID *uint `json:"purchase_order_item_id"`
	// Result 只接受 passed / failed / partial。
	Result string `json:"result" binding:"required,max=16"`
	// SampleRate 是抽检比例（百分比），0 表示未记录。
	SampleRate int `json:"sample_rate" binding:"gte=0,lte=100"`
	// InspectedAt 验货日期 YYYY-MM-DD，留空取今天。
	InspectedAt string `json:"inspected_at"`
	Inspector   string `json:"inspector" binding:"max=64"`
	Defects     string `json:"defects" binding:"max=1000"`
	Remark      string `json:"remark" binding:"max=255"`
}
