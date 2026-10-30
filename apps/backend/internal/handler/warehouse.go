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
//
// ?include_deleted=1 把已软删的仓库一并列出：列表页靠它给恢复留一个入口，
// 而下拉选仓库永远不带这个参数——删掉的仓不该出现在可选范围之内。
func (h *WarehouseHandler) List(c *gin.Context) {
	includeDeleted := c.Query("include_deleted") == "1" || c.Query("include_deleted") == "true"
	rows, err := h.warehouses.List(c.Request.Context(), includeDeleted)
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

// Delete DELETE /api/v1/warehouses/:id
func (h *WarehouseHandler) Delete(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	if err := h.warehouses.Delete(c.Request.Context(), id); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"deleted": true})
}

// Restore PUT /api/v1/warehouses/:id/restore
func (h *WarehouseHandler) Restore(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	m, err := h.warehouses.Restore(c.Request.Context(), id)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, m)
}
