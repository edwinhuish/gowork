package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/pkg/response"
	"github.com/edwinhuish/gowork/internal/service"
)

// WarehouseHandler 维护仓库主数据（T-56）。
type WarehouseHandler struct {
	warehouses service.WarehouseService
}

func NewWarehouseHandler(warehouses service.WarehouseService) *WarehouseHandler {
	return &WarehouseHandler{warehouses: warehouses}
}

// List GET /api/v1/warehouses
func (h *WarehouseHandler) List(c *gin.Context) {
	rows, err := h.warehouses.List(c.Request.Context())
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, rows)
}

// Create POST /api/v1/warehouses
func (h *WarehouseHandler) Create(c *gin.Context) {
	var in dto.WarehouseInput
	if !bindJSON(c, &in) {
		return
	}
	m, err := h.warehouses.Create(c.Request.Context(), in)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.Created(c, m)
}

// Update PUT /api/v1/warehouses/:id
func (h *WarehouseHandler) Update(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var in dto.WarehouseInput
	if !bindJSON(c, &in) {
		return
	}
	if err := h.warehouses.Update(c.Request.Context(), id, in); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"id": id})
}
