package pdf

import (
	"os"
	"testing"
)

func TestFill(t *testing.T) {
	values := map[string]string{"currency_code": "USD", "doc_no": "QT-202610-0001"}

	cases := []struct {
		in   string
		want string
	}{
		{"Total ({currency_code})", "Total (USD)"},
		{"No.: {doc_no}", "No.: QT-202610-0001"},
		// 缺键渲染成空串而不是原样保留：模板语法漏到客户面前比少一行字更糟。
		{"Remarks: {missing}", "Remarks: "},
		{"没有占位", "没有占位"},
	}
	for _, c := range cases {
		if got := fill(c.in, values); got != c.want {
			t.Errorf("fill(%q) = %q，期望 %q", c.in, got, c.want)
		}
	}
}

// TestBuiltinTemplates 校验内置模板能被解析，且明细列宽不超过可打印宽度。
//
// 列宽写错是模板最典型的错误：多出 5mm 在纸上就是右边两列挤在一起，
// 而这件事只能靠打开 PDF 才看得见——这条测试让它变成一个失败的用例。
func TestBuiltinTemplates(t *testing.T) {
	set, err := Load("")
	if err != nil {
		t.Fatalf("加载内置模板失败: %v", err)
	}

	for _, name := range []string{"quotation", "contract", "receivable-statement"} {
		tmpl, err := set.Get(name)
		if err != nil {
			t.Fatalf("取模板 %s 失败: %v", name, err)
		}
		if tmpl.Title == "" {
			t.Errorf("模板 %s 缺少 title", name)
		}
		if sum := columnWidth(tmpl.Items.Columns); sum > contentW+0.01 {
			t.Errorf("模板 %s 明细列宽合计 %.1fmm，超过可打印宽度 %.1fmm", name, sum, contentW)
		}
		for _, sec := range tmpl.Sections {
			if sum := columnWidth(sec.Columns); sum > contentW+0.01 {
				t.Errorf("模板 %s 附表 %q 列宽合计 %.1fmm，超过可打印宽度 %.1fmm",
					name, sec.Title, sum, contentW)
			}
		}
	}
}

func columnWidth(cols []Column) float64 {
	sum := 0.0
	for _, c := range cols {
		sum += c.Width
	}
	return sum
}

// TestCheckFontRejectsCJKOnly 锁住一个很贵的坑：Droid Sans Fallback 只含汉字，
// 用它渲染时 gopdf 不报错，只是把每个拉丁字符悄悄丢掉——
// 单号、金额、英文条款全没了，而 PDF 表面上"只是有点空"。
func TestCheckFontRejectsCJKOnly(t *testing.T) {
	const droid = "/usr/share/fonts/truetype/droid/DroidSansFallbackFull.ttf"
	if _, err := os.Stat(droid); err != nil {
		t.Skip("没有 DroidSansFallback，跳过")
	}
	if err := CheckFont(droid); err == nil {
		t.Fatal("只含汉字的回退字体应该被拒绝")
	}
}

// TestDetectFont 确认探测到的字体能通过同一套校验。
func TestDetectFont(t *testing.T) {
	path := DetectFont()
	if path == "" {
		t.Skip("系统没有可用的中文 TrueType 字体")
	}
	if err := CheckFont(path); err != nil {
		t.Fatalf("DetectFont 返回了不合格的字体 %s: %v", path, err)
	}
}

// TestLoadMissingTemplate 确认取不存在的模板时报错而不是给个空版式：
// 空版式会渲染出一张只有抬头的纸，没人会意识到是模板名写错了。
func TestLoadMissingTemplate(t *testing.T) {
	set, err := Load("")
	if err != nil {
		t.Fatalf("加载内置模板失败: %v", err)
	}
	if _, err := set.Get("nope"); err == nil {
		t.Fatal("取不存在的模板应该报错")
	}
}
