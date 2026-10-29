package model

import "github.com/edwinhuish/gowork/internal/pkg/money"

import (
	"fmt"
	"time"
)

// ContractType 区分形式发票与销售合同。
//
// 同一张表：两者的字段与生命周期一致（客户、金额、币种、付款条款、确认后不可改），
// 差别只在客户怎么称呼它、能不能凭它开信用证。拆两张表意味着分期校验、
// 状态机与单号生成写两遍，而它们本来就该完全一样。
type ContractType int8

const (
	// ContractTypePI 形式发票：客户凭它付定金或开信用证，常先于订单成立。
	ContractTypePI ContractType = 10
	// ContractTypeSales 销售合同：双方签字的正式合同。
	ContractTypeSales ContractType = 20
)

func (t ContractType) IsValid() bool {
	return t == ContractTypePI || t == ContractTypeSales
}

func (t ContractType) Label() string {
	switch t {
	case ContractTypePI:
		return "形式发票"
	case ContractTypeSales:
		return "销售合同"
	default:
		return fmt.Sprintf("未知类型(%d)", int8(t))
	}
}

// ContractSourceType 是这份合同的来源单据。
//
// PI 常先于订单成立，所以来源允许是报价单；已经有订单时也可以从订单生成，
// 此时金额与明细以订单为准——合同不复制明细，只存快照。
type ContractSourceType int8

const (
	// ContractSourceQuotation 来源报价单。
	ContractSourceQuotation ContractSourceType = 10
	// ContractSourceSalesOrder 来源销售订单。
	ContractSourceSalesOrder ContractSourceType = 20
)

func (t ContractSourceType) IsValid() bool {
	return t == ContractSourceQuotation || t == ContractSourceSalesOrder
}

func (t ContractSourceType) Label() string {
	switch t {
	case ContractSourceQuotation:
		return "报价单"
	case ContractSourceSalesOrder:
		return "销售订单"
	default:
		return fmt.Sprintf("未知来源(%d)", int8(t))
	}
}

// Contract 是形式发票或销售合同。
//
// **不存货物明细**：明细留在报价单或订单上，这里只存一份对外承诺的快照
// （客户、币种、汇率、总额、付款条款）。复制明细就是两个真相——
// 订单改价之后，合同上的旧价要么跟着变（那合同就不再是签过的那份），
// 要么不变（那两份单据对不上，出货时按哪个算没人说得清）。
//
// 金额与汇率在生成时**照抄来源单据**，不重取当日汇率：转单那一刻改价
// 等于单方面修改已经报出去的价，与报价转订单是同一条规矩。
type Contract struct {
	Base
	// 遮蔽 Base 里的同名字段，把它排进「未删才唯一」的复合索引，见 Product 的说明。
	DeletedAt DeletedAt `gorm:"softDelete:milli;not null;default:0;uniqueIndex:uni_contract_doc_no,priority:2" json:"deleted_at,omitempty"`
	// DocNo 单号：PI 用 PI- 前缀、销售合同用 SC- 前缀，各自一条序列。
	// 单号本身能看出这是哪一种，不必再翻 type 列。
	DocNo string       `gorm:"size:32;uniqueIndex:uni_contract_doc_no,priority:1;not null" json:"doc_no"`
	Type  ContractType `gorm:"not null;index" json:"type"`

	PartnerID uint `gorm:"index;not null" json:"partner_id"`

	// 来源单据：id + 单号快照。单号摊平存一份，列表与详情不必每次都回去 join。
	SourceType  ContractSourceType `gorm:"not null;index" json:"source_type"`
	SourceID    uint               `gorm:"index;not null" json:"source_id"`
	SourceDocNo string             `gorm:"size:32" json:"source_doc_no"`

	Status DocumentStatus `gorm:"not null;index" json:"status"`
	// SignDate 签订 / 出具日（业务日期），不是录入日；单号年月按它取。
	SignDate time.Time `gorm:"type:date;not null;index" json:"sign_date"`
	// DeliveryDate 约定交期，可空。
	DeliveryDate *time.Time `gorm:"type:date" json:"delivery_date"`

	// 四件套：币种、汇率、原币总额、本币总额，全部来自来源单据的锁定值。
	CurrencyCode    string        `gorm:"size:8;not null" json:"currency_code"`
	ExchangeRate    money.Decimal `gorm:"type:decimal(18,8);not null;default:1" json:"exchange_rate"`
	AmountTotal     money.Decimal `gorm:"type:decimal(16,4);not null;default:0" json:"amount_total"`
	AmountBaseTotal money.Decimal `gorm:"type:decimal(16,4);not null;default:0" json:"amount_base_total"`

	Remark string `gorm:"size:255" json:"remark"`

	Partner *Partner       `gorm:"foreignKey:PartnerID" json:"partner,omitempty"`
	Terms   []ContractTerm `gorm:"foreignKey:ContractID" json:"terms,omitempty"`
	// Clauses 是条款**快照**（见 model.ContractClause）：改模板不影响已生成的合同。
	Clauses []ContractClause `gorm:"foreignKey:ContractID" json:"clauses,omitempty"`
}

func (Contract) TableName() string { return "biz_contract" }

// IsEditable 报告这份合同还能不能改。只有草稿能改：确认之后它是对客户的
// 承诺，改动要留痕（T-44 的变更单），不是把原单悄悄改掉。
func (c Contract) IsEditable() bool { return c.Status == DocumentStatusDraft }

// ContractTerm 是合同的一条付款分期：30% 订金 / 70% 见提单副本。
//
// 金额用原币存（合同币种），本币按合同汇率折算随行存一份：
// 分期报表要按本币汇总，每次都乘一遍既慢又容易和合同总额差一个末位。
type ContractTerm struct {
	Base
	ContractID uint `gorm:"index;not null" json:"contract_id"`
	// Seq 是展示顺序，从 1 开始。付款有先后，顺序不是装饰。
	Seq  int    `gorm:"not null;default:1" json:"seq"`
	Name string `gorm:"size:64;not null" json:"name"`

	Amount     money.Decimal `gorm:"type:decimal(16,4);not null;default:0" json:"amount"`
	AmountBase money.Decimal `gorm:"type:decimal(16,4);not null;default:0" json:"amount_base"`

	// DueDate 是约定付款日；Trigger 是"见提单副本后 7 天内"这类条件。
	// 两者可同时存在：有具体日期就填，没有就把条件写清楚。
	DueDate *time.Time `gorm:"type:date" json:"due_date"`
	Trigger string     `gorm:"size:128" json:"trigger"`
}

func (ContractTerm) TableName() string { return "biz_contract_term" }
