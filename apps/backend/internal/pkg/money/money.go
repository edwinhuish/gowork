// Package money 收敛金额与汇率的数值处理规则。
//
// 单独成包而不是散在 service 里，是因为"保留几位、怎么舍入"必须全局只有一套答案：
// 报价单算出的毛利、订单汇总的总额、应收单的本币金额如果各自 math.Round，
// 同一笔生意在三张单上会差几分钱，对账时无法解释。
package money

import "math"

// 允许的小数位范围。上限不给到 8 位以上：本项目金额用 float64 承载
// （见 docs/architecture.md 第六节），超过 6 位后二进制误差已经大于末位本身。
const (
	MinDecimalPlaces = 0
	MaxDecimalPlaces = 6
)

// Round 按小数位四舍五入。
//
// 只做"把末位之后的数舍掉"这一件事，不做币种换算、不做进位到整数——
// 那些属于业务规则（比如某些市场习惯向上取整到 0.05），应在调用处显式写明。
func Round(v float64, decimalPlaces int) float64 {
	if decimalPlaces < MinDecimalPlaces {
		decimalPlaces = MinDecimalPlaces
	}
	if decimalPlaces > MaxDecimalPlaces {
		decimalPlaces = MaxDecimalPlaces
	}
	p := math.Pow(10, float64(decimalPlaces))
	return math.Round(v*p) / p
}
