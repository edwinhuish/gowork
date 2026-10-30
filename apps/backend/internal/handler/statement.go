package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/pkg/errs"
	"github.com/edwinhuish/gowork/internal/pkg/response"
	"github.com/edwinhuish/gowork/internal/service"
)

// StatementHandler 出客户对账单（T-53）。
type StatementHandler struct {
	statements service.StatementService
	pdf        service.PDFService
}

func NewStatementHandler(statements service.StatementService, pdf service.PDFService) *StatementHandler {
	return &StatementHandler{statements: statements, pdf: pdf}
}

// Receivable GET /api/v1/reports/receivable-statement?partner_id=&from=&to=&currency_code=
//
// 挂在 /reports 下而不是 /receivables 下：它不是一个资源，是一种聚合视图，
// 与账龄、单票毛利同类。
func (h *StatementHandler) Receivable(c *gin.Context) {
	var q dto.StatementQuery
	if !bindQuery(c, &q) {
		return
	}
	res, err := h.statements.Statement(c.Request.Context(), q)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, res)
}

// ReceivablePDF GET /api/v1/reports/receivable-statement/pdf?partner_id=&from=&to=&currency_code=
//
// 入参与 JSON 版完全一致：客户收到的那张纸，必须就是他刚才在屏幕上核过的那份数——
// 两套参数就会出现"页面说欠 3 万、PDF 说欠 2.8 万"。
func (h *StatementHandler) ReceivablePDF(c *gin.Context) {
	var q dto.StatementQuery
	if !bindQuery(c, &q) {
		return
	}
	if h.pdf == nil {
		response.Fail(c, errs.New(errs.CodeInternal, "未配置 PDF 中文字体（pdf.font_path），打印不可用"))
		return
	}
	f, err := h.pdf.ReceivableStatement(c.Request.Context(), q)
	if err != nil {
		response.Fail(c, err)
		return
	}
	writeFile(c, f)
}
