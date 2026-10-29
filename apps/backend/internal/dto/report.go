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
	// CostHidden 表示成本 / 运杂费 / 毛利被服务端清掉了（T-36：业务员看不到毛利）。
	// 收入仍然给：业务员要知道自己卖了多少，只是不知道赚了多少。
	CostHidden bool `json:"cost_hidden"`
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

// ProductProfitQuery 是产品毛利下钻的查询条件。
type ProductProfitQuery struct {
	ProductID uint   `form:"product_id" binding:"required"`
	From      string `form:"from" binding:"max=10"`
	To        string `form:"to" binding:"max=10"`
}

// ProductProfitRow 是某个产品在**一票**货里的获利情况。
// 按票聚合而不是按订单行：分批出货时同一产品在一票里可能出现两次，
// 拆成两行就看不出"这一票赚了多少"。
type ProductProfitRow struct {
	ShipmentID  uint          `json:"shipment_id"`
	DocNo       string        `json:"doc_no"`
	ShipDate    string        `json:"ship_date"`
	PartnerID   uint          `json:"partner_id"`
	PartnerName string        `json:"partner_name"`
	Qty         money.Decimal `json:"qty"`
	Revenue     money.Decimal `json:"revenue"`
	Cost        money.Decimal `json:"cost"`
	Charge      money.Decimal `json:"charge"`
	// ChargeByAmount 是按金额分摊的对照值（T-59）：默认口径下与 Charge 相同，
	// 换过分摊方式之后并排看，才知道"换口径会差多少"。
	ChargeByAmount money.Decimal `json:"charge_by_amount"`
	Profit         money.Decimal `json:"profit"`
	// CostMissing 表示这一票里有出货行没有可用采购成本，成本与毛利因此偏高。
	CostMissing bool `json:"cost_missing"`
}

// ProductProfitReport 是产品毛利下钻的结果，行按出货日倒序。
type ProductProfitReport struct {
	ProductID uint   `json:"product_id"`
	From      string `json:"from"`
	To        string `json:"to"`

	Rows []ProductProfitRow `json:"rows"`

	Revenue     money.Decimal `json:"revenue"`
	Cost        money.Decimal `json:"cost"`
	Charge      money.Decimal `json:"charge"`
	Profit      money.Decimal `json:"profit"`
	Margin      money.Decimal `json:"margin"`
	CostMissing bool          `json:"cost_missing"`
	// CostHidden 表示成本 / 运杂费 / 毛利 / 毛利率被服务端清掉了（T-36）。
	CostHidden bool `json:"cost_hidden"`
}

// TodoQuery 是待办清单的查询条件。
type TodoQuery struct {
	// Limit 是每类清单的行数上限，留空 20；0 取内部上限 100。
	Limit *int `form:"limit"`
	// AsOf 是"今天"，留空取服务器当天。放到参数里是为了测试能钉住日期。
	AsOf string `form:"as_of" binding:"max=10"`
}

// TodoFollowup 是一条该跟进的记录。
type TodoFollowup struct {
	ID           uint   `json:"id"`
	PartnerID    uint   `json:"partner_id"`
	PartnerName  string `json:"partner_name"`
	Subject      string `json:"subject"`
	NextFollowAt string `json:"next_follow_at"`
	// OverdueDays 是超过下次跟进日的天数。
	OverdueDays int `json:"overdue_days"`
}

// TodoReceivable 是一张该催的应收。
type TodoReceivable struct {
	ID              uint          `json:"id"`
	DocNo           string        `json:"doc_no"`
	PartnerID       uint          `json:"partner_id"`
	PartnerName     string        `json:"partner_name"`
	DueDate         string        `json:"due_date"`
	OutstandingBase money.Decimal `json:"outstanding_base"`
	OverdueDays     int           `json:"overdue_days"`
}

// TodoCurrency 是一个今天没有汇率的外币。
type TodoCurrency struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

// TodoReport 是待办清单。
//
// 每类都给「全量条数 + 前 N 条」：条数是"还剩多少事没做"，
// 清单是"先做哪几件"——只给清单会让人以为做完就没事了。
// TodoInspection 是一条已过计划日、还没做完成的验货节点（T-62）。
type TodoInspection struct {
	ID              uint   `json:"id"`
	PurchaseOrderID uint   `json:"purchase_order_id"`
	DocNo           string `json:"doc_no"`
	PartnerName     string `json:"partner_name"`
	SKUCode         string `json:"sku_code"`
	PlanDate        string `json:"plan_date"`
	OverdueDays     int    `json:"overdue_days"`
}

type TodoReport struct {
	AsOf string `json:"as_of"`

	Inspections     []TodoInspection `json:"inspections"`
	InspectionCount int64            `json:"inspection_count"`

	Followups     []TodoFollowup `json:"followups"`
	FollowupCount int64          `json:"followup_count"`

	Receivables     []TodoReceivable `json:"receivables"`
	ReceivableCount int64            `json:"receivable_count"`

	// MissingRates 是今天还没有汇率的外币，没有上限（币种总量本来就少）。
	MissingRates []TodoCurrency `json:"missing_rates"`
}

// Partner360Query 是客户 360 的查询条件。
type Partner360Query struct {
	PartnerID uint `form:"partner_id" binding:"required"`
	// Limit 是每个列表的行数上限，留空 10；0 取内部上限 50。
	Limit *int `form:"limit"`
	// AsOf 是超期判断的截止日，留空取今天。
	AsOf string `form:"as_of" binding:"max=10"`
}

// Partner360Totals 是一个主体的关键数字。金额全部本币（外币之间只有折本币才能相加）。
type Partner360Totals struct {
	OrderCount      int64         `json:"order_count"`
	OrderAmountBase money.Decimal `json:"order_amount_base"`
	LastOrderDate   string        `json:"last_order_date"`

	// ShipmentCount 是**含该客户订单行**的出货票数（去重）：拼柜时一票里还有别的客户的货。
	ShipmentCount int64 `json:"shipment_count"`

	ActivityCount  int64  `json:"activity_count"`
	LastActivityAt string `json:"last_activity_at"`

	// 商机只算未终结的：已赢的变成订单了，已输的不该再占预测。
	OpenOpportunityCount      int64         `json:"open_opportunity_count"`
	OpenOpportunityAmountBase money.Decimal `json:"open_opportunity_amount_base"`

	ReceivableOutstandingBase money.Decimal `json:"receivable_outstanding_base"`
	ReceivableOverdueBase     money.Decimal `json:"receivable_overdue_base"`
}

type Partner360Activity struct {
	ID           uint   `json:"id"`
	Type         string `json:"type"`
	Subject      string `json:"subject"`
	OccurredAt   string `json:"occurred_at"`
	NextFollowAt string `json:"next_follow_at"`
}

type Partner360Opportunity struct {
	ID                uint          `json:"id"`
	Subject           string        `json:"subject"`
	Stage             int8          `json:"stage"`
	AmountBase        money.Decimal `json:"amount_base"`
	Probability       int           `json:"probability"`
	ExpectedCloseDate string        `json:"expected_close_date"`
}

type Partner360Order struct {
	ID              uint          `json:"id"`
	DocNo           string        `json:"doc_no"`
	OrderDate       string        `json:"order_date"`
	Status          int8          `json:"status"`
	CurrencyCode    string        `json:"currency_code"`
	AmountBaseTotal money.Decimal `json:"amount_base_total"`
}

type Partner360Shipment struct {
	ID       uint   `json:"id"`
	DocNo    string `json:"doc_no"`
	ShipDate string `json:"ship_date"`
	Status   int8   `json:"status"`
}

type Partner360Receivable struct {
	ID              uint          `json:"id"`
	DocNo           string        `json:"doc_no"`
	CurrencyCode    string        `json:"currency_code"`
	DueDate         string        `json:"due_date"`
	Outstanding     money.Decimal `json:"outstanding"`
	OutstandingBase money.Decimal `json:"outstanding_base"`
	OverdueDays     int           `json:"overdue_days"`
}

// Partner360 是一个主体的全貌：抬头数字 + 五块明细（各带 id 与单号，能点进源单据）。
//
// 每块都**截断**（默认 10 条）：这一页是用来判断"接下来该跟哪一条"的，
// 全量明细去各自的列表页看。合计（Totals）永远是全量，不受截断影响。
type Partner360 struct {
	PartnerID   uint   `json:"partner_id"`
	PartnerCode string `json:"partner_code"`
	PartnerName string `json:"partner_name"`
	Country     string `json:"country"`
	// Role 是角色位掩码：同一家可能既是客户又是供应商。
	Role int8   `json:"role"`
	AsOf string `json:"as_of"`

	Totals Partner360Totals `json:"totals"`

	Activities    []Partner360Activity    `json:"activities"`
	Opportunities []Partner360Opportunity `json:"opportunities"`
	Orders        []Partner360Order       `json:"orders"`
	Shipments     []Partner360Shipment    `json:"shipments"`
	Receivables   []Partner360Receivable  `json:"receivables"`
}

// CollectionQuery 是回款与超期报表的查询条件。
type CollectionQuery struct {
	// Direction 10 应收回款 / 20 应付付款，留空按应收。
	Direction *int8 `form:"direction"`
	// From / To 是回款率的统计期间，闭区间，按**到期日**筛。
	From string `form:"from" binding:"max=10"`
	To   string `form:"to" binding:"max=10"`
	// AsOf 是超期明细的截止日，留空取今天。逾期天数相对它算。
	AsOf string `form:"as_of" binding:"max=10"`
	// Limit 是超期明细的行数上限，留空 50；0 取内部上限 500。
	Limit *int `form:"limit"`
}

// CollectionRow 是一张**已逾期**且尚未结清的单据。
//
// 逐笔而不是按客户聚合：催款是按单催的，客户欠三张单时只说"他欠 8 万"
// 没法决定先催哪一张。
type CollectionRow struct {
	ID           uint   `json:"id"`
	DocNo        string `json:"doc_no"`
	PartnerID    uint   `json:"partner_id"`
	PartnerCode  string `json:"partner_code"`
	PartnerName  string `json:"partner_name"`
	CurrencyCode string `json:"currency_code"`
	DueDate      string `json:"due_date"`
	OverdueDays  int    `json:"overdue_days"`

	// Outstanding 是未核销的原币金额，OutstandingBase 是它的本币金额。
	Outstanding     money.Decimal `json:"outstanding"`
	OutstandingBase money.Decimal `json:"outstanding_base"`
}

// CollectionReport 是回款率与超期明细。
//
// 两个数在一张表里但要分开读：**期间回款率**回答"这段时间该收的收回来多少"，
// 分子按到账日、分母按到期日；**超期明细**回答"今天还欠着哪几笔"，
// 是截至 as_of 的存量。前者是流量、后者是存量，不能互相校验。
type CollectionReport struct {
	Direction int8   `json:"direction"`
	From      string `json:"from"`
	To        string `json:"to"`
	AsOf      string `json:"as_of"`

	// DueAmountBase 是期间内到期的单据金额（本币），ReceivedBase 是期间内到账并核销的金额。
	DueAmountBase money.Decimal `json:"due_amount_base"`
	ReceivedBase  money.Decimal `json:"received_base"`
	// CollectionRate = ReceivedBase / DueAmountBase，分母为 0 时给 0。
	CollectionRate money.Decimal `json:"collection_rate"`

	// OutstandingBase 是截至 as_of 仍未核销的总额（含未到期）。
	OutstandingBase money.Decimal `json:"outstanding_base"`
	// OverdueBase 是其中已逾期的那部分，OverdueBuckets 是它的账龄分桶
	// （与账龄报表同一套规则，NotDue 恒为 0）。
	OverdueBase    money.Decimal `json:"overdue_base"`
	OverdueBuckets AgingBuckets  `json:"overdue_buckets"`

	// OverdueRows 按逾期天数从多到少：最该催的排最前面。
	OverdueRows []CollectionRow `json:"overdue_rows"`
	// Truncated 表示明细被 limit 截断，OverdueBase 与分桶仍是全量。
	Truncated bool `json:"truncated"`
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
	// CostHidden 表示成本 / 运杂费 / 毛利 / 毛利率被服务端清掉了（T-36）。
	// 订单额与收入照给：业务员要先知道自己卖了多少。
	CostHidden bool `json:"cost_hidden"`
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
