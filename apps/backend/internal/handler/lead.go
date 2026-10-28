package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/pkg/response"
	"github.com/edwinhuish/gowork/internal/service"
)

type LeadHandler struct {
	leads service.LeadService
}

func NewLeadHandler(leads service.LeadService) *LeadHandler {
	return &LeadHandler{leads: leads}
}

// List GET /api/v1/leads
func (h *LeadHandler) List(c *gin.Context) {
	var q dto.LeadQuery
	if !bindQuery(c, &q) {
		return
	}
	res, err := h.leads.List(c.Request.Context(), q)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, res)
}

// Get GET /api/v1/leads/:id
func (h *LeadHandler) Get(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	m, err := h.leads.Get(c.Request.Context(), id)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, m)
}

// Create POST /api/v1/leads
func (h *LeadHandler) Create(c *gin.Context) {
	var in dto.LeadInput
	if !bindJSON(c, &in) {
		return
	}
	m, err := h.leads.Create(c.Request.Context(), in)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.Created(c, m)
}

// Update PUT /api/v1/leads/:id
func (h *LeadHandler) Update(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var in dto.LeadInput
	if !bindJSON(c, &in) {
		return
	}
	m, err := h.leads.Update(c.Request.Context(), id, in)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, m)
}

// Delete DELETE /api/v1/leads/:id
func (h *LeadHandler) Delete(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	if err := h.leads.Delete(c.Request.Context(), id); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"deleted": true})
}

// Convert POST /api/v1/leads/:id/convert
//
// 转化是独立动作而不是"把 status 改成已转化"：它要建主体、回填 partner_id、
// 写转化时间，三件事必须一起发生。
func (h *LeadHandler) Convert(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var in dto.LeadConvertInput
	if !bindJSON(c, &in) {
		return
	}
	res, err := h.leads.Convert(c.Request.Context(), id, in)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.Created(c, res)
}
