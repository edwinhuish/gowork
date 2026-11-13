package model

// TradeTerm 贸易术语（Incoterms 里最常见的五种）。
//
// 它不是标签而是报价的一部分：FOB 的海运费由买方承担，CIF 由卖方承担并含保险。
// 同一批货报 FOB 与报 CIF 的价差可以有 10%，所以报价单与订单都必须带这个字段，
// 否则两个报价放在一起比价是错的。
type TradeTerm string

const (
	TradeTermEXW TradeTerm = "EXW" // 工厂交货
	TradeTermFOB TradeTerm = "FOB" // 装运港船上交货
	TradeTermCFR TradeTerm = "CFR" // 成本加运费
	TradeTermCIF TradeTerm = "CIF" // 成本、保险加运费
	TradeTermDDP TradeTerm = "DDP" // 完税后交货
)

// IsValid 拒绝未定义的术语。空串表示"未指定"，由调用方单独判断。
func (t TradeTerm) IsValid() bool {
	switch t {
	case TradeTermEXW, TradeTermFOB, TradeTermCFR, TradeTermCIF, TradeTermDDP:
		return true
	}
	return false
}

// SettlementMethod 结算方式。
//
// 决定账期与风险，也决定报价的价格基础：T/T 订金到账才排产，
// O/A 赊销要自己垫资，两者的价格不该一样。
type SettlementMethod string

const (
	SettlementTT SettlementMethod = "TT" // 电汇（常见订金 + 尾款）
	SettlementLC SettlementMethod = "LC" // 信用证
	SettlementDP SettlementMethod = "DP" // 付款交单
	SettlementDA SettlementMethod = "DA" // 承兑交单
	SettlementOA SettlementMethod = "OA" // 赊销
)

// IsValid 拒绝未定义的结算方式。空串表示"未指定"。
func (m SettlementMethod) IsValid() bool {
	switch m {
	case SettlementTT, SettlementLC, SettlementDP, SettlementDA, SettlementOA:
		return true
	}
	return false
}
