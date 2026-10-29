package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/pkg/response"
	"github.com/edwinhuish/gowork/internal/service"
)

// AuditHandler 是操作日志的只读入口。
//
// 只有 List：日志不可改，所以也就没有 create / update / delete 三个 handler。
type AuditHandler struct {
	audits service.AuditService
}

func NewAuditHandler(audits service.AuditService) *AuditHandler {
	return &AuditHandler{audits: audits}
}

// List GET /api/v1/audit-logs
//
// 参数：page / page_size、actor_id、entity、entity_id、action、
// from / to（按发生时间，闭区间且 To 含当天）、keyword（路径 / 请求体 / 操作人）。
func (h *AuditHandler) List(c *gin.Context) {
	var q dto.AuditLogQuery
	if !bindQuery(c, &q) {
		return
	}
	res, err := h.audits.List(c.Request.Context(), q)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, res)
}
