package model

import (
	"fmt"
	"time"
)

// StockMovementType 说明这条流水为什么发生，不决定方向。
//
// 方向单独存 `Direction`，因为同一种类型可以两个方向：调拨既有调入也有调出，
// 盘点既有盘盈也有盘亏。把方向编进类型（"调拨入"/"调拨出"）会让类型数量翻倍，
// 而报表又要按"调拨"归并时再拼回来。
//
// 中文标签要与前端 `constants/catalog.ts` 的 STOCK_MOVEMENT_OPTIONS 一致。
type StockMovementType int8

const (
	// StockMovementOpening 期初：系统上线时的存量，来源是盘点表而不是入库单。
	StockMovementOpening StockMovementType = 10
	// StockMovementPurchase 采购入库：由采购入库单产生（T-16）。
	StockMovementPurchase StockMovementType = 20
	// StockMovementSales 销售出库：由销售出库单产生（T-16）。
	StockMovementSales StockMovementType = 30
	// StockMovementTransfer 调拨：仓间转移，两侧各一条流水（T-16）。
	StockMovementTransfer StockMovementType = 40
	// StockMovementCount 盘点盈亏：盘盈为入、盘亏为出。差异走流水，不改快照（T-16）。
	StockMovementCount StockMovementType = 50
	// StockMovementOther 其它：样品、报废、赠品等没有单据的零星变动。
	StockMovementOther StockMovementType = 90
)

func (t StockMovementType) IsValid() bool {
	switch t {
	case StockMovementOpening, StockMovementPurchase, StockMovementSales,
		StockMovementTransfer, StockMovementCount, StockMovementOther:
		return true
	}
	return false
}

// IsManual 表示这类流水可以不经单据、直接手工录入。
//
// 采购入库与销售出库必须挂在单据上：手工放行等于允许绕过入库单直接加库存，
// 那么"库存数从哪来"就没有答案了。它们由 T-16 的单据层调用内部写入口产生。
func (t StockMovementType) IsManual() bool {
	switch t {
	case StockMovementOpening, StockMovementCount, StockMovementOther:
		return true
	}
	return false
}

func (t StockMovementType) Label() string {
	switch t {
	case StockMovementOpening:
		return "期初"
	case StockMovementPurchase:
		return "采购入库"
	case StockMovementSales:
		return "销售出库"
	case StockMovementTransfer:
		return "调拨"
	case StockMovementCount:
		return "盘点盈亏"
	case StockMovementOther:
		return "其它"
	default:
		return fmt.Sprintf("未知类型(%d)", int8(t))
	}
}

// StockDirection 是出入方向。用 ±1 而不是 0/1：
// 聚合库存时直接 SUM(direction * qty) 就是存量，少一次 CASE WHEN 的方言差异。
type StockDirection int8

const (
	StockDirectionIn  StockDirection = 1
	StockDirectionOut StockDirection = -1
)

func (d StockDirection) IsValid() bool {
	return d == StockDirectionIn || d == StockDirectionOut
}

func (d StockDirection) Label() string {
	if d == StockDirectionOut {
		return "出库"
	}
	return "入库"
}

// 流水的来源单据类型。手工录入的流水不带来源（三个字段皆空）。
const (
	StockSourceStockDocument = "stock_document"
)

// StockMovement 是库存变动的唯一事实来源。
//
// 每条记录都带产品快照（SKU / 品名 / 规格 / 单位）：流水是历史，
// 产品改名或改单位之后，回看三个月前的流水仍应显示当时那个说法。
//
// 没有金额字段：库存成本（加权平均）要等采购价与运杂费都落定，
// 那是成本核算的事，不在流水上先记一个会过期的单价。
type StockMovement struct {
	Base
	DocNo     string            `gorm:"size:32;not null;uniqueIndex" json:"doc_no"`
	ProductID uint              `gorm:"not null;index" json:"product_id"`
	SKUCode   string            `gorm:"size:32;not null" json:"sku_code"`
	NameCN    string            `gorm:"size:128" json:"name_cn"`
	Spec      string            `gorm:"size:255" json:"spec"`
	Unit      string            `gorm:"size:16" json:"unit"`
	Type      StockMovementType `gorm:"not null;index" json:"type"`
	Direction StockDirection    `gorm:"not null" json:"direction"`
	// Qty 始终为正数，方向看 Direction。允许存负数会让"两条 +5 与一条 -10"
	// 变得同样合法，而对账时分不清是录错了还是故意的。
	Qty float64 `gorm:"type:decimal(14,3);not null" json:"qty"`
	// OccurredAt 是业务发生日（只取日期，见 DateOnly），不是录入时间。
	OccurredAt time.Time `gorm:"not null;index" json:"occurred_at"`
	// SourceType / SourceID / SourceDocNo 指向产生这条流水的单据；
	// 手工录入的三者皆空，靠 DocNo 追溯。
	SourceType  string `gorm:"size:32" json:"source_type"`
	SourceID    uint   `json:"source_id"`
	SourceDocNo string `gorm:"size:32" json:"source_doc_no"`
	Remark      string `gorm:"size:255" json:"remark"`

	Product *Product `gorm:"foreignKey:ProductID" json:"product,omitempty"`
}

func (StockMovement) TableName() string { return "inv_stock_movement" }

// SignedQty 返回带符号的数量，正数入库、负数出库。
func (m StockMovement) SignedQty() float64 {
	return float64(m.Direction) * m.Qty
}

// StockSnapshot 是当前库存。
//
// **只能由流水推出来**：没有任何接口能直接写它。业务代码要是能改快照，
// "快照和流水对不上"这件事就永远查不出是谁的错——流水说是 100，快照说是 80，
// 两边都是权威，等于没有权威。
//
// 不存 SKU / 品名：快照是"现在"的视图，产品改名后应该跟着变；
// 快照字段一旦冗余，改名当天就会有一半界面显示旧名。
type StockSnapshot struct {
	Base
	ProductID uint `gorm:"not null;uniqueIndex" json:"product_id"`
	// Qty 可以为负：直发业务里"先出货、后补入库单"是常态，
	// 硬拦负库存只会逼人先造一张假的入库单。
	Qty float64 `gorm:"type:decimal(14,3);not null" json:"qty"`
	// LastMovementID / LastOccurredAt 记最后一次变动，用于"这个数有多旧"。
	LastMovementID uint       `gorm:"not null" json:"last_movement_id"`
	LastOccurredAt *time.Time `json:"last_occurred_at"`
}

func (StockSnapshot) TableName() string { return "inv_stock_snapshot" }
