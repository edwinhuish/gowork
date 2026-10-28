package model

import (
	"fmt"
	"time"

	"github.com/edwinhuish/gowork/internal/pkg/money"
)

// OpportunityStage 是商机阶段。
//
// 与 DocumentStatus 是两套东西，刻意不复用：
// 单据状态只会前进（已出货的单不能退回草稿），而销售阶段**会退回去**——
// 客户预算冻结、采购换人、汇率暴涨，谈着谈着回到"需求确认"是常态。
// 禁止回退的系统会被用户用"新建一个商机"绕过，历史就断了。
type OpportunityStage int8

const (
	OpportunityStageQualify     OpportunityStage = 10 // 初步接洽
	OpportunityStageNeed        OpportunityStage = 20 // 需求确认
	OpportunityStageQuoted      OpportunityStage = 30 // 已报价
	OpportunityStageNegotiating OpportunityStage = 40 // 谈判中
	OpportunityStageWon         OpportunityStage = 50 // 赢单
	OpportunityStageLost        OpportunityStage = 60 // 输单
)

func (s OpportunityStage) IsValid() bool {
	switch s {
	case OpportunityStageQualify, OpportunityStageNeed, OpportunityStageQuoted,
		OpportunityStageNegotiating, OpportunityStageWon, OpportunityStageLost:
		return true
	}
	return false
}

// IsFinal 报告是否为终态。赢单与输单都不再流转——
// 客户明年回头是新的一轮生意，应新建商机，而不是把去年那条改回"谈判中"。
func (s OpportunityStage) IsFinal() bool {
	return s == OpportunityStageWon || s == OpportunityStageLost
}

// CanTransitTo 允许非终态之间任意跳转（含回退），终态不可再改。
//
// 不限制"只能前进一格"：销售不是流水线，跳两格（接洽直接到已报价）很常见，
// 强制逐格只会让用户乱填中间阶段。
func (s OpportunityStage) CanTransitTo(next OpportunityStage) bool {
	if !next.IsValid() || next == s {
		return false
	}
	if s.IsFinal() {
		return false
	}
	return true
}

// DefaultProbability 是各阶段的默认赢单率。
//
// 只是一个默认值，落库时仍按"概率分列存"（见 docs/trade-erp-crm-blueprint.md B3）：
// 同一个"已报价"，老客户 70%、新客户 30%，按阶段反推概率会把这两种情况算成一个数。
func (s OpportunityStage) DefaultProbability() int {
	switch s {
	case OpportunityStageQualify:
		return 10
	case OpportunityStageNeed:
		return 30
	case OpportunityStageQuoted:
		return 50
	case OpportunityStageNegotiating:
		return 70
	case OpportunityStageWon:
		return 100
	case OpportunityStageLost:
		return 0
	default:
		return 0
	}
}

func (s OpportunityStage) Label() string {
	switch s {
	case OpportunityStageQualify:
		return "初步接洽"
	case OpportunityStageNeed:
		return "需求确认"
	case OpportunityStageQuoted:
		return "已报价"
	case OpportunityStageNegotiating:
		return "谈判中"
	case OpportunityStageWon:
		return "赢单"
	case OpportunityStageLost:
		return "输单"
	default:
		return fmt.Sprintf("未知阶段(%d)", int8(s))
	}
}

// Opportunity 是销售机会。
//
// 金额按「币种 + 原币 + 本币 + 汇率」四件套存（蓝图第五节第 1 条）：
// 只存一个数，等到要做加权预测时就没法把 USD 与 CNY 的商机加在一起。
// 与正式单据的区别：商机的金额是**预计值**，重算汇率不会影响任何已发生的交易，
// 所以金额变更时按当日汇率重算；报价单与订单在那边锁定汇率，两者规则不同。
type Opportunity struct {
	Base
	PartnerID uint             `gorm:"index;not null" json:"partner_id"`
	Subject   string           `gorm:"size:128;not null" json:"subject"`
	Stage     OpportunityStage `gorm:"not null;index" json:"stage"`

	// 三个金额字段用 money.Decimal（定点）：加权预测是把几十条商机相加之后
	// 与已成交金额对比，float64 的末位误差会在看板上变成"权重加起来比总额多几分钱"。
	Amount       money.Decimal `gorm:"type:decimal(14,2);not null;default:0" json:"amount"`
	CurrencyCode string        `gorm:"size:8;not null" json:"currency_code"`
	AmountBase   money.Decimal `gorm:"type:decimal(14,2);not null;default:0" json:"amount_base"`
	ExchangeRate money.Decimal `gorm:"type:decimal(18,8);not null;default:1" json:"exchange_rate"`

	// Probability 赢单概率 0-100。赢单恒为 100、输单恒为 0，不允许手填成别的值。
	Probability int `gorm:"not null;default:0" json:"probability"`
	// ExpectedCloseDate 预计成交日，按日（见 model.DateOnly）。
	ExpectedCloseDate time.Time `gorm:"type:date;index;not null" json:"expected_close_date"`
	Remark            string    `gorm:"size:255" json:"remark"`

	Partner *Partner `gorm:"foreignKey:PartnerID" json:"partner,omitempty"`
}

func (Opportunity) TableName() string { return "crm_opportunity" }

// WeightedAmount 是加权金额（本币）：预计金额 × 赢单概率。
func (o Opportunity) WeightedAmount() money.Decimal {
	return o.AmountBase.MulInt(int64(o.Probability)).Div(money.FromInt(100))
}
