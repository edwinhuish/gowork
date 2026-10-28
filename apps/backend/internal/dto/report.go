package dto

// AgingQuery 是账龄查询的条件。
type AgingQuery struct {
	// Direction 10 应收 / 20 应付，留空按应收。
	Direction *int8 `form:"direction"`
	// PartnerID 只看一个主体，留空看全部。
	PartnerID *uint `form:"partner_id"`
	// AsOf 是"截止到哪一天"，留空取今天。所有分桶都相对它算，
	// 所以同一批数据在不同 as_of 下会落到不同的桶——这是账龄的本意。
	AsOf string `form:"as_of"`
}

// AgingBuckets 是按逾期天数分桶的金额。
//
// 下标约定：0 未到期 / 1 逾期 1-30 天 / 2 31-60 天 / 3 61-90 天 / 4 90 天以上。
// 分桶规则只应有一份（service 的 agingBucket），这里只按下标累加：
// 让 DTO 自己判断"30 天算哪一桶"，规则就有了第二个版本。
type AgingBuckets struct {
	NotDue     float64 `json:"not_due"`
	Days1To30  float64 `json:"days_1_30"`
	Days31To60 float64 `json:"days_31_60"`
	Days61To90 float64 `json:"days_61_90"`
	Over90     float64 `json:"days_over_90"`
}

// Add 按下标累加一个金额。下标超出范围时并入 90+：
// 宁可让异常值出现在最老的一桶里被看见，也不要静默丢掉一笔钱。
func (b *AgingBuckets) Add(bucket int, v float64) {
	switch bucket {
	case 0:
		b.NotDue += v
	case 1:
		b.Days1To30 += v
	case 2:
		b.Days31To60 += v
	case 3:
		b.Days61To90 += v
	default:
		b.Over90 += v
	}
}

// Total 是各桶之和。
func (b AgingBuckets) Total() float64 {
	return b.NotDue + b.Days1To30 + b.Days31To60 + b.Days61To90 + b.Over90
}

// AgingRow 是一个「主体 + 币种」的账龄。
//
// 为什么按币种拆行而不是只按主体：同一客户可能一半美元一半人民币，
// 把两者的原币金额加在一起是个没有意义的数。本币金额（AmountBase）
// 可以跨币种相加，所以合计只给本币。
type AgingRow struct {
	PartnerID    uint   `json:"partner_id"`
	PartnerCode  string `json:"partner_code"`
	PartnerName  string `json:"partner_name"`
	CurrencyCode string `json:"currency_code"`

	// Amount 是未核销的原币金额分桶，AmountBase 是同样口径的本币金额。
	Amount     AgingBuckets `json:"amount"`
	AmountBase AgingBuckets `json:"amount_base"`

	Total     float64 `json:"total"`
	TotalBase float64 `json:"total_base"`
	// EarliestDueDate 是这一行里最早到期的那张单，催款从这里开始。
	EarliestDueDate string `json:"earliest_due_date"`
}

// AgingReport 是账龄报表的响应体。
type AgingReport struct {
	// Direction 10 应收 / 20 应付。
	Direction int8       `json:"direction"`
	AsOf      string     `json:"as_of"`
	Rows      []AgingRow `json:"rows"`
	// TotalBase 是全表本币分桶合计；只给本币，理由见 AgingRow。
	TotalBase      AgingBuckets `json:"total_base"`
	GrandTotalBase float64      `json:"grand_total_base"`
}
