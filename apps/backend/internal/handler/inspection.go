package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/pkg/response"
	"github.com/edwinhuish/gowork/internal/service"
)

// InspectionHandler 是验货记录的入口（T-51b）。
//
// 与收货单同一套路径约定：列表与新增挂在采购单下，删除挂在记录自己的前缀下。
type InspectionHandler struct {
	inspections service.InspectionService
}

func NewInspectionHandler(inspections service.InspectionService) *InspectionHandler {
	return &InspectionHandler{inspections: inspections}
}

// List GET /api/v1/purchase-orders/:id/inspections
func (h *InspectionHandler) List(c *gin.Context) {
	orderID, ok := parseID(c)
	if !ok {
		return
	}
	rows, err := h.inspections.List(c.Request.Context(), orderID)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, rows)
}

// Create POST /api/v1/purchase-orders/:id/inspections
func (h *InspectionHandler) Create(c *gin.Context) {
	orderID, ok := parseID(c)
	if !ok {
		return
	}
	var in dto.InspectionInput
	if !bindJSON(c, &in) {
		return
	}
	m, err := h.inspections.Create(c.Request.Context(), orderID, in)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.Created(c, m)
}

// Delete DELETE /api/v1/inspections/:id
func (h *InspectionHandler) Delete(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	if err := h.inspections.Delete(c.Request.Context(), id); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, nil)
}
