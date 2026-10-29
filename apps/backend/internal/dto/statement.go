package dto

import "github.com/edwinhuish/gowork/internal/pkg/money"

// StatementQuery 是客户对账单的入参。
//
// partner_id 必填：对账单是「跟某一家对」，没有全量对账单这种东西。
// 币种不传则按该客户出现过的第一个币种；跨币种客户要按币种分别对，
// 因为把美元和人民币加在一起得到的余额没有意义。
type StatementQuery struct {
	PartnerID uint   `form:"partner_id" binding:"required"`
	Currency  string `form:"currency_code"`
	// From / To 是账期区间，YYYY-MM-DD，左闭右闭。留空表示不限。
	From string `form:"from"`
	To   string `form:"to"`
}

// StatementLine 是对账单上的一笔发生额。
type StatementLine struct {
	// Kind 是这句话的来源：receivable 应收 / credit_note 贷项 / payment 收款。
	// 不合并成"发生额 + 方向"：看对账单的人想知道这是开票、退货还是收了钱。
	Kind    string        `json:"kind"`
	DocNo   string        `json:"doc_no"`
	DocDate string        `json:"doc_date"`
	Amount  money.Decimal `json:"amount"`
	// Signed 是带符号的金额：应收为正、贷项与收款为负，期末余额就是它的累加。
	Signed money.Decimal `json:"signed_amount"`
	Remark string        `json:"remark"`
}

// StatementResult 是客户对账单。
//
// 期初 + 本期发生 − 本期收款 = 期末，三项都给出，让人能自己验算——
// 只给期末余额的对账单，客户第一句话一定是"这个数怎么来的"。
type StatementResult struct {
	PartnerID   uint            `json:"partner_id"`
	PartnerName string          `json:"partner_name"`
	Currency    string          `json:"currency_code"`
	From        string          `json:"from"`
	To          string          `json:"to"`
	Opening     money.Decimal   `json:"opening"`
	Debit       money.Decimal   `json:"period_debit"`
	Credit      money.Decimal   `json:"period_credit"`
	Closing     money.Decimal   `json:"closing"`
	Lines       []StatementLine `json:"lines"`
}
