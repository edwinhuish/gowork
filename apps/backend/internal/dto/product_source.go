package dto

import "github.com/edwinhuish/gowork/internal/pkg/money"

// ProductSourceQuery 是产品货源列表的查询条件。
//
// ProductID 是主维度：货源总是"挂在这个产品下"被查看与维护。
// PartnerID 供反向查（这个供应商供了哪些产品），用在采购询价与供应商详情里。
type ProductSourceQuery struct {
	PageQuery
	ProductID *uint `form:"product_id"`
	PartnerID *uint `form:"partner_id"`
	// IncludeDeleted 把已软删的货源一并列出，这些行只能做一件事：恢复。
	IncludeDeleted bool `form:"include_deleted"`
}

// ProductSourceInput 新建与编辑共用。
//
// 没有 SalePrice 之类的字段：货源只描述"从这家进的成本"，
// 卖多少钱由报价单决定，两边混在一起会让同一条记录同时承担两个角色。
type ProductSourceInput struct {
	ProductID   uint   `json:"product_id" binding:"required"`
	PartnerID   uint   `json:"partner_id" binding:"required"`
	SupplierSKU string `json:"supplier_sku" binding:"max=64"`
	// PurchasePrice 原币采购价。币种留空即本币，由 service 折算并回填。
	PurchasePrice money.Decimal `json:"purchase_price"`
	CurrencyCode  string        `json:"currency_code" binding:"max=8"`
	MOQ           int           `json:"moq" binding:"gte=0"`
	DeliveryDays  int           `json:"delivery_days" binding:"gte=0,lte=3650"`
	Remark        string        `json:"remark" binding:"max=255"`
}
