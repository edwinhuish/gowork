package money

import (
	"encoding/json"
	"testing"
)

// 这条是整个改造的目的：float64 下 0.1+0.2 是 0.30000000000000004，
// 累加几十行金额之后就是"少了几分钱"，而账龄、毛利、汇兑损益都要能和明细逐笔对上。
func TestDecimalAddIsExact(t *testing.T) {
	sum := MustParse("0.1").Add(MustParse("0.2"))
	if sum.Cmp(MustParse("0.3")) != 0 {
		t.Fatalf("0.1 + 0.2 = %s，期望 0.3", sum)
	}

	// 一百次 0.01 必须正好是 1：这条在 float64 下会差 7e-17。
	total := Zero()
	for i := 0; i < 100; i++ {
		total = total.Add(MustParse("0.01"))
	}
	if total.String() != "1" {
		t.Fatalf("100 × 0.01 = %s，期望 1", total)
	}
}

func TestDecimalParseAndString(t *testing.T) {
	cases := []struct{ in, want string }{
		{"0", "0"},
		{"1", "1"},
		{"1.23", "1.23"},
		{"-1.23", "-1.23"},
		{"8", "8"},
		{"8.000000", "8"},
		{"0.001", "0.001"},
		{" 12.50 ", "12.5"},
		{"+7", "7"},
		{"-0", "0"},
	}
	for _, tc := range cases {
		got, err := Parse(tc.in)
		if err != nil {
			t.Fatalf("Parse(%q) 出错: %v", tc.in, err)
		}
		if got.String() != tc.want {
			t.Errorf("Parse(%q).String() = %q，期望 %q", tc.in, got.String(), tc.want)
		}
	}

	// 超过 Scale 位时四舍五入：录入端多给一位不该变成解析失败。
	if got := MustParse("1.2345678").String(); got != "1.234568" {
		t.Errorf("1.2345678 → %q，期望 1.234568", got)
	}
	// 进位要能跨过整数位。
	if got := MustParse("0.9999999").String(); got != "1" {
		t.Errorf("0.9999999 → %q，期望 1", got)
	}

	for _, bad := range []string{"1.2.3", "abc", "1e3", "1,234", "-", "1.2a"} {
		if _, err := Parse(bad); err == nil {
			t.Errorf("Parse(%q) 应报错", bad)
		}
	}
}

func TestDecimalRound(t *testing.T) {
	cases := []struct {
		in     string
		dp     int
		expect string
	}{
		{"1.2345", 2, "1.23"},
		{"1.2355", 2, "1.24"},
		// 2.675 在 float64 里是 2.67499999…，用 float 舍入会得到 2.67。
		{"2.675", 2, "2.68"},
		{"-1.235", 2, "-1.24"},
		{"7.123456", 4, "7.1235"},
		{"1000", 2, "1000.00"},
	}
	for _, tc := range cases {
		got := MustParse(tc.in).Round(tc.dp).StringFixed(tc.dp)
		if got != tc.expect {
			t.Errorf("%s 舍入到 %d 位 = %s，期望 %s", tc.in, tc.dp, got, tc.expect)
		}
	}
}

func TestDecimalMulDiv(t *testing.T) {
	// 数量 × 单价：40 × 12.3456 = 493.824
	if got := MustParse("12.3456").Mul(FromInt(40)).String(); got != "493.824" {
		t.Errorf("12.3456 × 40 = %s，期望 493.824", got)
	}
	// 本币折算：493.824 × 7.1 = 3506.1504
	if got := MustParse("493.824").Mul(MustParse("7.1")).String(); got != "3506.1504" {
		t.Errorf("493.824 × 7.1 = %s，期望 3506.1504", got)
	}
	// 除法：1 / 3 保留 6 位
	if got := MustParse("1").Div(FromInt(3)).String(); got != "0.333333" {
		t.Errorf("1 / 3 = %s，期望 0.333333", got)
	}
	// 除零返回 0 而不是 panic：报表里有"占比"这类指标，分母为 0 是常态。
	if got := MustParse("1").Div(Zero()); !got.IsZero() {
		t.Errorf("除以 0 应得 0，得到 %s", got)
	}
}

func TestDecimalJSON(t *testing.T) {
	type row struct {
		Amount Decimal `json:"amount"`
	}
	b, err := json.Marshal(row{Amount: MustParse("1234.56")})
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}
	// 必须是字符串：JSON 数字是双精度，写成数字等于把这次的改造丢掉。
	if string(b) != `{"amount":"1234.56"}` {
		t.Errorf("序列化结果 = %s", b)
	}

	var back row
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatalf("反序列化失败: %v", err)
	}
	if back.Amount.String() != "1234.56" {
		t.Errorf("回读 = %s", back.Amount)
	}

	// 过渡期前端可能仍发数字，解析要容忍。
	if err := json.Unmarshal([]byte(`{"amount":7.5}`), &back); err != nil {
		t.Fatalf("数字入参应被容忍: %v", err)
	}
	if back.Amount.String() != "7.5" {
		t.Errorf("数字入参回读 = %s", back.Amount)
	}
}

func TestDecimalScan(t *testing.T) {
	cases := []struct {
		src  any
		want string
	}{
		{[]byte("1234.560000"), "1234.56"},
		{"-8.500000", "-8.5"},
		{int64(12), "12"},
		// SQLite 的 NUMERIC 列会把 DECIMAL 存成 REAL，读回来是 float64。
		{float64(1.23), "1.23"},
		{nil, "0"},
	}
	for _, tc := range cases {
		var d Decimal
		if err := d.Scan(tc.src); err != nil {
			t.Fatalf("Scan(%v) 出错: %v", tc.src, err)
		}
		if d.String() != tc.want {
			t.Errorf("Scan(%#v) = %s，期望 %s", tc.src, d, tc.want)
		}
	}

	v, err := MustParse("1.23").Value()
	if err != nil {
		t.Fatalf("Value 出错: %v", err)
	}
	// 写库用字符串：DECIMAL 列要的是十进制字面量，float64 会被驱动再转一次。
	if v != "1.230000" {
		t.Errorf("Value = %v，期望 1.230000", v)
	}
}
