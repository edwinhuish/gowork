package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/pkg/response"
	"github.com/edwinhuish/gowork/internal/service"
)

type QuotationHandler struct {
	quotations service.QuotationService
}

func NewQuotationHandler(quotations service.QuotationService) *QuotationHandler {
	return &QuotationHandler{quotations: quotations}
}

// List GET /api/v1/quotations
func (h *QuotationHandler) List(c *gin.Context) {
	var q dto.QuotationQuery
	if !bindQuery(c, &q) {
		return
	}
	res, err := h.quotations.List(c.Request.Context(), q)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, res)
}

// Get GET /api/v1/quotations/:id
func (h *QuotationHandler) Get(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	m, err := h.quotations.Get(c.Request.Context(), id)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, m)
}

// Create POST /api/v1/quotations
func (h *QuotationHandler) Create(c *gin.Context) {
	var in dto.QuotationInput
	if !bindJSON(c, &in) {
		return
	}
	m, err := h.quotations.Create(c.Request.Context(), in)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.Created(c, m)
}

// Update PUT /api/v1/quotations/:id
func (h *QuotationHandler) Update(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var in dto.QuotationInput
	if !bindJSON(c, &in) {
		return
	}
	m, err := h.quotations.Update(c.Request.Context(), id, in)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, m)
}

// Delete DELETE /api/v1/quotations/:id
func (h *QuotationHandler) Delete(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	if err := h.quotations.Delete(c.Request.Context(), id); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"deleted": true})
}

// UpdateStatus PUT /api/v1/quotations/:id/status
//
// 状态流转单独一个接口而不是塞进 PUT：编辑明细与改变承诺是两件事，
// 混在一起之后"保存一下"顺手就把报价确认了。
func (h *QuotationHandler) UpdateStatus(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var in dto.QuotationStatusInput
	if !bindJSON(c, &in) {
		return
	}
	m, err := h.quotations.UpdateStatus(c.Request.Context(), id, in)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, m)
}

// NewVersion POST /api/v1/quotations/:id/versions
func (h *QuotationHandler) NewVersion(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var in dto.QuotationVersionInput
	if !bindJSON(c, &in) {
		return
	}
	m, err := h.quotations.NewVersion(c.Request.Context(), id, in)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.Created(c, m)
}

// Preview POST /api/v1/quotations-preview
//
// 独立前缀与 opportunities-summary 同理：避免新增静态段与 /:id 抢路径。
func (h *QuotationHandler) Preview(c *gin.Context) {
	var in dto.QuotationPreviewInput
	if !bindJSON(c, &in) {
		return
	}
	res, err := h.quotations.Preview(c.Request.Context(), in)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, res)
}
