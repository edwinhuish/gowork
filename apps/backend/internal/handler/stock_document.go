package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/pkg/response"
	"github.com/edwinhuish/gowork/internal/service"
)

// StockDocumentHandler 暴露出入库单。
type StockDocumentHandler struct {
	docs service.StockDocumentService
}

func NewStockDocumentHandler(docs service.StockDocumentService) *StockDocumentHandler {
	return &StockDocumentHandler{docs: docs}
}

// List GET /api/v1/stock-documents
func (h *StockDocumentHandler) List(c *gin.Context) {
	var q dto.StockDocumentQuery
	if !bindQuery(c, &q) {
		return
	}
	res, err := h.docs.List(c.Request.Context(), q)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, res)
}

// Get GET /api/v1/stock-documents/:id
func (h *StockDocumentHandler) Get(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	doc, err := h.docs.Get(c.Request.Context(), id)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, doc)
}

// Create POST /api/v1/stock-documents
func (h *StockDocumentHandler) Create(c *gin.Context) {
	var in dto.StockDocumentInput
	if !bindJSON(c, &in) {
		return
	}
	doc, err := h.docs.Create(c.Request.Context(), in)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.Created(c, doc)
}

// Update PUT /api/v1/stock-documents/:id
func (h *StockDocumentHandler) Update(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var in dto.StockDocumentInput
	if !bindJSON(c, &in) {
		return
	}
	doc, err := h.docs.Update(c.Request.Context(), id, in)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, doc)
}

// Delete DELETE /api/v1/stock-documents/:id
func (h *StockDocumentHandler) Delete(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	if err := h.docs.Delete(c.Request.Context(), id); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"deleted": true})
}

// Confirm POST /api/v1/stock-documents/:id/confirm
//
// 用 POST 而不是改成 status：确认这一步是动词，而且只有一次——
// 重复确认该是 409，不是"把状态改成已确认"这种看着幂等的操作。
func (h *StockDocumentHandler) Confirm(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	doc, err := h.docs.Confirm(c.Request.Context(), id)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, doc)
}
