package dto

// CustomsDeclarationQuery 是报关单的查询条件。
//
// ShipmentID 是主维度：报关单总挂在某一票货下被查看与维护。
type CustomsDeclarationQuery struct {
	PageQuery
	ShipmentID *uint `form:"shipment_id"`
	Status     *int8 `form:"status"`
}

// CustomsDeclarationInput 新建与编辑共用。
//
// decl_no 允许为空：要素先备齐、报关单号后补是常态，"待申报"这个状态
// 存在的意义就是把这段空档记下来。decl_date 同样是"申报了才有"。
type CustomsDeclarationInput struct {
	ShipmentID      uint   `json:"shipment_id" binding:"required"`
	Status          int8   `json:"status" binding:"required"`
	DeclNo          string `json:"decl_no" binding:"max=32"`
	DeclDate        string `json:"decl_date"`
	SupervisionMode string `json:"supervision_mode" binding:"max=8"`
	Remark          string `json:"remark" binding:"max=255"`
}
