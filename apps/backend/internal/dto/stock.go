package dto

import (
	"time"

	"github.com/edwinhuish/gowork/internal/pkg/money"
)

// StockMovementQuery 是库存流水的列表查询。
type StockMovementQuery struct {
	PageQuery
	ProductID *uint `form:"product_id"`
	Type      *int8 `form:"type"`
	// From / To 是业务发生日 YYYY-MM-DD（含两端），留空表示不限。
	From string `form:"from"`
	To   string `form:"to"`
}

// StockMovementInput 是手工录入一条库存流水的入参。
//
// 没有 source_type / source_id：带来源的流水必须由单据产生，
// 让前端能指定"这条流水来自哪张单"，等于开一个绕过单据直接造流水的口子。
type StockMovementInput struct {
	ProductID uint `json:"product_id" binding:"required"`
	// Type 只接受期初 / 盘点盈亏 / 其它，见 model.StockMovementType.IsManual。
	Type      int8 `json:"type" binding:"required"`
	Direction int8 `json:"direction" binding:"required"`
	// Qty 定点收发。gt 对结构体不生效，"必须大于 0"由服务层判。
	Qty money.Decimal `json:"qty"`
	// OccurredAt 留空取今天；不晚于今天才合理，未来的是计划不是事实。
	OccurredAt string `json:"occurred_at"`
	Remark     string `json:"remark" binding:"max=255"`
}

// StockSnapshotQuery 是库存快照（当前库存）的列表查询。
type StockSnapshotQuery struct {
	PageQuery
}

// StockSnapshotView 是库存列表的一行。
//
// 不直接回 model.StockSnapshot：快照表只存 product_id 与数量，
// 列表要的 SKU / 品名 / 单位是 join 出来的当前值。
type StockSnapshotView struct {
	ProductID      uint          `json:"product_id"`
	SKUCode        string        `json:"sku_code"`
	NameCN         string        `json:"name_cn"`
	Spec           string        `json:"spec"`
	Unit           string        `json:"unit"`
	Qty            money.Decimal `json:"qty"`
	LastOccurredAt *time.Time    `json:"last_occurred_at"`
	UpdatedAt      time.Time     `json:"updated_at"`
}

// StockRecalcInput 指定重算范围：给 product_id 只重算这一个产品，
// 留空重算全部有流水的产品。
type StockRecalcInput struct {
	ProductID *uint `json:"product_id"`
}

// StockRecalcDiff 是重算时发现的差异：重算前快照上的值 vs 流水算出来的值。
//
// 正常情况下永远为空。它一旦非空，说明有人绕过了流水改过快照（比如直接改库），
// 这条信息比"重算成功"本身更有价值。
type StockRecalcDiff struct {
	ProductID   uint          `json:"product_id"`
	SnapshotQty money.Decimal `json:"snapshot_qty"`
	MovementQty money.Decimal `json:"movement_qty"`
}

// StockRecalcResult 是重算结果。
type StockRecalcResult struct {
	Recalculated int               `json:"recalculated"`
	Mismatched   []StockRecalcDiff `json:"mismatched"`
}
