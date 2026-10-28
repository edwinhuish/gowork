package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/pkg/response"
	"github.com/edwinhuish/gowork/internal/service"
)

type PayableHandler struct {
	payables service.PayableService
}

func NewPayableHandler(payables service.PayableService) *PayableHandler {
	return &PayableHandler{payables: payables}
}

// List GET /api/v1/payables
func (h *PayableHandler) List(c *gin.Context) {
	var q dto.PayableQuery
	if !bindQuery(c, &q) {
		return
	}
	res, err := h.payables.List(c.Request.Context(), q)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, res)
}

// Get GET /api/v1/payables/:id
func (h *PayableHandler) Get(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	m, err := h.payables.Get(c.Request.Context(), id)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, m)
}

// Create POST /api/v1/payables
func (h *PayableHandler) Create(c *gin.Context) {
	var in dto.PayableInput
	if !bindJSON(c, &in) {
		return
	}
	m, err := h.payables.Create(c.Request.Context(), in)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.Created(c, m)
}

// Update PUT /api/v1/payables/:id
func (h *PayableHandler) Update(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var in dto.PayableInput
	if !bindJSON(c, &in) {
		return
	}
	m, err := h.payables.Update(c.Request.Context(), id, in)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, m)
}

// Delete DELETE /api/v1/payables/:id
func (h *PayableHandler) Delete(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	if err := h.payables.Delete(c.Request.Context(), id); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"deleted": true})
}
