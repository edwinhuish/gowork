package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/pkg/response"
	"github.com/edwinhuish/gowork/internal/service"
)

type ProductHandler struct {
	products service.ProductService
}

func NewProductHandler(products service.ProductService) *ProductHandler {
	return &ProductHandler{products: products}
}

// List GET /api/v1/products
func (h *ProductHandler) List(c *gin.Context) {
	var q dto.ProductQuery
	if !bindQuery(c, &q) {
		return
	}

	res, err := h.products.List(c.Request.Context(), q)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, res)
}

// Get GET /api/v1/products/:id
func (h *ProductHandler) Get(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}

	m, err := h.products.Get(c.Request.Context(), id)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, m)
}

// Create POST /api/v1/products
func (h *ProductHandler) Create(c *gin.Context) {
	var in dto.ProductInput
	if !bindJSON(c, &in) {
		return
	}

	m, err := h.products.Create(c.Request.Context(), in)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.Created(c, m)
}

// Update PUT /api/v1/products/:id
func (h *ProductHandler) Update(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var in dto.ProductInput
	if !bindJSON(c, &in) {
		return
	}

	m, err := h.products.Update(c.Request.Context(), id, in)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, m)
}

// Delete DELETE /api/v1/products/:id
func (h *ProductHandler) Delete(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}

	if err := h.products.Delete(c.Request.Context(), id); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"deleted": true})
}
