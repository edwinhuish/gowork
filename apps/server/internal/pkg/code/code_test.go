package code

import "testing"

func TestNormalize(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"ABC-123", "ABC123"},
		{"abc 123", "ABC123"},
		{"abc_123", "ABC123"},
		{"ABC.123", "ABC123"},
		{"ABC/123", "ABC123"},
		{"  ABC123  ", "ABC123"},
		{"ABC　123", "ABC123"}, // 全角空格也是空格
		{"", ""},
		// 只去掉分隔符，不去掉其它字符：A/B 与 AB 是两个号，不能混成一个。
		{"A+B", "A+B"},
		{"编号-01", "编号01"},
		// 全是分隔符时归一化结果为空，调用方要靠它判断"这个词搜不了编号"。
		{"---", ""},
	}

	for _, tc := range cases {
		if got := Normalize(tc.in); got != tc.want {
			t.Errorf("Normalize(%q) = %q，期望 %q", tc.in, got, tc.want)
		}
	}
}
