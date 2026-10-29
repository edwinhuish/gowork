package model

import (
	"time"

	"github.com/edwinhuish/gowork/internal/pkg/money"
)

// CreditNote 是贷项通知单（T-52b）。
//
// 它记录的是「这张发票上的这一部分，客户不用付了」：退货、折让、短装。
// 在此之前系统里表达不了这件事——只能把原应收作废重开，而重开意味着
// 单号变了，客户手上那份对不上。
//
// **不建负向应收单**：负金额会把核销校验、账龄分桶、回款率三处的前提
// （金额为正）一起打破。贷项单独一张表，扣减发生在"未结清额"这一层：
// remaining = amount − settled − credited。
type CreditNote struct {
	Base
	// 遮蔽 Base 里的同名字段，把它排进「未删才唯一」的复合索引，见 Product 的说明。
	DeletedAt DeletedAt `gorm:"softDelete:milli;not null;default:0;uniqueIndex:uni_credit_note_doc_no,priority:2" json:"deleted_at,omitempty"`
	DocNo     string    `gorm:"size:32;uniqueIndex:uni_credit_note_doc_no,priority:1;not null" json:"doc_no"`

	// ReceivableID 指向被冲减的那张应收单：贷项永远依附于一张原单，
	// 没有"独立存在的贷项"——那在账务上叫预收，不叫冲销。
	ReceivableID uint `gorm:"index;not null" json:"receivable_id"`
	// Amount 是冲减金额，**正数**。方向由这张表的语义决定，
	// 不靠符号：负金额会让所有"金额必须大于 0"的校验各自判断一遍。
	Amount money.Decimal `gorm:"type:decimal(18,4);not null" json:"amount"`
	// 币种与汇率随原应收单，不单独选：跨币种冲销要有明确的分摊口径，
	// 而现在连"按哪个汇率"都还没有答案。
	CurrencyCode string        `gorm:"size:8;not null" json:"currency_code"`
	ExchangeRate money.Decimal `gorm:"type:decimal(18,8);not null;default:1" json:"exchange_rate"`
	AmountBase   money.Decimal `gorm:"type:decimal(18,4);not null" json:"amount_base"`

	// IssuedDate 是贷项开出的日期，决定它落在哪个统计月份。
	IssuedDate time.Time `gorm:"type:date;not null;index" json:"issued_date"`
	// Reason 是退货 / 折让 / 短装等，自由文本：这一类原因远比枚举多，
	// 做成下拉只会逼人全选「其它」。
	Reason string `gorm:"size:255" json:"reason"`
	Remark string `gorm:"size:255" json:"remark"`

	// 以下三项是给列表用的投影，不落库。
	ReceivableDocNo string `gorm:"-" json:"receivable_doc_no,omitempty"`
	PartnerID       uint   `gorm:"-" json:"partner_id,omitempty"`
	PartnerName     string `gorm:"-" json:"partner_name,omitempty"`
}

func (CreditNote) TableName() string { return "biz_credit_note" }
