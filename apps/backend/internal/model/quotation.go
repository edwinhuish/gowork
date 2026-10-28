package model

import "time"

// Quotation 是对客户发出的价格方案：主链上第一张真正的单据。
//
// 与商机的区别在"承诺"二字：商机是内部预测，金额可以随汇率重算；
// 报价单发出之后，客户按它下单、按它开信用证，所以**金额、汇率、成本一律锁定**，
// 供应商明天涨价不该让三个月前那张报价单的毛利变成另一个数。
//
// 版本：同一份报价可以谈多轮。多轮共用同一个 DocNo，只递增 Version，
// 于是"QT-202610-0001 的第 3 版"在邮件里能被唯一指认；每版是独立的行，
// 只允许草稿可改，已确认的那版永远保持发给客户时的样子。
type Quotation struct {
	Base
	// DocNo 与 Version 联合唯一：同一份报价的一个版本只能存在一条。
	DocNo   string `gorm:"size:32;not null;uniqueIndex:uni_quotation_doc_version,priority:1" json:"doc_no"`
	Version int    `gorm:"not null;default:1;uniqueIndex:uni_quotation_doc_version,priority:2" json:"version"`
	// RootID 指向第 1 版；第 1 版自身为 0（它就是根）。
	// 不用 parent_id：链式结构要递归才能取出全部版本，而"这份报价谈了几轮"是列表要用的。
	RootID uint `gorm:"index;not null;default:0" json:"root_id"`

	PartnerID uint           `gorm:"index;not null" json:"partner_id"`
	TradeTerm TradeTerm      `gorm:"size:8;not null" json:"trade_term"`
	Status    DocumentStatus `gorm:"not null;index" json:"status"`

	// 报价币种与折算率。汇率在创建时定死：有效期内的价格对应的是发出那天的人民币成本。
	CurrencyCode string  `gorm:"size:8;not null" json:"currency_code"`
	ExchangeRate float64 `gorm:"type:decimal(18,8);not null;default:1" json:"exchange_rate"`
	// ValidUntil 有效期截止日（含当天）。过期后不可转订单——客户拿着去年的价来下单，
	// 而原料已经涨过，那不是折扣而是亏损。
	ValidUntil time.Time `gorm:"type:date;index;not null" json:"valid_until"`

	// SalesOrderID / SalesOrderDocNo 是转订单后回填的引用（一张报价最多转成一张订单）。
	//
	// 只存 id 与单号快照、不做双向约束：订单是独立单据，报价单只是它的来源说明；
	// 用外键把两者绑死会让"删订单"变成"要连报价一起动"。
	// 这一列同时是幂等闸门：回填走条件更新（WHERE sales_order_id IS NULL），
	// 两个人同时点"转订单"时只会有一个成功。
	SalesOrderID    *uint  `gorm:"index" json:"sales_order_id"`
	SalesOrderDocNo string `gorm:"size:32" json:"sales_order_doc_no"`

	// 以下合计是写入时一次算好的锁定快照，不随货源或汇率变动重算。
	// 这与"派生数据不加列"不冲突：单据上的合计是事实的一部分（客户看到的数字），
	// 派生数据指的是可以由明细随时聚合、且必须实时一致的中间结果。
	AmountTotal     float64 `gorm:"type:decimal(16,4);not null;default:0" json:"amount_total"`
	AmountBaseTotal float64 `gorm:"type:decimal(16,4);not null;default:0" json:"amount_base_total"`
	CostTotal       float64 `gorm:"type:decimal(16,4);not null;default:0" json:"cost_total"`
	GrossProfit     float64 `gorm:"type:decimal(16,4);not null;default:0" json:"gross_profit"`
	// MarginRate 毛利率，0.235 表示 23.5%。存比例而不是百分数：
	// 百分数在计算里每次都要除 100，漏一处就是差一百倍。
	MarginRate float64 `gorm:"type:decimal(9,6);not null;default:0" json:"margin_rate"`

	Remark string `gorm:"size:255" json:"remark"`

	Partner *Partner        `gorm:"foreignKey:PartnerID" json:"partner,omitempty"`
	Items   []QuotationItem `gorm:"foreignKey:QuotationID" json:"items,omitempty"`
}

func (Quotation) TableName() string { return "biz_quotation" }

// IsEditable 报告这一版还能不能改。只有草稿能改：已确认的那一版是发给客户的承诺，
// 改它等于改历史；要改就另存新版本。
func (q Quotation) IsEditable() bool { return q.Status == DocumentStatusDraft }

// IsExpired 报告报价是否已过有效期（按整天比较，截止日当天仍然有效）。
func (q Quotation) IsExpired(now time.Time) bool {
	return DateOnly(now).After(q.ValidUntil)
}

// ConvertBlocker 说明这张报价单为什么不能转订单；空串表示可以转。
//
// T-13 的转换接口直接用它决定放行与否，前端详情页也用它显示原因——
// 两处各写一套判断，就会出现"按钮亮了但接口拒绝"这种没人能解释的状态。
func (q Quotation) ConvertBlocker(now time.Time) string {
	// 有效期排在"还是草稿"之前：过期的草稿既确认不了（确认即承诺，价格已不成立），
	// 也就转不了单。先说"请先确认"会让人去点确认、被拒、再回来，来回两圈还是同一个坑。
	switch {
	case q.Status == DocumentStatusCancelled:
		return "报价单已取消"
	case q.SalesOrderID != nil:
		return "报价单已转为订单 " + q.SalesOrderDocNo
	case q.IsExpired(now):
		return "报价单已过有效期（截止 " + q.ValidUntil.Format("2006-01-02") + "），请另存新版本"
	case q.Status == DocumentStatusDraft:
		return "报价单还是草稿，请先确认后再转订单"
	case q.Status != DocumentStatusConfirmed:
		return "报价单当前状态不可转订单"
	default:
		return ""
	}
}

// QuotationItem 是报价明细行。
//
// 成本价与成本额是**快照**：写下这一行时从货源取当时的价格，之后不再跟着货源变。
// 供应商涨价后回看三个月前的报价，应当仍显示当时的毛利，否则"这单到底赚没赚"永远算不清。
type QuotationItem struct {
	Base
	QuotationID uint `gorm:"index;not null" json:"quotation_id"`
	ProductID   uint `gorm:"index;not null" json:"product_id"`

	// 以下三项是产品信息的快照：产品改名或改规格后，报价单上仍是发给客户的那个说法。
	SKUCode string `gorm:"size:32;not null" json:"sku_code"`
	NameCN  string `gorm:"size:128" json:"name_cn"`
	Spec    string `gorm:"size:128" json:"spec"`
	Unit    string `gorm:"size:16" json:"unit"`

	Qty float64 `gorm:"type:decimal(14,3);not null" json:"qty"`
	// UnitPrice 报价币种的单价；Amount 是它的合计（原币）。
	UnitPrice float64 `gorm:"type:decimal(14,4);not null;default:0" json:"unit_price"`
	Amount    float64 `gorm:"type:decimal(16,4);not null;default:0" json:"amount"`
	// AmountBase 按单据汇率折出的本币金额。毛利统一在本币口径算：
	// 成本来自供应商（本币口径），报价可能是任何币种，不比到同一口径就没法相减。
	AmountBase float64 `gorm:"type:decimal(16,4);not null;default:0" json:"amount_base"`

	// CostPrice 本币单位成本快照；CostAmount = CostPrice × Qty。
	CostPrice  float64 `gorm:"type:decimal(14,4);not null;default:0" json:"cost_price"`
	CostAmount float64 `gorm:"type:decimal(16,4);not null;default:0" json:"cost_amount"`

	// SourceID 记下成本取自哪条货源，事后能回答"这个价是按谁家算的"。
	SourceID *uint  `gorm:"index" json:"source_id"`
	Remark   string `gorm:"size:255" json:"remark"`

	Product *Product `gorm:"foreignKey:ProductID" json:"product,omitempty"`
}

func (QuotationItem) TableName() string { return "biz_quotation_item" }

// GrossProfit 是这一行的本币毛利。负数意味着这一行在亏，界面上要能直接看出来。
func (i QuotationItem) GrossProfit() float64 { return i.AmountBase - i.CostAmount }
