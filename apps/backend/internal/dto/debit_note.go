package dto

import "github.com/edwinhuish/gowork/internal/pkg/money"

// DebitNoteInput 是开一张借项通知单的入参。
//
// 与贷项一样没有币种与汇率：它们随原应付单，不让调用方选。
type DebitNoteInput struct {
	PayableID  uint          `json:"payable_id" binding:"required"`
	Amount     money.Decimal `json:"amount"`
	IssuedDate string        `json:"issued_date"`
	Reason     string        `json:"reason" binding:"max=255"`
	Remark     string        `json:"remark" binding:"max=255"`
}

// DebitNoteQuery 是借项列表的筛选条件。
type DebitNoteQuery struct {
	PageQuery
	PayableID *uint `form:"payable_id"`
	PartnerID *uint `form:"partner_id"`
}
