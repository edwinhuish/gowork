package dto

import "github.com/edwinhuish/gowork/internal/pkg/money"

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
	NotDue     money.Decimal `json:"not_due"`
	Days1To30  money.Decimal `json:"days_1_30"`
	Days31To60 money.Decimal `json:"days_31_60"`
	Days61To90 money.Decimal `json:"days_61_90"`
	Over90     money.Decimal `json:"days_over_90"`
}

// Add 按下标累加一个金额。下标超出范围时并入 90+：
// 宁可让异常值出现在最老的一桶里被看见，也不要静默丢掉一笔钱。
func (b *AgingBuckets) Add(bucket int, v money.Decimal) {
	switch bucket {
	case 0:
		b.NotDue = b.NotDue.Add(v)
	case 1:
		b.Days1To30 = b.Days1To30.Add(v)
	case 2:
		b.Days31To60 = b.Days31To60.Add(v)
	case 3:
		b.Days61To90 = b.Days61To90.Add(v)
	default:
		b.Over90 = b.Over90.Add(v)
	}
}

// Total 是各桶之和。
func (b AgingBuckets) Total() money.Decimal {
	return b.NotDue.Add(b.Days1To30).Add(b.Days31To60).Add(b.Days61To90).Add(b.Over90)
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

	Total     money.Decimal `json:"total"`
	TotalBase money.Decimal `json:"total_base"`
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
	TotalBase      AgingBuckets  `json:"total_base"`
	GrandTotalBase money.Decimal `json:"grand_total_base"`
}

// ShipmentProfitQuery 是单票毛利的查询条件。
type ShipmentProfitQuery struct {
	PageQuery
	// ShipmentID 只看一票：出货详情页的面板只需要这一票。
	ShipmentID *uint `form:"shipment_id"`
}

// ShipmentProfit 是一票货的毛利。金额全部是**本币**：
// 外币收入与人民币成本只有折到同一个币种才能相减。
//
// 口径：毛利 = 收入 − 采购成本 − 运杂费。**汇兑损益不在这里**——它由 T-23
// 单独成行，混进毛利会让同一笔差额在两张报表里各出现一次。
//
// 收入按"本票数量 ÷ 销售行数量"分摊该行的本币金额；成本按该销售行累计采购的
// 加权单价 × 本票数量。两者都取单据上锁定的汇率，不回头查汇率表。
type ShipmentProfit struct {
	ShipmentID uint   `json:"shipment_id"`
	DocNo      string `json:"doc_no"`
	ShipDate   string `json:"ship_date"`
	Status     int8   `json:"status"`

	Revenue money.Decimal `json:"revenue"`
	Cost    money.Decimal `json:"cost"`
	Charge  money.Decimal `json:"charge"`
	Profit  money.Decimal `json:"profit"`
	// Margin 是毛利率（0.2254 = 22.54%）。收入为 0 时给 0，不给 NaN。
	Margin money.Decimal `json:"margin"`

	// CostMissing 表示这票里有销售行没有可用成本（没挂采购单，或采购单还是草稿）。
	// 此时 Cost 与 Profit 都不完整——不标出来，就等于把"没录采购"报成"毛利极高"。
	CostMissing bool `json:"cost_missing"`
}

// ExchangeGainQuery 是汇兑损益的查询条件。日期按**到账日**筛：
// 损益发生在钱动的那天，不是单据开的那天。
type ExchangeGainQuery struct {
	From         string `form:"from"`
	To           string `form:"to"`
	PartnerID    *uint  `form:"partner_id"`
	CurrencyCode string `form:"currency_code" binding:"max=8"`
}

// ExchangeGainLine 是一笔核销产生的汇兑损益。
//
// 一行 = 收付款单上的核销行：到账汇率与目标单据的开单汇率不同，
// 同一笔原币折出来的本币就不同。
type ExchangeGainLine struct {
	PaymentID    uint   `json:"payment_id"`
	PaymentDocNo string `json:"payment_doc_no"`
	PayDate      string `json:"pay_date"`
	Direction    int8   `json:"direction"`
	PartnerID    uint   `json:"partner_id"`
	PartnerCode  string `json:"partner_code"`
	PartnerName  string `json:"partner_name"`
	CurrencyCode string `json:"currency_code"`
	// TargetType 10 应收单 / 20 应付单。
	TargetType  int8   `json:"target_type"`
	TargetDocNo string `json:"target_doc_no"`

	// Amount 是核销原币；AmountBase 是按到账汇率折的本币。
	Amount     money.Decimal `json:"amount"`
	AmountBase money.Decimal `json:"amount_base"`
	// PaymentRate 是到账汇率，DocumentRate 是目标单据锁定的汇率。
	PaymentRate  money.Decimal `json:"payment_rate"`
	DocumentRate money.Decimal `json:"document_rate"`
	// DocumentBase 是按单据汇率折出来的同一笔原币，GainBase = ±(AmountBase − DocumentBase)。
	DocumentBase money.Decimal `json:"document_base"`
	// GainBase 正数是收益、负数是损失，单位本币。
	GainBase money.Decimal `json:"gain_base"`
}

// ExchangeGainSubtotal 是按币种的小计。原币只有同币种能加，本币可以跨币种加，
// 所以两边各给一份：原币小计用来看"这个币种动了多少"，本币总计才是账上的数。
type ExchangeGainSubtotal struct {
	CurrencyCode string        `json:"currency_code"`
	Amount       money.Decimal `json:"amount"`
	GainBase     money.Decimal `json:"gain_base"`
}

// ExchangeGainReport 是汇兑损益报表。
type ExchangeGainReport struct {
	From string `json:"from"`
	To   string `json:"to"`
	// Lines 逐笔可核对；Subtotals 按币种；TotalGainBase 是全部折本币后的合计。
	Lines         []ExchangeGainLine     `json:"lines"`
	Subtotals     []ExchangeGainSubtotal `json:"subtotals"`
	TotalGainBase money.Decimal          `json:"total_gain_base"`
}

// SalesDashboardQuery 是销售看板的查询条件。
type SalesDashboardQuery struct {
	// Dimension 取 customer / product / country / month，留空按 customer。
	Dimension string `form:"dimension" binding:"max=16"`
	// From / To 是闭区间的 YYYY-MM-DD，留空表示不限。
	// 出货毛利按出货日、订单额按下单日各自筛，见 SalesDashboardRow 的说明。
	From string `form:"from" binding:"max=10"`
	To   string `form:"to" binding:"max=10"`
	// Limit 是返回行数上限，留空取 20；0 表示不截断（内部上限 500）。
	Limit *int `form:"limit"`
}

// SalesDashboardRow 是看板的一行：同一维度键下的金额汇总。
//
// 两个口径并排放在一行里，但**不是同一批单据**：
// OrderAmount 是按下单日统计的订单金额，Revenue 是出货日在范围内那部分货的收入。
// 两者相减没有意义，所以列名与页头说明都要写清各自的时间口径。
type SalesDashboardRow struct {
	Key   string `json:"key"`
	Label string `json:"label"`

	OrderAmount money.Decimal `json:"order_amount"`
	Revenue     money.Decimal `json:"revenue"`
	Cost        money.Decimal `json:"cost"`
	Charge      money.Decimal `json:"charge"`
	Profit      money.Decimal `json:"profit"`
	// Margin 是毛利率（0.2254 = 22.54%）。收入为 0 时给 0，不给 NaN。
	Margin money.Decimal `json:"margin"`

	// Shipments 是这一行涉及的出货票数（去重），用来看"这个数是几票堆出来的"。
	Shipments int `json:"shipments"`
	// CostMissing 表示这一行里有出货行没有可用成本（没挂采购单，或采购单还是草稿）。
	// 不标出来，等于把"没录采购"报成"毛利极高"。
	CostMissing bool `json:"cost_missing"`
}

// SalesDashboard 是销售看板的响应体。
type SalesDashboard struct {
	Dimension string `json:"dimension"`
	From      string `json:"from"`
	To        string `json:"to"`

	Rows []SalesDashboardRow `json:"rows"`
	// Total 是**全量**合计，不受 limit 截断影响——截断后拿 Rows 自己加会少算。
	Total SalesDashboardRow `json:"total"`
	// Truncated 表示 Rows 被 limit 截断过。
	Truncated bool `json:"truncated"`
}
