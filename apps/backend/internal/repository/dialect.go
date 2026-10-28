package repository

import (
	"strings"

	"gorm.io/gorm"

	"github.com/edwinhuish/gowork/internal/pkg/dialect"
)

// likeEscape 是 LIKE 的转义字符。
//
// 用 '!' 而不是反斜杠：postgres 的标准字符串里 '\' 就是反斜杠，
// 而 mysql 的字符串里要写 '\\'，两者不同；换成 '!' 三个库的写法完全一样。
const likeEscape = '!'

// likeAny 生成「任一列模糊匹配」的条件，三个库行为保持一致：
//
//   - 大小写：mysql 默认排序规则与 sqlite 的 LIKE 本就不敏感，
//     postgres 的 LIKE 区分大小写，因此改用 ILIKE；
//   - 通配符：用户输入里的 % _ ! 一律转义，避免搜 "M8" 把 "M8x" 以外的行也带出来；
//   - 转义符：统一 ESCAPE '!'，不依赖各驱动对反斜杠的不同解释。
func likeAny(keyword string, columns ...string) Scope {
	pattern := "%" + escapeLike(keyword) + "%"

	return func(db *gorm.DB) *gorm.DB {
		op := "LIKE"
		if db.Dialector.Name() == dialect.Postgres {
			op = "ILIKE"
		}

		parts := make([]string, len(columns))
		args := make([]any, len(columns))
		for i, col := range columns {
			parts[i] = col + " " + op + " ? ESCAPE '" + string(likeEscape) + "'"
			args[i] = pattern
		}
		return db.Where(strings.Join(parts, " OR "), args...)
	}
}

// escapeLike 转义 LIKE 的特殊字符，顺序很重要：先转义符本身。
func escapeLike(s string) string {
	var b strings.Builder
	b.Grow(len(s) + 4)
	for _, r := range s {
		if r == likeEscape || r == '%' || r == '_' {
			b.WriteRune(likeEscape)
		}
		b.WriteRune(r)
	}
	return b.String()
}
