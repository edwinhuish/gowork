package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/pkg/response"
	"github.com/edwinhuish/gowork/internal/service"
)

type SupplierHandler struct {
	suppliers service.SupplierService
}

func NewSupplierHandler(suppliers service.SupplierService) *SupplierHandler {
	return &SupplierHandler{suppliers: suppliers}
}

func (h *SupplierHandler) List(c *gin.Context) {
	var q dto.SupplierQuery
	if !bindQuery(c, &q) {
		return
	}

	res, err := h.suppliers.List(c.Request.Context(), q)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, res)
}

func (h *SupplierHandler) Get(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}

	m, err := h.suppliers.Get(c.Request.Context(), id)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, m)
}

func (h *SupplierHandler) Create(c *gin.Context) {
	var in dto.SupplierInput
	if !bindJSON(c, &in) {
		return
	}

	m, err := h.suppliers.Create(c.Request.Context(), in)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.Created(c, m)
}

func (h *SupplierHandler) Update(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var in dto.SupplierInput
	if !bindJSON(c, &in) {
		return
	}

	m, err := h.suppliers.Update(c.Request.Context(), id, in)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, m)
}

func (h *SupplierHandler) Delete(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}

	if err := h.suppliers.Delete(c.Request.Context(), id); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"deleted": true})
}
