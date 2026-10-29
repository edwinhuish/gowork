package dto

import (
	"time"

	"github.com/edwinhuish/gowork/internal/pkg/money"
)

// PurchaseOrderQuery 是采购订单列表的查询条件。
type PurchaseOrderQuery struct {
	PageQuery
	SupplierID *uint `form:"supplier_id"`
	Status     *int8 `form:"status"`
	// OverdueNode 筛"这个跟单节点已逾期（计划日已过、还没完成）"的采购单（T-38）。
	// 取 10 下单 / 20 生产完成 / 30 验货 / 40 入仓 / 50 出运。
	OverdueNode *int8 `form:"overdue_node"`
}

// PurchaseOrderItemInput 是一条采购明细。
type PurchaseOrderItemInput struct {
	ProductID uint `json:"product_id" binding:"required"`
	// Qty 定点收发。binding 的 gt 对结构体不生效，"必须大于 0"由服务层判。
	Qty money.Decimal `json:"qty"`
	// UnitPrice 按字符串收发（money.Decimal）。
	UnitPrice money.Decimal `json:"unit_price"`
	// SalesOrderItemID 非空表示这一行为该销售订单行采购（以销定采）。
	SalesOrderItemID *uint `json:"sales_order_item_id"`
	// SourceID 指定这行采购对应的货源，仅作追溯，不参与取价：
	// 采购价是谈出来的，不是从货源表里读出来的。
	SourceID *uint  `json:"source_id"`
	Remark   string `json:"remark" binding:"max=255"`
}

// PurchaseOrderInput 新建与编辑共用。
//
// 与销售订单同理：**没有**合计字段。前端把 amount_total 塞进来会被绑定阶段丢弃，
// 合计只从明细汇总一次。
type PurchaseOrderInput struct {
	SupplierID   uint   `json:"supplier_id" binding:"required"`
	TradeTerm    string `json:"trade_term" binding:"required,max=8"`
	CurrencyCode string `json:"currency_code" binding:"max=8"`
	// OrderDate 下单日，留空取今天。
	OrderDate string `json:"order_date"`
	// DeliveryDate 供应商承诺的到货日，可留空。
	DeliveryDate string                   `json:"delivery_date"`
	Remark       string                   `json:"remark" binding:"max=255"`
	Items        []PurchaseOrderItemInput `json:"items" binding:"required,min=1,dive"`
	// UpdatedAt 是编辑页打开时读到的版本；带上它做条件更新，期间被别人改过则 409。
	UpdatedAt *time.Time `json:"updated_at"`
}

// PurchaseOrderStatusInput 走完整单据状态机，具体能不能跳由 service 判定。
type PurchaseOrderStatusInput struct {
	Status int8 `json:"status" binding:"required"`
}
