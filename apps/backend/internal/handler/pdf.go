package handler

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/edwinhuish/gowork/internal/pkg/errs"
	"github.com/edwinhuish/gowork/internal/pkg/response"
	"github.com/edwinhuish/gowork/internal/service"
)

// PDFHandler 输出单据 PDF。
//
// 文件下载不走统一响应体：它是二进制附件，包一层 JSON 就没法直接存成文件了。
// 错误仍然走统一体，前端按 code 判断即可。
type PDFHandler struct {
	pdf service.PDFService
}

// NewPDFHandler 允许传 nil：没配中文字体时打印不可用，
// 但报价、合同这些业务照常跑，不该因为一个附加能力让服务起不来。
func NewPDFHandler(pdf service.PDFService) *PDFHandler {
	return &PDFHandler{pdf: pdf}
}

// ready 检查打印能力是否可用：不可用时写一个明确的错误并返回 false，
// 避免四个入口各抄一遍同一段提示。
func (h *PDFHandler) ready(c *gin.Context) bool {
	if h.pdf == nil {
		response.Fail(c, errs.New(errs.CodeInternal, "未配置 PDF 中文字体（pdf.font_path），打印不可用"))
		return false
	}
	return true
}

// Quotation GET /api/v1/quotations/:id/pdf
func (h *PDFHandler) Quotation(c *gin.Context) {
	if !h.ready(c) {
		return
	}
	id, ok := parseID(c)
	if !ok {
		return
	}
	f, err := h.pdf.Quotation(c.Request.Context(), id)
	if err != nil {
		response.Fail(c, err)
		return
	}
	writeFile(c, f)
}

// Contract GET /api/v1/contracts/:id/pdf
func (h *PDFHandler) Contract(c *gin.Context) {
	if !h.ready(c) {
		return
	}
	id, ok := parseID(c)
	if !ok {
		return
	}
	f, err := h.pdf.Contract(c.Request.Context(), id)
	if err != nil {
		response.Fail(c, err)
		return
	}
	writeFile(c, f)
}

// Invoice GET /api/v1/shipments/:id/invoice
//
// 商业发票与装箱单都不单独发号：它们印的是送货单的套号。
func (h *PDFHandler) Invoice(c *gin.Context) {
	if !h.ready(c) {
		return
	}
	id, ok := parseID(c)
	if !ok {
		return
	}
	f, err := h.pdf.Invoice(c.Request.Context(), id)
	if err != nil {
		response.Fail(c, err)
		return
	}
	writeFile(c, f)
}

// PackingList GET /api/v1/shipments/:id/packing-list
func (h *PDFHandler) PackingList(c *gin.Context) {
	if !h.ready(c) {
		return
	}
	id, ok := parseID(c)
	if !ok {
		return
	}
	f, err := h.pdf.PackingList(c.Request.Context(), id)
	if err != nil {
		response.Fail(c, err)
		return
	}
	writeFile(c, f)
}

// CartonLabels GET /api/v1/shipments/:id/carton-labels
func (h *PDFHandler) CartonLabels(c *gin.Context) {
	if !h.ready(c) {
		return
	}
	id, ok := parseID(c)
	if !ok {
		return
	}
	f, err := h.pdf.CartonLabels(c.Request.Context(), id)
	if err != nil {
		response.Fail(c, err)
		return
	}
	writeFile(c, f)
}

func writeFile(c *gin.Context, f *service.RenderedFile) {
	// filename 用引号包住：单号里不会出现引号，但将来要是加了客户简称就有可能出现，
	// 到那时这个头会直接把响应拆坏。
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, f.Filename))
	c.Header("Cache-Control", "private, no-store")
	c.Data(http.StatusOK, "application/pdf", f.Data)
}
