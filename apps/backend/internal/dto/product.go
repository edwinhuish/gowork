package dto

// ProductQuery 产品列表查询条件。
type ProductQuery struct {
	PageQuery
	Category   string `form:"category"`
	SupplierID *uint  `form:"supplier_id"`
	Status     *int8  `form:"status"`
}

// ProductInput 产品新增与编辑共用的入参。
//
// 编辑走全量覆盖（PUT 语义）：前端表单本来就会提交全部可见字段，
// 用同一套结构可以避免 Create/Update 两份几乎相同的校验规则各自漂移。
type ProductInput struct {
	SkuCode    string  `json:"sku_code" binding:"required,max=32"`
	NameCN     string  `json:"name_cn" binding:"required,max=128"`
	NameEN     string  `json:"name_en" binding:"max=128"`
	Category   string  `json:"category" binding:"max=32"`
	Spec       string  `json:"spec" binding:"max=128"`
	Unit       string  `json:"unit" binding:"max=16"`
	SupplierID *uint   `json:"supplier_id"`
	CostPrice  float64 `json:"cost_price" binding:"gte=0"`
	SalePrice  float64 `json:"sale_price" binding:"gte=0"`
	Status     *int8   `json:"status"`
	Remark     string  `json:"remark" binding:"max=255"`
}
