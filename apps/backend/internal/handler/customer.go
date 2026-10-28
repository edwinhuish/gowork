package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/pkg/response"
	"github.com/edwinhuish/gowork/internal/service"
)

type CustomerHandler struct {
	customers service.CustomerService
}

func NewCustomerHandler(customers service.CustomerService) *CustomerHandler {
	return &CustomerHandler{customers: customers}
}

func (h *CustomerHandler) List(c *gin.Context) {
	var q dto.CustomerQuery
	if !bindQuery(c, &q) {
		return
	}

	res, err := h.customers.List(c.Request.Context(), q)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, res)
}

func (h *CustomerHandler) Get(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}

	m, err := h.customers.Get(c.Request.Context(), id)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, m)
}

func (h *CustomerHandler) Create(c *gin.Context) {
	var in dto.CustomerInput
	if !bindJSON(c, &in) {
		return
	}

	m, err := h.customers.Create(c.Request.Context(), in)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.Created(c, m)
}

func (h *CustomerHandler) Update(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var in dto.CustomerInput
	if !bindJSON(c, &in) {
		return
	}

	m, err := h.customers.Update(c.Request.Context(), id, in)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, m)
}

func (h *CustomerHandler) Delete(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}

	if err := h.customers.Delete(c.Request.Context(), id); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"deleted": true})
}
