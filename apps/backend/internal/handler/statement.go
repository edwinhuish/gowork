package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/pkg/response"
	"github.com/edwinhuish/gowork/internal/service"
)

// StatementHandler 出客户对账单（T-53）。
type StatementHandler struct {
	statements service.StatementService
}

func NewStatementHandler(statements service.StatementService) *StatementHandler {
	return &StatementHandler{statements: statements}
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
