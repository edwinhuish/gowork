package dto

import (
	"time"

	"github.com/edwinhuish/gowork/internal/pkg/money"
)

// SalesOrderQuery 是销售订单列表的查询条件。
type SalesOrderQuery struct {
	PageQuery
	PartnerID *uint `form:"partner_id"`
	Status    *int8 `form:"status"`
}

// SalesOrderItemInput 是一条订单明细：只给数量与单价。
type SalesOrderItemInput struct {
	ProductID uint `json:"product_id" binding:"required"`
	// Qty 定点收发。binding 的 gt 对结构体不生效，"必须大于 0"由服务层判。
	Qty money.Decimal `json:"qty"`
	// UnitPrice 按字符串收发（money.Decimal）。
	UnitPrice money.Decimal `json:"unit_price"`
	Remark    string        `json:"remark" binding:"max=255"`
}

// SalesOrderInput 新建与编辑共用。
//
// 这里**故意没有** amount_total / amount_base_total 字段：合计只由 service 从明细汇总。
// 前端即使把总额塞进请求体，绑定阶段就会被丢弃——比"接收了再判空"更彻底，
// 因为没有任何代码路径能读到它，也就不可能有人哪天一顺手把它存进库。
type SalesOrderInput struct {
	PartnerID    uint   `json:"partner_id" binding:"required"`
	// TradeTerm 不再必填：留空取客户默认（md_partner.trade_term）。
	TradeTerm    string `json:"trade_term" binding:"max=8"`
	CurrencyCode string `json:"currency_code" binding:"max=8"`
	// OrderDate 下单日，YYYY-MM-DD；留空为今天。
	OrderDate string `json:"order_date"`
	// DeliveryDate 约定交期，可留空。
	DeliveryDate string                `json:"delivery_date"`
	Remark       string                `json:"remark" binding:"max=255"`
	Items        []SalesOrderItemInput `json:"items" binding:"required,min=1,dive"`
	// UpdatedAt 是编辑页打开时读到的版本；带上它做条件更新，期间被别人改过则 409。
	UpdatedAt *time.Time `json:"updated_at"`
}

// SalesOrderStatusInput 走完整的单据状态机，具体能不能跳由 service 判定。
type SalesOrderStatusInput struct {
	Status int8 `json:"status" binding:"required"`
	// ForceCredit 在确认（10→20）时强制越过客户信用额度。
	//
	// 只有 admin 能用：service 会看请求上下文里的身份，非 admin 传了返回 403，
	// 放行时落一条 warn 日志留痕。它不改变订单的任何金额。
	ForceCredit bool `json:"force_credit"`
}
