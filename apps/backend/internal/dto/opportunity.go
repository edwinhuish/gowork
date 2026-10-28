package dto

// OpportunityQuery 是商机列表的查询条件。
type OpportunityQuery struct {
	PageQuery
	PartnerID *uint `form:"partner_id"`
	Stage     *int8 `form:"stage"`
	// OpenOnly=1 只看未结商机（排除赢单输单）。
	OpenOnly *int8 `form:"open_only"`
}

// OpportunityStageSummary 是按阶段聚合的一行。金额一律本币：
// 把 USD 与 CNY 的原币金额相加没有意义。
type OpportunityStageSummary struct {
	Stage      int8    `json:"stage"`
	Count      int64   `json:"count"`
	AmountBase float64 `json:"amount_base"`
	Weighted   float64 `json:"weighted"`
}

// OpportunitySummary 是商机看板的汇总结果。
//
// 各阶段金额之和未必等于 TotalAmountBase —— 前者按阶段分，后者是全部；
// 中间差的那部分只可能是终态商机，所以两者都返回，方便前端核对。
type OpportunitySummary struct {
	Stages          []OpportunityStageSummary `json:"stages"`
	TotalCount      int64                     `json:"total_count"`
	TotalAmountBase float64                   `json:"total_amount_base"`
	TotalWeighted   float64                   `json:"total_weighted"`
}

// OpportunityInput 新建与编辑共用。
type OpportunityInput struct {
	PartnerID uint    `json:"partner_id" binding:"required"`
	Subject   string  `json:"subject" binding:"required,max=128"`
	Stage     int8    `json:"stage" binding:"required"`
	Amount    float64 `json:"amount"`
	// CurrencyCode 留空则用本币。
	CurrencyCode string `json:"currency_code" binding:"max=8"`
	// Probability 留空（nil）时按阶段取默认值；赢单/输单会被强制成 100/0。
	Probability       *int   `json:"probability"`
	ExpectedCloseDate string `json:"expected_close_date" binding:"required,max=32"`
	Remark            string `json:"remark" binding:"max=255"`
}
