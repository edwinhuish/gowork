// Package csvx 输出"能直接双击打开"的 CSV。
//
// 除了 BOM，这里还负责**公式注入防护**：单元格以 `=` / `+` / `-` / `@` 开头时，
// Excel 会把它当公式执行。导出文件会被转发出去（"把客户名单发给同事一份"），
// 而名单里的公司名、联系地址都是人工录进来的——一个叫 `=cmd|'/C calc'!A0` 的客户，
// 足以让打开文件的人中招。
package csvx

import (
	"encoding/csv"
	"io"
)

// bom 是 UTF-8 的字节序标记。
//
// 必须写：Excel 打开不带 BOM 的 UTF-8 CSV 会按本地代码页解码，
// 中文全变成乱码，而拿到文件的人第一反应是"你们系统导出的东西是坏的"。
var bom = []byte{0xEF, 0xBB, 0xBF}

// Write 把表头与数据行写成带 BOM 的 CSV。
//
// 用 CSV 而不是 xlsx：不引第三方依赖，Excel / Numbers / WPS 都能直接打开，
// 而"把客户名单发给同事一份"这个需求等不起一个新依赖。
func Write(w io.Writer, header []string, rows [][]string) error {
	if _, err := w.Write(bom); err != nil {
		return err
	}

	writer := csv.NewWriter(w)
	if err := writer.Write(sanitizeRow(header)); err != nil {
		return err
	}
	for _, row := range rows {
		if err := writer.Write(sanitizeRow(row)); err != nil {
			return err
		}
	}
	writer.Flush()
	return writer.Error()
}

// sanitizeRow 逐格转义，返回新切片——不就地改调用方的数据。
func sanitizeRow(row []string) []string {
	out := make([]string, len(row))
	for i, cell := range row {
		out[i] = sanitizeCell(cell)
	}
	return out
}

// sanitizeCell 给可能被当成公式的单元格加前导单引号。
//
// 加 `'` 而不是包引号：CSV 的引号只是字段分隔手段，Excel 读进来仍会按公式求值。
// `'` 是 Excel 自己的"这一格是文本"标记，导出后显示的是原文，不会带上那个引号。
func sanitizeCell(s string) string {
	if !couldBeFormula(s) {
		return s
	}
	return "'" + s
}

// couldBeFormula 判断这一格会不会被 Excel 当公式。
//
// `-` 单独判：负数（-12.34、-1e-3）是正常的金额与数量，
// 加了前缀就变成文本，用户在 Excel 里连求和都做不了。
func couldBeFormula(s string) bool {
	if s == "" {
		return false
	}
	switch s[0] {
	case '=', '+', '@', '\t', '\r':
		return true
	case '-':
		return !isNumericTail(s[1:])
	default:
		return false
	}
}

// isNumericTail 报告去掉负号后的剩余部分是否是纯数字（允许小数与科学计数）。
func isNumericTail(s string) bool {
	if s == "" {
		return false
	}
	hasDigit := false
	// 只允许一个指数符号，且后面必须是数字：-1e-3 合法，-1e-x 不合法。
	expSeen := false
	for i, r := range s {
		switch {
		case r >= '0' && r <= '9':
			hasDigit = true
		case r == '.':
		case r == 'e' || r == 'E':
			if expSeen || !hasDigit {
				return false
			}
			expSeen = true
		case (r == '+' || r == '-') && expSeen && i > 0 && (s[i-1] == 'e' || s[i-1] == 'E'):
		default:
			return false
		}
	}
	return hasDigit
}
