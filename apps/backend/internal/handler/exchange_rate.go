package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/pkg/response"
	"github.com/edwinhuish/gowork/internal/service"
)

// ExchangeRateHandler 只挂四个动作：列表、详情、新增、删除。
// 没有 PUT /:id —— 历史汇率不可改，理由见 service.ExchangeRateService 的注释。
type ExchangeRateHandler struct {
	rates service.ExchangeRateService
}

func NewExchangeRateHandler(rates service.ExchangeRateService) *ExchangeRateHandler {
	return &ExchangeRateHandler{rates: rates}
}

// List GET /api/v1/exchange-rates
func (h *ExchangeRateHandler) List(c *gin.Context) {
	var q dto.ExchangeRateQuery
	if !bindQuery(c, &q) {
		return
	}
	res, err := h.rates.List(c.Request.Context(), q)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, res)
}

// Get GET /api/v1/exchange-rates/:id
func (h *ExchangeRateHandler) Get(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	m, err := h.rates.Get(c.Request.Context(), id)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, m)
}

// Create POST /api/v1/exchange-rates
func (h *ExchangeRateHandler) Create(c *gin.Context) {
	var in dto.ExchangeRateInput
	if !bindJSON(c, &in) {
		return
	}
	m, err := h.rates.Create(c.Request.Context(), in)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.Created(c, m)
}

// Delete DELETE /api/v1/exchange-rates/:id
func (h *ExchangeRateHandler) Delete(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	if err := h.rates.Delete(c.Request.Context(), id); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"deleted": true})
}
