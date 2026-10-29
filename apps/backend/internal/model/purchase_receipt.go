package model

import (
	"time"

	"github.com/edwinhuish/gowork/internal/pkg/money"
)

// PurchaseReceipt 是一次实际到货（T-51a）。
//
// 采购订单说的是「我要这些货」，收货单才是「货到了」。在此之前两者之间只有一个人
// 填的字符串 source_doc_no：在途量算不出（不知道还有多少没到），入库单追不回是哪张
// 采购单，分批到货只能建几张互不关联的单。
//
// 挂在采购订单**行**上而不是订单头：一张单常有好几个 SKU、分批从不同工厂出来，
// 与跟单节点（PurchaseMilestone）同一个理由。
type PurchaseReceipt struct {
	Base
	// 遮蔽 Base 里的同名字段，把它排进「未删才唯一」的复合索引，见 Product 的说明。
	DeletedAt       DeletedAt      `gorm:"softDelete:milli;not null;default:0;uniqueIndex:uni_purchase_receipt_doc_no,priority:2" json:"deleted_at,omitempty"`
	DocNo           string         `gorm:"size:32;uniqueIndex:uni_purchase_receipt_doc_no,priority:1;not null" json:"doc_no"`
	PurchaseOrderID uint           `gorm:"index;not null" json:"purchase_order_id"`
	Status          DocumentStatus `gorm:"not null;index" json:"status"`
	// ReceivedDate 是实际到货日，决定它落在哪个统计月份。
	ReceivedDate time.Time `gorm:"type:date;not null;index" json:"received_date"`
	Remark       string    `gorm:"size:255" json:"remark"`

	Items []PurchaseReceiptItem `gorm:"foreignKey:ReceiptID" json:"items,omitempty"`
	// PurchaseOrderDocNo 是给列表与详情用的只读投影，省得前端再去翻一层。
	PurchaseOrderDocNo string `gorm:"-" json:"purchase_order_doc_no,omitempty"`
}

func (PurchaseReceipt) TableName() string { return "biz_purchase_receipt" }

// IsEditable 报告这张收货单还能不能被改。
//
// 只有草稿可改可删：已确认的收货单已经动过库存，改它等于改一批已经入账的货。
func (r PurchaseReceipt) IsEditable() bool { return r.Status == DocumentStatusDraft }

// PurchaseReceiptItem 是收货单的一行，挂到采购订单行上。
//
// 存物料快照（SKU / 品名 / 单位）而不是每次回采购单取：采购单上的品名改了，
// 历史收货单要能复现当时收到的到底是什么。
type PurchaseReceiptItem struct {
	Base
	ReceiptID           uint          `gorm:"index;not null" json:"receipt_id"`
	PurchaseOrderItemID uint          `gorm:"index;not null" json:"purchase_order_item_id"`
	ProductID           uint          `gorm:"not null" json:"product_id"`
	SKUCode             string        `gorm:"size:32" json:"sku_code"`
	NameCN              string        `gorm:"size:128" json:"name_cn"`
	Unit                string        `gorm:"size:16" json:"unit"`
	Qty                 money.Decimal `gorm:"type:decimal(14,3);not null" json:"qty"`
	// ReturnedQty 是这一行已退货的数量（T-52b3）。累计不超 Qty：
	// 退超过收到的，库存会变成凭空多出来的负数，而对账时谁也解释不清。
	// BatchNo 是这次到货的批次号（T-58）。工厂的批号、封箱号、
	// 客户投诉时说的"那一批"都是它；留空表示不按批次管理。
	BatchNo     string        `gorm:"size:64;index" json:"batch_no"`
	ReturnedQty money.Decimal `gorm:"type:decimal(14,3);not null;default:0" json:"returned_qty"`
	Remark      string        `gorm:"size:255" json:"remark"`
}

func (PurchaseReceiptItem) TableName() string { return "biz_purchase_receipt_item" }
