// Package code 归一化业务编号，供「按编号检索」使用。
//
// 同一个货号在客户邮件、供应商单据、工厂标签上写法各不相同：`ABC-123`、`abc 123`、
// `ABC123` 说的是同一件货。检索要一条关键词全部命中，所以入库时额外存一份归一化结果，
// 查询时把关键词也走同一个函数——两边口径不同，就会出现「明明有却搜不到」。
//
// 只做归一化，不做校验：编号的格式由业务方决定，这里不假设它是纯数字或带前缀。
package code

import "strings"

// separators 是编号里"可有可无"的字符。
//
// 只去掉分隔符，不把非字母数字一律删掉：那样 `A/B` 与 `AB` 会混成一个编号，
// 而它们在客户那边很可能是两个东西（一个带斜杠的是规格区分号）。
const separators = " \t\u3000-_./\\"

// Normalize 转大写并去掉分隔符。
//
// 不做 Unicode 折叠（全角字母转半角）：外贸编号里的全角字符基本是从中文输入法
// 误带出来的，同一个词两份写法并存，正确做法是录的时候删掉，而不是在检索里猜。
func Normalize(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}

	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if strings.ContainsRune(separators, r) {
			continue
		}
		b.WriteRune(r)
	}
	return strings.ToUpper(b.String())
}
