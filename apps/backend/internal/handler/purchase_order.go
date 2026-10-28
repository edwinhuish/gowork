package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/pkg/response"
	"github.com/edwinhuish/gowork/internal/service"
)

type PurchaseOrderHandler struct {
	orders service.PurchaseOrderService
}

func NewPurchaseOrderHandler(orders service.PurchaseOrderService) *PurchaseOrderHandler {
	return &PurchaseOrderHandler{orders: orders}
}

// List GET /api/v1/purchase-orders
func (h *PurchaseOrderHandler) List(c *gin.Context) {
	var q dto.PurchaseOrderQuery
	if !bindQuery(c, &q) {
		return
	}
	res, err := h.orders.List(c.Request.Context(), q)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, res)
}

// Get GET /api/v1/purchase-orders/:id
func (h *PurchaseOrderHandler) Get(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	m, err := h.orders.Get(c.Request.Context(), id)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, m)
}

// Create POST /api/v1/purchase-orders
func (h *PurchaseOrderHandler) Create(c *gin.Context) {
	var in dto.PurchaseOrderInput
	if !bindJSON(c, &in) {
		return
	}
	m, err := h.orders.Create(c.Request.Context(), in)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.Created(c, m)
}

// Update PUT /api/v1/purchase-orders/:id
func (h *PurchaseOrderHandler) Update(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var in dto.PurchaseOrderInput
	if !bindJSON(c, &in) {
		return
	}
	m, err := h.orders.Update(c.Request.Context(), id, in)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, m)
}

// Delete DELETE /api/v1/purchase-orders/:id
func (h *PurchaseOrderHandler) Delete(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	if err := h.orders.Delete(c.Request.Context(), id); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"deleted": true})
}

// UpdateStatus PUT /api/v1/purchase-orders/:id/status
func (h *PurchaseOrderHandler) UpdateStatus(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var in dto.PurchaseOrderStatusInput
	if !bindJSON(c, &in) {
		return
	}
	m, err := h.orders.UpdateStatus(c.Request.Context(), id, in)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, m)
}
