package dto

// DashboardSummary 首页概览数据。
type DashboardSummary struct {
	ProductTotal    int64 `json:"product_total"`
	EnabledProducts int64 `json:"enabled_products"`
	SupplierTotal   int64 `json:"supplier_total"`
	CustomerTotal   int64 `json:"customer_total"`
}
