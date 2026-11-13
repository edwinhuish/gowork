package dto

// PurchaseMilestoneInput 是一条跟单节点的补录。
//
// 计划日与完成日都可空：先排计划、过后再补完成，是跟单最常见的节奏。
// 两者传**空串表示清空**（不是"不修改"）——这个接口提交的是"这一条现在长什么样"，
// 想不动它就别提交这一条。
type PurchaseMilestoneInput struct {
	PurchaseOrderItemID uint   `json:"purchase_order_item_id" binding:"required"`
	Node                int8   `json:"node" binding:"required"`
	PlanDate            string `json:"plan_date" binding:"max=10"`
	DoneDate            string `json:"done_date" binding:"max=10"`
	Remark              string `json:"remark" binding:"max=255"`
}

// PurchaseMilestoneSaveInput 是一次补录请求，可只提交部分节点。
type PurchaseMilestoneSaveInput struct {
	Milestones []PurchaseMilestoneInput `json:"milestones" binding:"required,min=1,dive"`
}
