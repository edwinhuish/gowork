package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/pkg/response"
	"github.com/edwinhuish/gowork/internal/service"
)

type CurrencyHandler struct {
	currencies service.CurrencyService
}

func NewCurrencyHandler(currencies service.CurrencyService) *CurrencyHandler {
	return &CurrencyHandler{currencies: currencies}
}

// List GET /api/v1/currencies
func (h *CurrencyHandler) List(c *gin.Context) {
	var q dto.CurrencyQuery
	if !bindQuery(c, &q) {
		return
	}
	res, err := h.currencies.List(c.Request.Context(), q)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, res)
}

// Get GET /api/v1/currencies/:id
func (h *CurrencyHandler) Get(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	m, err := h.currencies.Get(c.Request.Context(), id)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, m)
}

// Create POST /api/v1/currencies
func (h *CurrencyHandler) Create(c *gin.Context) {
	var in dto.CurrencyInput
	if !bindJSON(c, &in) {
		return
	}
	m, err := h.currencies.Create(c.Request.Context(), in)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.Created(c, m)
}

// Update PUT /api/v1/currencies/:id
func (h *CurrencyHandler) Update(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var in dto.CurrencyInput
	if !bindJSON(c, &in) {
		return
	}
	m, err := h.currencies.Update(c.Request.Context(), id, in)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, m)
}

// Delete DELETE /api/v1/currencies/:id
func (h *CurrencyHandler) Delete(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	if err := h.currencies.Delete(c.Request.Context(), id); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"deleted": true})
}
