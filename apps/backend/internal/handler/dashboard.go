package handler

import (
	"time"

	"github.com/gin-gonic/gin"

	"github.com/edwinhuish/gowork/internal/pkg/response"
	"github.com/edwinhuish/gowork/internal/service"
)

type DashboardHandler struct {
	dashboard service.DashboardService
}

func NewDashboardHandler(dashboard service.DashboardService) *DashboardHandler {
	return &DashboardHandler{dashboard: dashboard}
}

func (h *DashboardHandler) Summary(c *gin.Context) {
	res, err := h.dashboard.Summary(c.Request.Context())
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, res)
}

// Health 供负载均衡与容器健康检查使用，不参与鉴权。
func Health(c *gin.Context) {
	response.OK(c, gin.H{
		"status": "ok",
		"time":   time.Now().Format(time.RFC3339),
	})
}
