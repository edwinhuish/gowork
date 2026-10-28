package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/pkg/response"
	"github.com/edwinhuish/gowork/internal/service"
)

type ReceivableHandler struct {
	receivables service.ReceivableService
}

func NewReceivableHandler(receivables service.ReceivableService) *ReceivableHandler {
	return &ReceivableHandler{receivables: receivables}
}

// List GET /api/v1/receivables
func (h *ReceivableHandler) List(c *gin.Context) {
	var q dto.ReceivableQuery
	if !bindQuery(c, &q) {
		return
	}
	res, err := h.receivables.List(c.Request.Context(), q)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, res)
}

// Get GET /api/v1/receivables/:id
func (h *ReceivableHandler) Get(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	m, err := h.receivables.Get(c.Request.Context(), id)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, m)
}

// Create POST /api/v1/receivables
func (h *ReceivableHandler) Create(c *gin.Context) {
	var in dto.ReceivableInput
	if !bindJSON(c, &in) {
		return
	}
	m, err := h.receivables.Create(c.Request.Context(), in)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.Created(c, m)
}

// Update PUT /api/v1/receivables/:id
func (h *ReceivableHandler) Update(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var in dto.ReceivableInput
	if !bindJSON(c, &in) {
		return
	}
	m, err := h.receivables.Update(c.Request.Context(), id, in)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, m)
}

// Delete DELETE /api/v1/receivables/:id
func (h *ReceivableHandler) Delete(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	if err := h.receivables.Delete(c.Request.Context(), id); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"deleted": true})
}
