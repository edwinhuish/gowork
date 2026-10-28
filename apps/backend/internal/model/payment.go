package model

import (
	"fmt"
	"time"
)

// PaymentDirection 是这笔钱的方向：收进来还是付出去。
//
// 收付款共用一张表：两者字段完全一致（对方、金额、汇率、水单号），
// 方向只决定"能核销应收还是应付"。拆两张表的代价是同一套核销逻辑写两遍。
type PaymentDirection int8

const (
	// PaymentDirectionIn 收款：钱从客户进来，核销应收。
	PaymentDirectionIn PaymentDirection = 10
	// PaymentDirectionOut 付款：钱付给供应商，核销应付。
	PaymentDirectionOut PaymentDirection = 20
)

func (d PaymentDirection) IsValid() bool {
	switch d {
	case PaymentDirectionIn, PaymentDirectionOut:
		return true
	}
	return false
}

func (d PaymentDirection) Label() string {
	switch d {
	case PaymentDirectionIn:
		return "收款"
	case PaymentDirectionOut:
		return "付款"
	default:
		return fmt.Sprintf("未知方向(%d)", int8(d))
	}
}

// SettlementTarget 是核销对象：应收单还是应付单。
//
// 不直接用两个可空外键（receivable_id / payable_id）：那样"恰好填一个"
// 在数据库里表达不了，只能靠 service 每次记得判空；一个类型列 + 一个 id
// 至少能把"类型和 id 必须同时给"这一条写进校验。
type SettlementTarget int8

const (
	// SettlementTargetReceivable 核销应收单。
	SettlementTargetReceivable SettlementTarget = 10
	// SettlementTargetPayable 核销应付单。
	SettlementTargetPayable SettlementTarget = 20
)

func (t SettlementTarget) IsValid() bool {
	switch t {
	case SettlementTargetReceivable, SettlementTargetPayable:
		return true
	}
	return false
}

func (t SettlementTarget) Label() string {
	switch t {
	case SettlementTargetReceivable:
		return "应收单"
	case SettlementTargetPayable:
		return "应付单"
	default:
		return fmt.Sprintf("未知核销对象(%d)", int8(t))
	}
}

// Payment 是实际发生的一笔收款或付款。
//
// 与应收应付的分工：应收应付是"应该收/付多少"（开票时锁定），
// 收付款是"实际收/付了多少"（到账时定格）。两者之间靠核销行连接，
// 差额与时间差才是账龄和汇兑损益的来源。
//
// 金额按「原币 + 币种 + 本币 + 汇率」四件套存，但**汇率取到账那天**，
// 不是应收开单时锁的那个：同一个 1000 美元，开票时 7.1、到账时 7.3，
// 差额 200 元就是汇兑损益（T-23）。把收款汇率也造成 7.1，这笔损益就永远看不见。
type Payment struct {
	Base
	DocNo     string           `gorm:"size:32;uniqueIndex;not null" json:"doc_no"`
	Direction PaymentDirection `gorm:"not null;index" json:"direction"`
	// PartnerID 是收付款的对方：收款指向客户，付款指向供应商。
	PartnerID uint `gorm:"index;not null" json:"partner_id"`
	// PayDate 是实际到账 / 付出日（业务日期），不是录入日。
	// 账龄与汇兑损益都按它算，所以它是"钱动的那天"而不是"单开的那天"。
	PayDate time.Time `gorm:"type:date;not null;index" json:"pay_date"`

	CurrencyCode string  `gorm:"size:8;not null" json:"currency_code"`
	ExchangeRate float64 `gorm:"type:decimal(18,8);not null;default:1" json:"exchange_rate"`
	Amount       float64 `gorm:"type:decimal(16,4);not null;default:0" json:"amount"`
	AmountBase   float64 `gorm:"type:decimal(16,4);not null;default:0" json:"amount_base"`

	// Method 是结算方式（TT / LC …），与主体的默认结算方式同源。
	Method SettlementMethod `gorm:"size:8" json:"method"`
	// Reference 是银行水单号或流水号，**外部号**：只记录、不参与发号与唯一性。
	Reference string `gorm:"size:64" json:"reference"`
	Remark    string `gorm:"size:255" json:"remark"`

	Partner     *Partner            `gorm:"foreignKey:PartnerID" json:"partner,omitempty"`
	Allocations []PaymentAllocation `gorm:"foreignKey:PaymentID" json:"allocations,omitempty"`
}

func (Payment) TableName() string { return "biz_payment" }

// AllocatedAmount 是这笔收付款已核销出去的原币金额。
// 小于 Amount 的部分是预收 / 预付（还没对应到任何单据）。
func (p Payment) AllocatedAmount() float64 {
	var sum float64
	for _, a := range p.Allocations {
		sum += a.Amount
	}
	return sum
}

// PaymentAllocation 是收付款单与应收 / 应付之间的分配行。
//
// **核销不是冲销。** 核销回答"这笔钱还的是哪张单"，冲销是财务的红字冲回。
// 所以这里只增不改：录错了就删掉整张收付款单重录，单据上的已核销金额
// 跟着回滚——而不是在这一行上改成负数。
type PaymentAllocation struct {
	Base
	PaymentID  uint             `gorm:"index;not null" json:"payment_id"`
	TargetType SettlementTarget `gorm:"not null;index" json:"target_type"`
	TargetID   uint             `gorm:"not null;index" json:"target_id"`
	// TargetDocNo 是目标单号，摊平存一份：核销清单要显示"还的是哪张单"，
	// 每行都去 join 应收或应付表不划算，而单号本身不会变。
	TargetDocNo string `gorm:"size:32" json:"target_doc_no"`

	// Amount 是核销的原币金额，币种跟随收付款单。
	// **必须与目标单据的币种一致**：跨币种核销要先约定换算口径，
	// 而那种约定一出口岸就各家不同，先不做（见 service 的校验）。
	Amount float64 `gorm:"type:decimal(16,4);not null;default:0" json:"amount"`
	// AmountBase 是本币金额，按**收付款单的汇率**折。
	// 它与目标单据上锁定的本币金额之差，就是这一笔的汇兑损益原料。
	AmountBase float64 `gorm:"type:decimal(16,4);not null;default:0" json:"amount_base"`
}

func (PaymentAllocation) TableName() string { return "biz_payment_allocation" }

// SettlementStatusFor 由金额推导结清状态。
//
// 状态是金额的派生值，不单独维护：允许两者各改各的，就会出现
// "状态写着已结清、已收金额却差 500"的单据，而这种矛盾没人负责解释。
func SettlementStatusFor(amount, settled float64) SettlementStatus {
	switch {
	case settled <= 0:
		return SettlementStatusOpen
	// 金额按 4 位小数存，允许 0.0001 的浮点余量：分批收款累加到最后一笔时，
	// 差一个末位就把单据卡在"部分结清"上，比数字不准更让人困惑。
	case settled+0.0001 >= amount:
		return SettlementStatusSettled
	default:
		return SettlementStatusPartial
	}
}
