package dto

import (
	"time"

	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/money"
)

// SalesOrderChangeInput 是一次变更的提交内容。
//
// 不含"新增行 / 删除行"：出货分配、以销定采的挂接都按明细行 id 引用，
// 增删行会让这些引用指向不存在的行。要加减货请作废重下——
// 那本来也是一次新的商务谈判。
type SalesOrderChangeInput struct {
	// Reason 必填：变更记录的价值全在"为什么改"。
	Reason string `json:"reason" binding:"required,max=255"`
	// DeliveryDate 留空表示这次不改交期；填了就以它为准。
	DeliveryDate *string `json:"delivery_date"`
	// Lines 只提交发生变化的行；空数组表示这次只改交期。
	Lines []SalesOrderChangeLineInput `json:"lines" binding:"dive"`
	// UpdatedAt 是打开页面时的订单版本，用于条件更新（与编辑同一套乐观锁）。
	UpdatedAt *time.Time `json:"updated_at"`
}

// SalesOrderChangeLineInput 是某一行订单明细的新值。
type SalesOrderChangeLineInput struct {
	SalesOrderItemID uint `json:"sales_order_item_id" binding:"required"`
	// Qty 与 UnitPrice 是变更后的目标值（不是增量）。
	// 目标值比增量好核对：客户说"改成 800"，就填 800。
	Qty       money.Decimal `json:"qty"`
	UnitPrice money.Decimal `json:"unit_price"`
}

// SalesOrderChangeView 是变更历史的展示结构。
type SalesOrderChangeView struct {
	*model.SalesOrderChange
	// ShippedQty 是变更后该行的已出货量，供前端提示"这行已经出掉多少"。
	// 变更时只允许把数量改到不低于已出货量。
	ShippedQty map[uint]money.Decimal `json:"shipped_qty,omitempty"`
}
