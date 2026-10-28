package dto

import "github.com/edwinhuish/gowork/internal/model"

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
	ProductID uint    `json:"product_id" binding:"required"`
	Qty       float64 `json:"qty" binding:"required,gt=0"`
	// UnitPrice 报价币种的单价。
	UnitPrice float64 `json:"unit_price" binding:"gte=0"`
	// SourceID 指定成本取自哪条货源；留空取该产品本币价最低的启用货源。
	// 允许指定是为了"这单指定从某家进"——对方账期好、或某家正在做活动价。
	SourceID *uint  `json:"source_id"`
	Remark   string `json:"remark" binding:"max=255"`
}

// QuotationInput 新建与编辑共用。编辑只允许发生在草稿上。
type QuotationInput struct {
	PartnerID    uint   `json:"partner_id" binding:"required"`
	TradeTerm    string `json:"trade_term" binding:"required,max=8"`
	CurrencyCode string `json:"currency_code" binding:"max=8"`
	// ValidUntil 有效期截止日，YYYY-MM-DD，含当天。
	ValidUntil string `json:"valid_until" binding:"required"`
	Remark     string `json:"remark" binding:"max=255"`
	// Items 至少一行：空报价单没法解释它想表达什么，也过不了 ConvertBlocker 之外的任何校验。
	Items []QuotationItemInput `json:"items" binding:"required,min=1,dive"`
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

// QuotationPreviewInput 是"算一遍但不保存"的入参：报价时先看毛利再决定报不报。
type QuotationPreviewInput struct {
	CurrencyCode string               `json:"currency_code" binding:"max=8"`
	Items        []QuotationItemInput `json:"items" binding:"required,min=1,dive"`
}

// QuotationPreviewItem 是试算结果的一行，含成本来源，便于判断"这个价按谁家算的"。
type QuotationPreviewItem struct {
	ProductID   uint    `json:"product_id"`
	SKUCode     string  `json:"sku_code"`
	NameCN      string  `json:"name_cn"`
	Unit        string  `json:"unit"`
	Qty         float64 `json:"qty"`
	UnitPrice   float64 `json:"unit_price"`
	Amount      float64 `json:"amount"`
	AmountBase  float64 `json:"amount_base"`
	CostPrice   float64 `json:"cost_price"`
	CostAmount  float64 `json:"cost_amount"`
	GrossProfit float64 `json:"gross_profit"`
	SourceID    *uint   `json:"source_id"`
	SourceName  string  `json:"source_name"`
}

// QuotationPreview 是试算汇总，口径与保存后落库的完全一致（同一个折算函数）。
type QuotationPreview struct {
	CurrencyCode    string                 `json:"currency_code"`
	ExchangeRate    float64                `json:"exchange_rate"`
	Items           []QuotationPreviewItem `json:"items"`
	AmountTotal     float64                `json:"amount_total"`
	AmountBaseTotal float64                `json:"amount_base_total"`
	CostTotal       float64                `json:"cost_total"`
	GrossProfit     float64                `json:"gross_profit"`
	MarginRate      float64                `json:"margin_rate"`
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
