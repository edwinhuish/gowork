package dto

import (
	"time"

	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/money"
)

// PurchaseReceiptInput 是建收货单的入参。
//
// 只有草稿能建：确认动作单独走 `PUT /purchase-receipts/:id/confirm`，
// 因为它会写库存流水——建单与动库存不该是同一个请求。
type PurchaseReceiptInput struct {
	// ReceivedDate 实际到货日，YYYY-MM-DD，留空取今天。
	ReceivedDate string                     `json:"received_date"`
	Remark       string                     `json:"remark" binding:"max=255"`
	Items        []PurchaseReceiptItemInput `json:"items" binding:"required,min=1,dive"`
}

// PurchaseReceiptItemInput 一行收货，挂到采购订单行上。
//
// Qty 必填且为正：收 0 件的收货单没有意义，而"先建空单再补"会让在途量
// 在一段时间里算错。
type PurchaseReceiptItemInput struct {
	PurchaseOrderItemID uint          `json:"purchase_order_item_id" binding:"required"`
	Qty                 money.Decimal `json:"qty"`
	Remark              string        `json:"remark" binding:"max=255"`
}

// PurchaseInIssueInput 是收货确认时生成采购入库单的入参（窄接口，不对外）。
type PurchaseInIssueInput struct {
	// ReceiptNo 既是来源单号，也是判重的键：一张收货单只能生成一张入库单。
	ReceiptNo string
	PartnerID *uint
	DocDate   time.Time
	Remark    string
	Items     []PurchaseInIssueItem
}

// PurchaseReceiptSummary 是采购详情页要的一组数：收货单列表 + 每行已收量。
//
// 已收量按「订单行 id → 定点字符串」给：JSON 的 map key 只能是字符串，
// 而让前端自己遍历收货单去加总，等于把在途量的算法复制到前端。
type PurchaseReceiptSummary struct {
	Receipts []model.PurchaseReceipt `json:"receipts"`
	Received map[string]string       `json:"received_by_item"`
}

// PurchaseInIssueItem 是入库单的一行，照抄收货单的物料快照。
type PurchaseInIssueItem struct {
	ProductID uint
	SKUCode   string
	NameCN    string
	Unit      string
	Qty       money.Decimal
	Remark    string
}
