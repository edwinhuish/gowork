package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/pkg/errs"
	"github.com/edwinhuish/gowork/internal/pkg/response"
	"github.com/edwinhuish/gowork/internal/service"
)

type ShipmentChargeHandler struct {
	charges service.ShipmentChargeService
}

func NewShipmentChargeHandler(charges service.ShipmentChargeService) *ShipmentChargeHandler {
	return &ShipmentChargeHandler{charges: charges}
}

// List GET /api/v1/shipment-charges?shipment_id=1
func (h *ShipmentChargeHandler) List(c *gin.Context) {
	var q dto.ShipmentChargeQuery
	if !bindQuery(c, &q) {
		return
	}
	res, err := h.charges.List(c.Request.Context(), q)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, res)
}

// Get GET /api/v1/shipment-charges/:id
func (h *ShipmentChargeHandler) Get(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	m, err := h.charges.Get(c.Request.Context(), id)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, m)
}

// Create POST /api/v1/shipment-charges
func (h *ShipmentChargeHandler) Create(c *gin.Context) {
	var in dto.ShipmentChargeInput
	if !bindJSON(c, &in) {
		return
	}
	m, err := h.charges.Create(c.Request.Context(), in)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.Created(c, m)
}

// Update PUT /api/v1/shipment-charges/:id
func (h *ShipmentChargeHandler) Update(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var in dto.ShipmentChargeInput
	if !bindJSON(c, &in) {
		return
	}
	m, err := h.charges.Update(c.Request.Context(), id, in)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, m)
}

// Delete DELETE /api/v1/shipment-charges/:id
func (h *ShipmentChargeHandler) Delete(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	if err := h.charges.Delete(c.Request.Context(), id); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"deleted": true})
}

// Summary GET /api/v1/shipment-charges-summary?shipment_id=1
//
// 独立前缀而不是 /shipment-charges/:id/summary 之类的子路径：
// 与 /opportunities-summary、/quotations-preview 同一套做法，
// 固定段与 :id 并列时 gin 有歧义，独立的固定前缀最省事。
func (h *ShipmentChargeHandler) Summary(c *gin.Context) {
	var q dto.ShipmentChargeQuery
	if !bindQuery(c, &q) {
		return
	}
	if q.ShipmentID == nil || *q.ShipmentID == 0 {
		response.Fail(c, errs.BadRequest("缺少 shipment_id"))
		return
	}
	res, err := h.charges.Summary(c.Request.Context(), *q.ShipmentID)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, res)
}
