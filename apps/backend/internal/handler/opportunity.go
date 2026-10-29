package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/pkg/response"
	"github.com/edwinhuish/gowork/internal/service"
)

type OpportunityHandler struct {
	opportunities service.OpportunityService
}

func NewOpportunityHandler(opportunities service.OpportunityService) *OpportunityHandler {
	return &OpportunityHandler{opportunities: opportunities}
}

// List GET /api/v1/opportunities
func (h *OpportunityHandler) List(c *gin.Context) {
	var q dto.OpportunityQuery
	if !bindQuery(c, &q) {
		return
	}
	res, err := h.opportunities.List(c.Request.Context(), q)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, res)
}

// Summary GET /api/v1/opportunities-summary
//
// 单独一条路径而不是 /opportunities/summary：后者会与 /opportunities/:id
// 抢同一段路径，路由树的静态段与通配段谁先注册谁能命中取决于注册顺序，
// 那是一种"加一个接口就 404 一个接口"的脆弱设计。
func (h *OpportunityHandler) Summary(c *gin.Context) {
	var q dto.OpportunityQuery
	if !bindQuery(c, &q) {
		return
	}
	res, err := h.opportunities.Summary(c.Request.Context(), q)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, res)
}

// Get GET /api/v1/opportunities/:id
func (h *OpportunityHandler) Get(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	m, err := h.opportunities.Get(c.Request.Context(), id)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, m)
}

// Create POST /api/v1/opportunities
func (h *OpportunityHandler) Create(c *gin.Context) {
	var in dto.OpportunityInput
	if !bindJSON(c, &in) {
		return
	}
	m, err := h.opportunities.Create(c.Request.Context(), in)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.Created(c, m)
}

// Update PUT /api/v1/opportunities/:id
func (h *OpportunityHandler) Update(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var in dto.OpportunityInput
	if !bindJSON(c, &in) {
		return
	}
	m, err := h.opportunities.Update(c.Request.Context(), id, in)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, m)
}

// Revalue POST /api/v1/opportunities/:id/revalue
//
// 按今日汇率重估本币金额（T-61）。与 PUT 分开：保存一次就把预测重估一遍，
// 等于让"改备注"顺手改了金额。
func (h *OpportunityHandler) Revalue(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	m, err := h.opportunities.Revalue(c.Request.Context(), id)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, m)
}

// Delete DELETE /api/v1/opportunities/:id
func (h *OpportunityHandler) Delete(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	if err := h.opportunities.Delete(c.Request.Context(), id); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"deleted": true})
}
