package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/pkg/response"
	"github.com/edwinhuish/gowork/internal/service"
)

// ReportHandler 是只读报表的 HTTP 入口。
//
// 报表全部挂在 /reports 下，不散在各资源后面当 `-summary`：
// 它们是跨表的聚合，既不属于某张单据，也不跟着某张单据的权限与状态机走。
type ReportHandler struct {
	reports service.ReportService
}

func NewReportHandler(reports service.ReportService) *ReportHandler {
	return &ReportHandler{reports: reports}
}

// Aging GET /api/v1/reports/aging
//
// 参数：direction=10|20（留空按应收）、partner_id、as_of=YYYY-MM-DD（留空取今天）。
func (h *ReportHandler) Aging(c *gin.Context) {
	var q dto.AgingQuery
	if !bindQuery(c, &q) {
		return
	}
	res, err := h.reports.Aging(c.Request.Context(), q)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, res)
}
