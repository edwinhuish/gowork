package dto

// CurrencyQuery 是币种列表的查询条件。
type CurrencyQuery struct {
	PageQuery
	Status *int8 `form:"status"`
}

// CurrencyInput 新建与编辑共用。
//
// Code 允许提交但只在新建时生效：它是业务身份（单据上存的就是这个代码），
// 改了它，引用它的历史汇率与单据会指向一个不存在的币种。
type CurrencyInput struct {
	Code          string            `json:"code" binding:"required,max=8"`
	Name          string            `json:"name" binding:"required,max=64"`
	Symbol        string            `json:"symbol" binding:"max=8"`
	DecimalPlaces *int              `json:"decimal_places"`
	IsBase        *bool             `json:"is_base"`
	Status        EnableStatusInput `json:"status"`
	Remark        string            `json:"remark" binding:"max=255"`
}

// ExchangeRateQuery 是汇率列表的查询条件。
type ExchangeRateQuery struct {
	PageQuery
	CurrencyCode string `form:"currency_code"`
	BaseCode     string `form:"base_code"`
}

// ExchangeRateInput 新增汇率。
//
// 汇率没有编辑入口：改一条历史汇率会让已经生成的单据按新价重算，
// 那笔生意当时到底按什么价成交就再也说不清了。录错了就删掉重录。
type ExchangeRateInput struct {
	CurrencyCode string `json:"currency_code" binding:"required,max=8"`
	// Rate 是「1 单位外币 = Rate 单位本币」。binding 的 required
	// 对数值类型等价于"不能为 0"，正好挡住最常见的漏填。
	Rate float64 `json:"rate" binding:"required"`
	// EffectiveDate 只认日期，格式 YYYY-MM-DD。
	EffectiveDate string `json:"effective_date" binding:"required,max=32"`
	Source        string `json:"source" binding:"max=32"`
	Remark        string `json:"remark" binding:"max=255"`
}
