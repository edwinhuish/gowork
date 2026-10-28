package service

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/pdf"
)

// pdfSvc 构造 PDF 服务。没有可用中文字体时跳过：
// CI 镜像里常常没有 fonts-droid-fallback，而"打印"不是这些测试要证明的事。
func (f contractFixture) pdfSvc(t *testing.T) PDFService {
	t.Helper()
	fontPath := pdf.DetectFont()
	if fontPath == "" {
		t.Skip("系统没有中文 TrueType 字体，跳过 PDF 渲染测试")
	}
	set, err := pdf.Load("")
	if err != nil {
		t.Fatalf("加载内置模板失败: %v", err)
	}
	renderer, err := pdf.NewRenderer(set, fontPath)
	if err != nil {
		t.Fatalf("加载字体失败: %v", err)
	}
	return NewPDFService(f.quotationRepo, f.orderRepo, f.contractRepo, f.currencySvc, renderer,
		CompanyInfo{
			Name:    "Test Trading Co., Ltd.",
			Address: "Ningbo, Zhejiang, China",
			Contact: "sales@test.example",
		})
}

// assertPDF 只校验"确实生成了 PDF"这一层：内容对不对靠 PDF_OUT_DIR 落盘后人工打开。
//
// 用 PDF_OUT_DIR 而不是测试目录：测试结束后临时目录会被清掉，
// 排查版式问题时恰恰需要文件留着。
func assertPDF(t *testing.T, f *RenderedFile, wantName string) {
	t.Helper()
	if f.Filename != wantName {
		t.Errorf("文件名 = %q，期望 %q", f.Filename, wantName)
	}
	if !bytes.HasPrefix(f.Data, []byte("%PDF-")) {
		t.Fatalf("输出不是 PDF：%q", f.Data[:min(8, len(f.Data))])
	}
	// 小于 2KB 基本意味着只画出了抬头，没画出明细与合计。
	if len(f.Data) < 2000 {
		t.Errorf("PDF 只有 %d 字节，看起来没画出内容", len(f.Data))
	}

	if dir := os.Getenv("PDF_OUT_DIR"); dir != "" {
		path := filepath.Join(dir, wantName)
		if err := os.WriteFile(path, f.Data, 0o644); err != nil {
			t.Logf("写调试 PDF 失败: %v", err)
		} else {
			t.Logf("调试 PDF: %s", path)
		}
	}
}

func TestPDFQuotation(t *testing.T) {
	ctx := context.Background()
	fx := newContractFixture(t, time.Date(2026, 10, 28, 10, 0, 0, 0, time.Local))
	svc := fx.pdfSvc(t)

	q := fx.confirmedQuotation(t)
	f, err := svc.Quotation(ctx, q.ID)
	if err != nil {
		t.Fatalf("渲染报价单 PDF 失败: %v", err)
	}
	assertPDF(t, f, q.DocNo+".pdf")
}

func TestPDFContract(t *testing.T) {
	ctx := context.Background()
	fx := newContractFixture(t, time.Date(2026, 10, 28, 10, 0, 0, 0, time.Local))
	svc := fx.pdfSvc(t)

	q := fx.confirmedQuotation(t)
	c, err := fx.svc.Generate(ctx, dto.ContractGenerateInput{
		Type:       int8(model.ContractTypePI),
		SourceType: int8(model.ContractSourceQuotation),
		SourceID:   q.ID,
		SignDate:   "2026-10-28",
		Terms: []dto.ContractTermInput{
			{Name: "订金 30%", Amount: 300, DueDate: "2026-11-05"},
			{Name: "尾款 70%", Amount: 700, Trigger: "见提单副本后 7 天"},
		},
	})
	if err != nil {
		t.Fatalf("生成合同失败: %v", err)
	}

	// 合同不存明细：明细要回来源报价单取。这条同时验证了那条回源路径。
	f, err := svc.Contract(ctx, c.ID)
	if err != nil {
		t.Fatalf("渲染合同 PDF 失败: %v", err)
	}
	assertPDF(t, f, c.DocNo+".pdf")
}

func TestPDFContractMissingSource(t *testing.T) {
	ctx := context.Background()
	fx := newContractFixture(t, time.Date(2026, 10, 28, 10, 0, 0, 0, time.Local))
	svc := fx.pdfSvc(t)

	if _, err := svc.Quotation(ctx, 999999); err == nil {
		t.Fatal("报价单不存在时应该报错")
	}
	if _, err := svc.Contract(ctx, 999999); err == nil {
		t.Fatal("合同不存在时应该报错")
	}
}

// TestPDFTextHelpers 覆盖打印用的数字与文案格式。
//
// 这几条不依赖字体，所以在任何环境都跑。
func TestPDFTextHelpers(t *testing.T) {
	moneyCases := []struct {
		v    float64
		dp   int
		want string
	}{
		{1234567.5, 2, "1,234,567.50"},
		{0, 2, "0.00"},
		{0, 0, "0"},
		{-1234.5, 2, "-1,234.50"},
		{1000, 0, "1,000"},
	}
	for _, c := range moneyCases {
		if got := moneyText(c.v, c.dp); got != c.want {
			t.Errorf("moneyText(%v, %d) = %q，期望 %q", c.v, c.dp, got, c.want)
		}
	}

	qtyCases := []struct {
		v    float64
		want string
	}{
		{1000, "1,000"},
		{12.5, "12.5"},
		{0, "0"},
		{1234.567, "1,234.567"},
	}
	for _, c := range qtyCases {
		if got := qtyText(c.v); got != c.want {
			t.Errorf("qtyText(%v) = %q，期望 %q", c.v, got, c.want)
		}
	}

	if got := itemDescription("BH01-0001", "不锈钢合页", "100×75×3"); got != "BH01-0001 不锈钢合页 (100×75×3)" {
		t.Errorf("itemDescription = %q", got)
	}
	if got := contractTitle(model.ContractTypeSales); got != "SALES CONTRACT" {
		t.Errorf("销售合同标题 = %q", got)
	}
	if got := contractTitle(model.ContractTypePI); got != "PROFORMA INVOICE" {
		t.Errorf("PI 标题 = %q", got)
	}
}
