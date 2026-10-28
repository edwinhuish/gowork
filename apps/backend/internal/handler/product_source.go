package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/pkg/response"
	"github.com/edwinhuish/gowork/internal/service"
)

type ProductSourceHandler struct {
	sources service.ProductSourceService
}

func NewProductSourceHandler(sources service.ProductSourceService) *ProductSourceHandler {
	return &ProductSourceHandler{sources: sources}
}

// List GET /api/v1/product-sources?product_id=1
func (h *ProductSourceHandler) List(c *gin.Context) {
	var q dto.ProductSourceQuery
	if !bindQuery(c, &q) {
		return
	}
	res, err := h.sources.List(c.Request.Context(), q)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, res)
}

// Get GET /api/v1/product-sources/:id
func (h *ProductSourceHandler) Get(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	m, err := h.sources.Get(c.Request.Context(), id)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, m)
}

// Create POST /api/v1/product-sources
func (h *ProductSourceHandler) Create(c *gin.Context) {
	var in dto.ProductSourceInput
	if !bindJSON(c, &in) {
		return
	}
	m, err := h.sources.Create(c.Request.Context(), in)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.Created(c, m)
}

// Update PUT /api/v1/product-sources/:id
func (h *ProductSourceHandler) Update(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var in dto.ProductSourceInput
	if !bindJSON(c, &in) {
		return
	}
	m, err := h.sources.Update(c.Request.Context(), id, in)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, m)
}

// Delete DELETE /api/v1/product-sources/:id
func (h *ProductSourceHandler) Delete(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	if err := h.sources.Delete(c.Request.Context(), id); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"deleted": true})
}
