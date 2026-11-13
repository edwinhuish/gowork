package money

import (
	"database/sql/driver"
	"fmt"
	"math"
	"math/big"
	"strconv"
	"strings"
)

// Scale 是 Decimal 的小数位：金额与汇率统一按 1e-6 存。
//
// 6 位覆盖了现有全部用法：汇率要 4~6 位，金额要 2 位，单价与成本要 4 位。
// 再多的位数在业务上没有意义，而每多一位，int64 能表示的上限就少十倍。
const Scale = 6

// pow10 是一次性算好的 10^6，乘法与舍入都只用到它。
const pow10 int64 = 1_000_000

// Decimal 是定点小数：内部按「1e-6」为最小单位存成 int64。
//
// 为什么不用 float64：0.1+0.2 在二进制浮点里不等于 0.3，而单票毛利、账龄分桶、
// 汇兑损益都是把几十上百个金额相加之后再看结果——末位误差会累积成"少了 3 分钱"，
// 对账时无从解释。也不引第三方 decimal 库：需要的只有加减乘除与舍入，
// 自己写 40 行比多一个依赖更可控。
//
// 上限：int64 除以 1e6 约 9.2e12，即单笔 9.2 万亿元；超出时请先想想是不是单位错了。
type Decimal struct {
	// v 是最小单位数：1.23 存成 1230000。
	v int64
}

// Zero 返回 0。
func Zero() Decimal { return Decimal{} }

// FromInt 用整数构造。
func FromInt(i int64) Decimal { return Decimal{v: i * pow10} }

// FromUnits 用最小单位构造（读库、测试用）。
func FromUnits(units int64) Decimal { return Decimal{v: units} }

// FromFloat 从 float64 构造，按 Scale 位四舍五入。
//
// 只该用在边界上：旧数据、Excel 导入、第三方接口。业务计算一律走 Decimal 之间的运算，
// 中途转 float64 就等于把这次改造作废。
func FromFloat(f float64) Decimal {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return Decimal{}
	}
	scaled := f * float64(pow10)
	// 先四舍五入再截断：直接 int64() 会把 2.675 的尾差抹成 2.674999。
	return Decimal{v: int64(math.Round(scaled))}
}

// Parse 解析十进制字符串（可带正负号与小数点）。空串视为 0。
//
// 不做千分位与货币符号的处理：那些是显示层的写法，解析层替它猜反而会把
// "1,234" 读成 1 或 1234，两种都错得没有提示。
func Parse(s string) (Decimal, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return Decimal{}, nil
	}

	neg := false
	switch s[0] {
	case '-':
		neg = true
		s = s[1:]
	case '+':
		s = s[1:]
	}

	intPart, fracPart, hasDot := strings.Cut(s, ".")
	if intPart == "" && fracPart == "" {
		return Decimal{}, fmt.Errorf("金额格式不合法: %q", s)
	}
	if !hasDot {
		fracPart = ""
	}

	var units int64
	for _, r := range intPart {
		if r < '0' || r > '9' {
			return Decimal{}, fmt.Errorf("金额格式不合法: %q", s)
		}
		next, ok := mulAdd(units, int64(r-'0'))
		if !ok {
			return Decimal{}, fmt.Errorf("金额超出可表示范围: %q", s)
		}
		units = next
	}
	units, ok := safeMul(units, pow10)
	if !ok {
		return Decimal{}, fmt.Errorf("金额超出可表示范围: %q", s)
	}

	// 小数位逐位补到 Scale：多的部分四舍五入，少的部分补零。
	var fracUnits int64
	for i := 0; i < Scale; i++ {
		var digit int64
		if i < len(fracPart) {
			c := fracPart[i]
			if c < '0' || c > '9' {
				return Decimal{}, fmt.Errorf("金额格式不合法: %q", s)
			}
			digit = int64(c - '0')
		}
		fracUnits = fracUnits*10 + digit
	}
	if len(fracPart) > Scale && fracPart[Scale] >= '5' {
		fracUnits++
	}

	total, ok := safeAdd(units, fracUnits)
	if !ok {
		return Decimal{}, fmt.Errorf("金额超出可表示范围: %q", s)
	}
	if neg {
		total = -total
	}
	return Decimal{v: total}, nil
}

// MustParse 是 Parse 的 panic 版本，只给常量与测试用。
func MustParse(s string) Decimal {
	d, err := Parse(s)
	if err != nil {
		panic(err)
	}
	return d
}

// Add 相加。溢出时返回 0 而不是静默绕回：绕回的金额会以"看起来正常"的方式落库。
func (d Decimal) Add(o Decimal) Decimal {
	if sum, ok := safeAdd(d.v, o.v); ok {
		return Decimal{v: sum}
	}
	return Decimal{}
}

// Sub 相减，溢出处理同 Add。
func (d Decimal) Sub(o Decimal) Decimal {
	if diff, ok := safeAdd(d.v, -o.v); ok {
		return Decimal{v: diff}
	}
	return Decimal{}
}

// Mul 相乘（按 Scale 位四舍五入）。
//
// 用 big.Int 做中间积：金额（1e12 单位）× 汇率（1e7 单位）就已经到 1e19，
// 超过 int64。乘法不是热点路径，多一次分配换不掉坑。
func (d Decimal) Mul(o Decimal) Decimal {
	if d.v == 0 || o.v == 0 {
		return Decimal{}
	}
	p := new(big.Int).Mul(big.NewInt(d.v), big.NewInt(o.v))
	q, r := new(big.Int).QuoRem(p, big.NewInt(pow10), new(big.Int))
	// 余数的一半 ≥ 除数的一半就进位：向零取整会让利润每次都少一点。
	absR := new(big.Int).Abs(r)
	absR.Lsh(absR, 1)
	if absR.Cmp(big.NewInt(pow10)) >= 0 {
		if p.Sign() < 0 {
			q.Sub(q, big.NewInt(1))
		} else {
			q.Add(q, big.NewInt(1))
		}
	}
	if !q.IsInt64() {
		return Decimal{}
	}
	return Decimal{v: q.Int64()}
}

// MulInt 乘整数（数量、期数这类）。
func (d Decimal) MulInt(n int64) Decimal { return d.Mul(FromInt(n)) }

// Div 相除（按 Scale 位四舍五入）。除数为 0 时返回 0。
//
// 与 Mul 一样要显式进位：Quo 是向零截断，1/3 会得到 0.333333 而不是 0.333333，
// 但 2/3 截断成 0.666666 就比四舍五入少了末位——毛利率这类比例正是靠末位对账的。
func (d Decimal) Div(o Decimal) Decimal {
	if o.v == 0 {
		return Decimal{}
	}
	num := new(big.Int).Mul(big.NewInt(d.v), big.NewInt(pow10))
	den := big.NewInt(o.v)
	q, r := new(big.Int).QuoRem(num, den, new(big.Int))

	absR := new(big.Int).Abs(r)
	absR.Lsh(absR, 1)
	absDen := new(big.Int).Abs(den)
	if absR.Cmp(absDen) >= 0 {
		// 结果符号 = 被除数符号 × 除数符号；同号进位，异号退位。
		if num.Sign()*den.Sign() < 0 {
			q.Sub(q, big.NewInt(1))
		} else {
			q.Add(q, big.NewInt(1))
		}
	}
	if !q.IsInt64() {
		return Decimal{}
	}
	return Decimal{v: q.Int64()}
}

// Round 按业务口径舍入到 dp 位小数（dp ≤ Scale）。
//
// 这是"落到单据上的金额"的最后一刀：票面 2 位、单价 4 位都由它决定，
// 与 pkg/money 里既有的 float 版 Round 是同一套规则（四舍五入、负数对称）。
func (d Decimal) Round(dp int) Decimal {
	if dp >= Scale {
		return d
	}
	if dp < 0 {
		dp = 0
	}
	factor := pow10
	for i := 0; i < dp; i++ {
		factor /= 10
	}
	if factor == 0 {
		return d
	}

	// 先按 half-up 放大到以 factor 为单位，再缩回去。
	q, r := d.v/factor, d.v%factor
	if r < 0 {
		r = -r
	}
	if r*2 >= factor {
		if d.v < 0 {
			q--
		} else {
			q++
		}
	}
	return Decimal{v: q * factor}
}

// Float64 转回 float64，只供显示层与尚未改造的旧代码使用。
func (d Decimal) Float64() float64 { return float64(d.v) / float64(pow10) }

// IsZero 判断是否为 0。
func (d Decimal) IsZero() bool { return d.v == 0 }

// IsNegative 判断是否为负。
func (d Decimal) IsNegative() bool { return d.v < 0 }

// IsPositive 判断是否大于 0。
//
// 与 IsZero / IsNegative 并列而不是让调用方写 Cmp(Zero()) > 0：
// 数量与金额的"必须有值"校验到处都要，读起来越直白越不容易写反。
func (d Decimal) IsPositive() bool { return d.v > 0 }

// Neg 取反。
func (d Decimal) Neg() Decimal {
	if d.v == math.MinInt64 {
		return Decimal{}
	}
	return Decimal{v: -d.v}
}

// Abs 取绝对值。
func (d Decimal) Abs() Decimal {
	if d.v < 0 {
		return d.Neg()
	}
	return d
}

// Cmp 比较：-1 小于、0 相等、1 大于。
func (d Decimal) Cmp(o Decimal) int {
	switch {
	case d.v < o.v:
		return -1
	case d.v > o.v:
		return 1
	default:
		return 0
	}
}

// String 输出去掉尾零的十进制串（1.230000 → "1.23"）。
func (d Decimal) String() string { return d.StringFixed(-1) }

// StringFixed 输出固定 dp 位小数的十进制串；dp < 0 表示去掉尾零。
func (d Decimal) StringFixed(dp int) string {
	if d.v == 0 {
		if dp <= 0 {
			return "0"
		}
		return "0." + strings.Repeat("0", dp)
	}

	neg := d.v < 0
	units := d.v
	if neg {
		units = -units
	}
	intPart := units / pow10
	frac := units % pow10

	var b strings.Builder
	if neg {
		b.WriteByte('-')
	}
	b.WriteString(strconv.FormatInt(intPart, 10))

	if dp == 0 {
		return b.String()
	}
	fracStr := fmt.Sprintf("%06d", frac)
	if dp > 0 {
		if dp >= Scale {
			b.WriteByte('.')
			b.WriteString(fracStr)
			return b.String()
		}
		fracStr = fracStr[:dp]
		return b.String() + "." + fracStr
	}
	fracStr = strings.TrimRight(fracStr, "0")
	if fracStr == "" {
		return b.String()
	}
	return b.String() + "." + fracStr
}

// MarshalJSON 以字符串输出：JSON 的数字是 IEEE754 双精度，写成数字等于把精度丢掉。
func (d Decimal) MarshalJSON() ([]byte, error) {
	return []byte(`"` + d.String() + `"`), nil
}

// UnmarshalJSON 接受字符串，也容忍数字（过渡期前端仍可能发数字）。
func (d *Decimal) UnmarshalJSON(b []byte) error {
	s := strings.TrimSpace(string(b))
	if s == "null" || s == "" {
		*d = Decimal{}
		return nil
	}
	if strings.HasPrefix(s, `"`) && strings.HasSuffix(s, `"`) {
		s = s[1 : len(s)-1]
	}
	parsed, err := Parse(s)
	if err != nil {
		return err
	}
	*d = parsed
	return nil
}

// Scan 实现 sql.Scanner：三库读回来的形态不同（[]byte / string / float64）。
func (d *Decimal) Scan(src any) error {
	switch v := src.(type) {
	case nil:
		*d = Decimal{}
		return nil
	case []byte:
		return d.scanString(string(v))
	case string:
		return d.scanString(v)
	case int64:
		*d = FromInt(v)
		return nil
	case float64:
		// SQLite 的 NUMERIC 列会把 DECIMAL 存成 REAL；这里按 Scale 位四舍五入收回。
		*d = FromFloat(v)
		return nil
	default:
		return fmt.Errorf("无法把 %T 读成金额", src)
	}
}

func (d *Decimal) scanString(s string) error {
	parsed, err := Parse(s)
	if err != nil {
		return err
	}
	*d = parsed
	return nil
}

// Value 实现 driver.Valuer：写库一律给字符串。
//
// 给 float64 等于让驱动做一次二进制转换，而 DECIMAL 列要的是十进制字面量；
// 字符串在 mysql / postgres 的 DECIMAL 列上是精确写入，sqlite 会按 NUMERIC
// 亲和性转成 REAL（它没有真正的定点类型），读回来时由 Scan 按 Scale 位收回。
func (d Decimal) Value() (driver.Value, error) {
	return d.StringFixed(Scale), nil
}

// mulAdd 做 a*10+b 并检查溢出。
func mulAdd(a, b int64) (int64, bool) {
	if a > (math.MaxInt64-b)/10 {
		return 0, false
	}
	return a*10 + b, true
}

// safeMul 是带溢出检查的乘法（只用于 Parse 里的整数部分，两因子都非负）。
func safeMul(a, b int64) (int64, bool) {
	if a == 0 || b == 0 {
		return 0, true
	}
	if a > math.MaxInt64/b {
		return 0, false
	}
	return a * b, true
}

// safeAdd 是带溢出检查的加法。
func safeAdd(a, b int64) (int64, bool) {
	sum := a + b
	if (b > 0 && sum < a) || (b < 0 && sum > a) {
		return 0, false
	}
	return sum, true
}
