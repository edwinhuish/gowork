package dto

import (
	"time"

	"github.com/edwinhuish/gowork/internal/pkg/money"
)

// PurchaseReturnInput 是从收货单发起退货的入参（T-52b3）。
//
// 退货从收货单发起而不是从出入库单发起，是为了让依赖方向保持
// receipt → stockIn：出入库服务已经被收货服务依赖（收货确认要生成采购入库），
// 反过来再让出入库服务查"这张收货单还剩多少可退"，依赖就成环了。
type PurchaseReturnInput struct {
	// ReturnDate 退货日，YYYY-MM-DD，留空取今天。
	ReturnDate string                    `json:"return_date"`
	Reason     string                    `json:"reason" binding:"max=255"`
	Items      []PurchaseReturnItemInput `json:"items" binding:"required,min=1,dive"`
}

// PurchaseReturnItemInput 一行退货，挂到收货单明细上。
//
// 按收货行退而不是按产品退：同一张收货单可能两批到货、单价不同，
// 按产品退就分不清退的是哪一批。
type PurchaseReturnItemInput struct {
	PurchaseReceiptItemID uint          `json:"purchase_receipt_item_id" binding:"required"`
	Qty                   money.Decimal `json:"qty"`
	Remark                string        `json:"remark" binding:"max=255"`
}

// PurchaseReturnIssueInput 是退货确认时生成采购退货出库单的入参（窄接口，不对外）。
type PurchaseReturnIssueInput struct {
	// SourceNo 既是来源单号也是判重键：同一张收货单可以分多次退货，
	// 但每一次都要有自己的来源号（收货单号 + 序号），否则第二次会被判成重复。
	SourceNo  string
	PartnerID *uint
	DocDate   time.Time
	Remark    string
	Items     []PurchaseReturnIssueItem
}

// PurchaseReturnIssueItem 是退货出库单的一行，照抄收货单的物料快照。
type PurchaseReturnIssueItem struct {
	ProductID uint
	SKUCode   string
	NameCN    string
	Unit      string
	Qty       money.Decimal
	Remark    string
}
