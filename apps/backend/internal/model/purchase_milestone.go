package model

import (
	"fmt"
	"time"
)

// PurchaseNode 是采购跟单的五个节点（T-38）。
//
// 编号留间隔是为了将来能在中间插节点（比如"大货样确认"夹在生产与验货之间），
// 而不必改已有数据的含义。
type PurchaseNode int8

const (
	PurchaseNodeOrdered    PurchaseNode = 10 // 下单（向工厂下生产指令）
	PurchaseNodeProduced   PurchaseNode = 20 // 生产完成
	PurchaseNodeInspected  PurchaseNode = 30 // 验货
	PurchaseNodeWarehoused PurchaseNode = 40 // 入仓
	PurchaseNodeShipped    PurchaseNode = 50 // 出运
)

var purchaseNodeNames = []struct {
	node  PurchaseNode
	label string
}{
	{PurchaseNodeOrdered, "下单"},
	{PurchaseNodeProduced, "生产完成"},
	{PurchaseNodeInspected, "验货"},
	{PurchaseNodeWarehoused, "入仓"},
	{PurchaseNodeShipped, "出运"},
}

// PurchaseNodes 按业务顺序返回五个节点。
func PurchaseNodes() []PurchaseNode {
	out := make([]PurchaseNode, 0, len(purchaseNodeNames))
	for _, n := range purchaseNodeNames {
		out = append(out, n.node)
	}
	return out
}

func (n PurchaseNode) IsValid() bool {
	for _, item := range purchaseNodeNames {
		if item.node == n {
			return true
		}
	}
	return false
}

func (n PurchaseNode) Label() string {
	for _, item := range purchaseNodeNames {
		if item.node == n {
			return item.label
		}
	}
	return fmt.Sprintf("node(%d)", int8(n))
}

func (n PurchaseNode) String() string { return n.Label() }

// PurchaseMilestone 是**采购订单行**上的一个跟单节点（T-38）。
//
// 为什么挂在行上而不是订单头：一张采购单常有好几个 SKU、分批从不同工厂出来，
// "生产完成了"对整张单没有意义——A 厂做完 B 厂还没开工。
//
// 计划与完成分两个日期而不是一个：
//   - 只有完成日期，没法回答"该做而没做"（逾期视图就是这么来的）；
//   - 只有计划日期，没法回答"到底做完没有"。
//
// 完成日期为空 = 未完成；补录时写完成日期并把补录人记进 Operator。
type PurchaseMilestone struct {
	Base
	// 遮蔽 Base 里的同名字段：一条节点独占 (行, 节点)，软删后重建不冲突。
	DeletedAt           DeletedAt    `gorm:"softDelete:milli;not null;default:0;uniqueIndex:uni_purchase_milestone,priority:3" json:"deleted_at,omitempty"`
	PurchaseOrderID     uint         `gorm:"index;not null" json:"purchase_order_id"`
	PurchaseOrderItemID uint         `gorm:"not null;uniqueIndex:uni_purchase_milestone,priority:1" json:"purchase_order_item_id"`
	Node                PurchaseNode `gorm:"not null;uniqueIndex:uni_purchase_milestone,priority:2" json:"node"`

	// PlanDate 计划完成日，可空：下单时还没和工厂谈定是常态。
	PlanDate *time.Time `gorm:"type:date" json:"plan_date"`
	// DoneDate 实际完成日；为空表示这个节点还没做。
	DoneDate *time.Time `gorm:"type:date" json:"done_date"`
	// Operator 是补录完成的人（用户名的快照，不是外键）：账号改名后历史仍读得出当时是谁。
	Operator string `gorm:"size:64" json:"operator"`
	Remark   string `gorm:"size:255" json:"remark"`
}

func (PurchaseMilestone) TableName() string { return "biz_purchase_milestone" }

func (m PurchaseMilestone) NodeLabel() string { return m.Node.Label() }
