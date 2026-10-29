package csvx

import (
	"bytes"
	"encoding/csv"
	"strings"
	"testing"
)

// BOM 是这套输出存在的理由：没有它，Excel 会按本地代码页解码，
// 整列中文变乱码，而拿到文件的人只会说"你们导出的东西是坏的"。
func TestWriteStartsWithBOM(t *testing.T) {
	var buf bytes.Buffer
	if err := Write(&buf, []string{"编码", "名称"}, [][]string{{"C001", "张三"}}); err != nil {
		t.Fatalf("写出失败: %v", err)
	}

	out := buf.Bytes()
	if !bytes.HasPrefix(out, []byte{0xEF, 0xBB, 0xBF}) {
		t.Fatalf("缺少 UTF-8 BOM，前几个字节是 % x", out[:min(6, len(out))])
	}
	// BOM 之后必须是合法 UTF-8 中文，而不是二次编码。
	body := string(out[3:])
	if !strings.HasPrefix(body, "编码,名称") {
		t.Errorf("表头不对: %q", body)
	}
}

// 逗号、双引号、换行必须按 CSV 规则转义，否则一行会被拆成两列或两行。
func TestWriteEscapesAwkwardValues(t *testing.T) {
	var buf bytes.Buffer
	rows := [][]string{
		{"C001", `上海某某公司, 分部`},
		{"C002", `他说"先看看"`},
		{"C003", "第一行\n第二行"},
	}
	if err := Write(&buf, []string{"编码", "名称"}, rows); err != nil {
		t.Fatalf("写出失败: %v", err)
	}

	reader := csv.NewReader(bytes.NewReader(buf.Bytes()[3:]))
	records, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("写出的内容读不回来（转义有问题）: %v", err)
	}
	if len(records) != len(rows)+1 {
		t.Fatalf("应有 %d 行（含表头），得到 %d", len(rows)+1, len(records))
	}
	for i, want := range rows {
		got := records[i+1]
		if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
			t.Errorf("第 %d 行往返不一致: 期望 %q，得到 %q", i+1, want, got)
		}
	}
}

// 空行与空字符串也要写成"一列空值"，而不是少一列——少一列会让 Excel 里整行错位。
func TestWriteKeepsEmptyCells(t *testing.T) {
	var buf bytes.Buffer
	if err := Write(&buf, []string{"编码", "备注"}, [][]string{{"C001", ""}}); err != nil {
		t.Fatalf("写出失败: %v", err)
	}

	reader := csv.NewReader(bytes.NewReader(buf.Bytes()[3:]))
	records, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("读回失败: %v", err)
	}
	if len(records) != 2 || len(records[1]) != 2 {
		t.Fatalf("空单元格应保留为空列，得到 %q", records)
	}
	if records[1][1] != "" {
		t.Errorf("第二列应为空串，得到 %q", records[1][1])
	}
}

// 没有数据行时也要有表头 + BOM：导出的是一张"空表"，不是一个 0 字节文件。
func TestWriteHeaderOnly(t *testing.T) {
	var buf bytes.Buffer
	if err := Write(&buf, []string{"编码"}, nil); err != nil {
		t.Fatalf("写出失败: %v", err)
	}
	if got := buf.String(); got != "\ufeff编码\n" {
		t.Errorf("应只有 BOM + 表头，得到 %q", got)
	}
}
