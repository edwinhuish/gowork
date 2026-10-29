package dto

import "github.com/edwinhuish/gowork/internal/pkg/money"

// CreditNoteInput 是开一张贷项通知单的入参。
//
// 没有币种与汇率：它们随原应收单，不让调用方选——跨币种冲销要先有分摊口径，
// 而"按哪个汇率冲"这个问题现在还没有答案。
type CreditNoteInput struct {
	ReceivableID uint          `json:"receivable_id" binding:"required"`
	Amount       money.Decimal `json:"amount"`
	IssuedDate   string        `json:"issued_date"`
	Reason       string        `json:"reason" binding:"max=255"`
	Remark       string        `json:"remark" binding:"max=255"`
}

// CreditNoteQuery 是贷项列表的筛选条件。
type CreditNoteQuery struct {
	PageQuery
	ReceivableID *uint `form:"receivable_id"`
	PartnerID    *uint `form:"partner_id"`
}
