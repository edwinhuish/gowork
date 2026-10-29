package model

import (
	"fmt"
	"time"

	"github.com/edwinhuish/gowork/internal/pkg/money"
)

// StockDocType 是出入库单的类型。
//
// 四类单据共用一张表而不是四张：它们的表头（日期、往来单位、来源单号、备注）
// 与明细（产品、数量）几乎一样，差别只在"确认时怎么生成流水"。
// 四张表会带来四套 CRUD、四个列表页，以及"查这个产品进出了几次"要 UNION 四张表。
type StockDocType int8

const (
	// StockDocPurchaseIn 采购入库：方向固定入，来源通常是采购订单。
	StockDocPurchaseIn StockDocType = 10
	// StockDocSalesOut 销售出库：方向固定出，来源通常是出货批次。
	StockDocSalesOut StockDocType = 20
	// StockDocTransfer 调拨：仓间转移，**尚未实现**——它需要仓库维度，
	// 而当前库存只有一个池子（见 docs/roadmap.md T-16b）。
	StockDocTransfer StockDocType = 30
	// StockDocCount 盘点：填实盘数，确认时按「实盘 − 账面」生成盈亏流水。
	StockDocCount StockDocType = 40
)

func (t StockDocType) IsValid() bool {
	switch t {
	case StockDocPurchaseIn, StockDocSalesOut, StockDocTransfer, StockDocCount:
		return true
	}
	return false
}

// IsImplemented 与 IsValid 分开：调拨的类型编号先占好，
// 但当前没有仓库维度，"调到自己"会生成两条方向相反的流水，等于什么也没发生。
func (t StockDocType) IsImplemented() bool {
	switch t {
	case StockDocPurchaseIn, StockDocSalesOut, StockDocCount:
		return true
	}
	return false
}

func (t StockDocType) Label() string {
	switch t {
	case StockDocPurchaseIn:
		return "采购入库"
	case StockDocSalesOut:
		return "销售出库"
	case StockDocTransfer:
		return "调拨"
	case StockDocCount:
		return "盘点"
	default:
		return fmt.Sprintf("未知类型(%d)", int8(t))
	}
}

// MovementType 是确认这张单据时生成的流水类型。
func (t StockDocType) MovementType() StockMovementType {
	switch t {
	case StockDocPurchaseIn:
		return StockMovementPurchase
	case StockDocSalesOut:
		return StockMovementSales
	case StockDocCount:
		return StockMovementCount
	default:
		return 0
	}
}

// DocType 决定单号前缀。
func (t StockDocType) DocType() DocType {
	switch t {
	case StockDocPurchaseIn:
		return DocTypeStockIn
	case StockDocSalesOut:
		return DocTypeStockOut
	case StockDocCount:
		return DocTypeStockCount
	default:
		return ""
	}
}

// FixedDirection 返回出入库单的固定方向；盘点没有固定方向（按盈亏逐行决定），
// 第二个返回值为 false。
func (t StockDocType) FixedDirection() (StockDirection, bool) {
	switch t {
	case StockDocPurchaseIn:
		return StockDirectionIn, true
	case StockDocSalesOut:
		return StockDirectionOut, true
	default:
		return 0, false
	}
}

// StockDocument 是出入库单。
//
// 草稿不产生任何库存变动——入库单先在系统里核对，确认那一刻才入账。
// 确认之后不可改、不可删、也不可取消：流水已经落地，改它就是改历史；
// 错了要做一张反向单（红字），这样两次操作都留在账上。
type StockDocument struct {
	Base
	// 遮蔽 Base 里的同名字段，把它排进「未删才唯一」的复合索引，见 Product 的说明。
	DeletedAt DeletedAt    `gorm:"softDelete:milli;not null;default:0;uniqueIndex:uni_stock_document_doc_no,priority:2" json:"-"`
	DocNo     string       `gorm:"size:32;not null;uniqueIndex:uni_stock_document_doc_no,priority:1" json:"doc_no"`
	Type   StockDocType   `gorm:"not null;index" json:"type"`
	Status DocumentStatus `gorm:"not null;index" json:"status"`
	// DocDate 是业务发生日，决定这批货算在哪个月。
	DocDate time.Time `gorm:"not null;index" json:"doc_date"`
	// PartnerID 是入库的供应商 / 出库的客户；盘点单为空。
	PartnerID *uint    `gorm:"index" json:"partner_id"`
	Partner   *Partner `gorm:"foreignKey:PartnerID" json:"partner,omitempty"`
	// SourceDocNo 记来源单号（PO-… / SH-…），只作追溯，不做外键校验：
	// 存 id 而不校验会制造悬空引用，而真正对账时用单号查得到就够了。
	SourceDocNo string              `gorm:"size:32" json:"source_doc_no"`
	Remark      string              `gorm:"size:255" json:"remark"`
	PostedAt    *time.Time          `json:"posted_at"`
	Items       []StockDocumentItem `gorm:"foreignKey:DocumentID" json:"items,omitempty"`
}

func (StockDocument) TableName() string { return "inv_stock_document" }

// IsEditable 只有草稿可改可删。已确认的单据已经产生了库存流水。
func (d StockDocument) IsEditable() bool { return d.Status == DocumentStatusDraft }

// StockDocumentItem 是一行出入库明细。
//
// 产品信息存快照：单据要打印给仓库看，产品改名或改单位之后，
// 三个月前那张入库单上仍应是当时那个说法。
//
// Qty 的含义看单据类型：入库 / 出库是数量，盘点是**实盘数**。
// BookQty 只在盘点单确认时写入（账面数），差异 = Qty − BookQty，不单独存。
type StockDocumentItem struct {
	Base
	DocumentID uint    `gorm:"not null;index" json:"document_id"`
	ProductID  uint    `gorm:"not null;index" json:"product_id"`
	SKUCode    string  `gorm:"size:32;not null" json:"sku_code"`
	NameCN     string  `gorm:"size:128" json:"name_cn"`
	Spec       string  `gorm:"size:255" json:"spec"`
	Unit       string  `gorm:"size:16" json:"unit"`
	Qty        money.Decimal  `gorm:"type:decimal(14,3);not null" json:"qty"`
	// BookQty 为 nil 表示还没确认过（草稿），或这张单不是盘点单。
	BookQty *money.Decimal `gorm:"type:decimal(14,3)" json:"book_qty"`
	Remark  string         `gorm:"size:255" json:"remark"`
}

func (StockDocumentItem) TableName() string { return "inv_stock_document_item" }
