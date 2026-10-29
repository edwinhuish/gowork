package model

import (
	"time"

	"github.com/edwinhuish/gowork/internal/pkg/money"
)

// DebitNote 是借项通知单（T-52b3 的最后一段）。
//
// 与贷项通知单对称：贷项冲的是"客户不用付的那部分"，借项冲的是
// "我们不用付给供应商的那部分"——退货、短装、来料不合格的让价。
//
// 没有它，采购退货会减库存却减不了应付：账上永远显示还欠供应商这批货的钱，
// 而仓库里货已经退回去了。
type DebitNote struct {
	Base
	// 遮蔽 Base 里的同名字段，把它排进「未删才唯一」的复合索引，见 Product 的说明。
	DeletedAt DeletedAt `gorm:"softDelete:milli;not null;default:0;uniqueIndex:uni_debit_note_doc_no,priority:2" json:"deleted_at,omitempty"`
	DocNo     string    `gorm:"size:32;uniqueIndex:uni_debit_note_doc_no,priority:1;not null" json:"doc_no"`

	// PayableID 指向被冲减的那张应付单：借项永远依附于一张原单。
	PayableID uint `gorm:"index;not null" json:"payable_id"`
	// Amount 是冲减金额，正数。方向由表的语义决定，不靠符号。
	Amount money.Decimal `gorm:"type:decimal(18,4);not null" json:"amount"`
	// 币种与汇率随原应付单，不单独选：跨币种冲销要有明确的分摊口径。
	CurrencyCode string        `gorm:"size:8;not null" json:"currency_code"`
	ExchangeRate money.Decimal `gorm:"type:decimal(18,8);not null;default:1" json:"exchange_rate"`
	AmountBase   money.Decimal `gorm:"type:decimal(18,4);not null" json:"amount_base"`

	IssuedDate time.Time `gorm:"type:date;not null;index" json:"issued_date"`
	Reason     string    `gorm:"size:255" json:"reason"`
	Remark     string    `gorm:"size:255" json:"remark"`

	// 供列表显示的投影，不落库。
	PayableDocNo string `gorm:"-" json:"payable_doc_no,omitempty"`
	PartnerName  string `gorm:"-" json:"partner_name,omitempty"`
}

func (DebitNote) TableName() string { return "biz_debit_note" }
