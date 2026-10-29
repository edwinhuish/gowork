package dto

import (
	"time"

	"github.com/edwinhuish/gowork/internal/pkg/money"
)

// PaymentQuery 是收付款单的查询条件。
type PaymentQuery struct {
	PageQuery
	Direction *int8 `form:"direction"`
	PartnerID *uint `form:"partner_id"`
}

// PaymentAllocationInput 是一行核销：还的是哪张单、还多少。
type PaymentAllocationInput struct {
	// TargetType 10=应收单 20=应付单，必须与收付款方向匹配（收款只能核应收）。
	TargetType int8          `json:"target_type" binding:"required"`
	TargetID   uint          `json:"target_id" binding:"required"`
	Amount     money.Decimal `json:"amount"`
}

// PaymentInput 新建收付款单。
//
// 没有 exchange_rate：收款汇率取**到账日**的汇率，不是应收开单时锁的那个。
// 同一个 1000 美元开票 7.1、到账 7.3，差额才是汇兑损益；把收款汇率也填成 7.1，
// 这笔损益就永远看不见。
//
// partner_id / currency_code / amount 都可以不传：分别从核销目标带出、
// 从核销目标带出、取核销合计。只有"预收款"（暂不核销）时才必须显式给。
type PaymentInput struct {
	Direction    int8          `json:"direction" binding:"required"`
	PartnerID    uint          `json:"partner_id"`
	PayDate      string        `json:"pay_date"`
	CurrencyCode string        `json:"currency_code" binding:"max=8"`
	Amount       money.Decimal `json:"amount"`
	Method       string        `json:"method" binding:"max=8"`
	Reference    string        `json:"reference" binding:"max=64"`
	Remark       string        `json:"remark" binding:"max=255"`

	Allocations []PaymentAllocationInput `json:"allocations"`
}

// PaymentUpdateInput 只允许改不涉及金额的字段。
//
// 金额与核销明细不给改：改一行核销要先回滚它、再按新值重算目标单据的状态，
// 而"录错一笔"的真实操作是删掉重录——与汇率录错删掉重录同一条约定。
type PaymentUpdateInput struct {
	PayDate   string `json:"pay_date"`
	Method    string `json:"method" binding:"max=8"`
	Reference string `json:"reference" binding:"max=64"`
	Remark    string `json:"remark" binding:"max=255"`
	// UpdatedAt 是编辑页打开时读到的版本；带上它做条件更新，期间被别人改过则 409。
	UpdatedAt *time.Time `json:"updated_at"`
}
