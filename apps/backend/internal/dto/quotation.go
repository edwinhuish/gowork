package dto

import (
	"time"

	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/money"
)

// QuotationQuery 是报价单列表的查询条件。
type QuotationQuery struct {
	PageQuery
	PartnerID *uint `form:"partner_id"`
	Status    *int8 `form:"status"`
}

// QuotationItemInput 是一条报价明细。
//
// 没有 amount / cost_price 这类字段：金额由 service 按「数量 × 单价 × 单据汇率」算，
// 成本从货源取。前端提交合计一律不被信任——"改了明细没改总额"是必然会发生的事。
type QuotationItemInput struct {
	ProductID uint `json:"product_id" binding:"required"`
	// Qty 定点收发。binding 的 gt 对结构体不生效，"必须大于 0"由服务层判。
	Qty money.Decimal `json:"qty"`
	// UnitPrice 报价币种的单价。字符串收发（money.Decimal）：
	// 单价被乘上数量、再乘汇率，进来时掉一位精度，最后就会落到合计上。
	UnitPrice money.Decimal `json:"unit_price"`
	// SourceID 指定成本取自哪条货源；留空取该产品本币价最低的启用货源。
	// 允许指定是为了"这单指定从某家进"——对方账期好、或某家正在做活动价。
	SourceID *uint  `json:"source_id"`
	Remark   string `json:"remark" binding:"max=255"`
}

// QuotationInput 新建与编辑共用。编辑只允许发生在草稿上。
type QuotationInput struct {
	PartnerID    uint   `json:"partner_id" binding:"required"`
	// TradeTerm 不再必填：留空时取客户默认（md_partner.trade_term）。
	// 取完仍为空才报错——那时说明客户档案也没有，得先补档案。
	TradeTerm    string `json:"trade_term" binding:"max=8"`
	CurrencyCode string `json:"currency_code" binding:"max=8"`
	// ValidUntil 有效期截止日，YYYY-MM-DD，含当天。
	ValidUntil string `json:"valid_until" binding:"required"`
	Remark     string `json:"remark" binding:"max=255"`
	// Items 至少一行：空报价单没法解释它想表达什么，也过不了 ConvertBlocker 之外的任何校验。
	Items []QuotationItemInput `json:"items" binding:"required,min=1,dive"`
	// UpdatedAt 是编辑页打开时读到的版本。带上它做条件更新：
	// 期间被别人改过就返回 409，而不是把对方的改动静默盖掉。新建时留空。
	UpdatedAt *time.Time `json:"updated_at"`
}

// QuotationStatusInput 只接受报价单真正会走到的两个状态：
// 已确认（20）与已取消（90）。备货中 / 已出货属于订单，报价单永远不该出现。
type QuotationStatusInput struct {
	Status int8 `json:"status" binding:"required"`
}

// QuotationVersionInput 另存新版本。
//
// 已经过期的报价单必须给出新的有效期：照抄旧日期会立刻造出一版"存下来就不能确认"的单。
type QuotationVersionInput struct {
	ValidUntil string `json:"valid_until"`
}

// QuotationConvertInput 是转订单时的可选覆盖项。
//
// 客户、币种、汇率、单价、数量一律取报价单上的值，不接受覆盖：
// 转订单是"把已经谈好的那张纸变成要出货的单据"，任何一项可以改，
// 就变成"转单时顺手改了价"而没人发现。
type QuotationConvertInput struct {
	// OrderDate 下单日，留空取今天。
	OrderDate string `json:"order_date"`
	// DeliveryDate 约定交期，可留空。
	DeliveryDate string `json:"delivery_date"`
}

// QuotationPreviewInput 是"算一遍但不保存"的入参：报价时先看毛利再决定报不报。
type QuotationPreviewInput struct {
	CurrencyCode string               `json:"currency_code" binding:"max=8"`
	Items        []QuotationItemInput `json:"items" binding:"required,min=1,dive"`
}

// QuotationPreviewItem 是试算结果的一行，含成本来源，便于判断"这个价按谁家算的"。
type QuotationPreviewItem struct {
	ProductID   uint          `json:"product_id"`
	SKUCode     string        `json:"sku_code"`
	NameCN      string        `json:"name_cn"`
	Unit        string        `json:"unit"`
	Qty         money.Decimal `json:"qty"`
	UnitPrice   money.Decimal `json:"unit_price"`
	Amount      money.Decimal `json:"amount"`
	AmountBase  money.Decimal `json:"amount_base"`
	CostPrice   money.Decimal `json:"cost_price"`
	CostAmount  money.Decimal `json:"cost_amount"`
	GrossProfit money.Decimal `json:"gross_profit"`
	SourceID    *uint         `json:"source_id"`
	SourceName  string        `json:"source_name"`
}

// QuotationPreview 是试算汇总，口径与保存后落库的完全一致（同一个折算函数）。
type QuotationPreview struct {
	CurrencyCode    string                 `json:"currency_code"`
	ExchangeRate    money.Decimal          `json:"exchange_rate"`
	Items           []QuotationPreviewItem `json:"items"`
	AmountTotal     money.Decimal          `json:"amount_total"`
	AmountBaseTotal money.Decimal          `json:"amount_base_total"`
	CostTotal       money.Decimal          `json:"cost_total"`
	GrossProfit     money.Decimal          `json:"gross_profit"`
	MarginRate      money.Decimal          `json:"margin_rate"`
}

// QuotationDetail 在单据本体上补两个计算字段。
//
// expired 与 convert_block 由服务端给出而不是前端自己比日期：时区与"截止日是否含当天"
// 一旦各算一套，就会出现"列表说已过期、详情说还能转"。
type QuotationDetail struct {
	*model.Quotation
	Expired      bool   `json:"expired"`
	ConvertBlock string `json:"convert_block,omitempty"`
}
