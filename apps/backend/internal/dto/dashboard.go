package dto

// DashboardSummary 首页概览数据。
//
// ProductTotal 是全部未删除的产品数：停用概念取消之后，列表里剩下的
// 就是全部产品，再给一个"其中启用多少"是自相矛盾的两个总数。
type DashboardSummary struct {
	ProductTotal  int64 `json:"product_total"`
	SupplierTotal int64 `json:"supplier_total"`
	CustomerTotal int64 `json:"customer_total"`
}
