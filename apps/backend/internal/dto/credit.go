package dto

// CreditSummary 是一个主体当前的信用占用情况，单位本币。
//
// 它只回答"还能不能再接单"，不构成对账依据：明细在应收单、订单与出货单上，
// 这里任何一个数字都能由那几张表重新算出来。
type CreditSummary struct {
	PartnerID uint `json:"partner_id"`
	// CreditLimit 是额度，0 表示不限（不是"没有额度"）。
	CreditLimit float64 `json:"credit_limit"`
	Unlimited   bool    `json:"unlimited"`

	// OpenReceivable 是未结清应收的本币余额。
	OpenReceivable float64 `json:"open_receivable"`
	// UnbilledOrder 是已确认及以上、还没开应收的订单本币金额。
	UnbilledOrder float64 `json:"unbilled_order"`
	// Exposure = OpenReceivable + UnbilledOrder。
	Exposure float64 `json:"exposure"`

	// Available 是额度减敞口；超了就是负数（说明已经压了多少超额），
	// 不限额度时为 0，看 Unlimited 判断。
	Available float64 `json:"available"`
}
