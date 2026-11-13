package dto

// ProductCodeQuery 列出某个产品的编号。
//
// product_id 必填：编号表是"产品的别名"，没有"全库编号"这种问法，
// 而不带条件的查询会把所有产品的别名摊在一个响应里。
type ProductCodeQuery struct {
	ProductID uint `form:"product_id" binding:"required"`
}

// ProductCodeInput 新增一个编号。
//
// 没有 code_norm：归一化形态由服务端算。让客户端传，等于让它决定
// "这个号该怎么被搜到"——改一个字段就能让搜索失效。
type ProductCodeInput struct {
	ProductID uint   `json:"product_id" binding:"required"`
	Code      string `json:"code" binding:"required,max=64"`
	// Source 取值见 model.ProductCodeSource*；留空按 other。
	Source string `json:"source" binding:"max=16"`
	Remark string `json:"remark" binding:"max=255"`
}
