package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/pkg/response"
	"github.com/edwinhuish/gowork/internal/service"
)

type SalesOrderHandler struct {
	orders service.SalesOrderService
}

func NewSalesOrderHandler(orders service.SalesOrderService) *SalesOrderHandler {
	return &SalesOrderHandler{orders: orders}
}

// List GET /api/v1/sales-orders
func (h *SalesOrderHandler) List(c *gin.Context) {
	var q dto.SalesOrderQuery
	if !bindQuery(c, &q) {
		return
	}
	res, err := h.orders.List(c.Request.Context(), q)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, res)
}

// Get GET /api/v1/sales-orders/:id
func (h *SalesOrderHandler) Get(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	m, err := h.orders.Get(c.Request.Context(), id)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, m)
}

// Create POST /api/v1/sales-orders
func (h *SalesOrderHandler) Create(c *gin.Context) {
	var in dto.SalesOrderInput
	if !bindJSON(c, &in) {
		return
	}
	m, err := h.orders.Create(c.Request.Context(), in)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.Created(c, m)
}

// Update PUT /api/v1/sales-orders/:id
func (h *SalesOrderHandler) Update(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var in dto.SalesOrderInput
	if !bindJSON(c, &in) {
		return
	}
	m, err := h.orders.Update(c.Request.Context(), id, in)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, m)
}

// Delete DELETE /api/v1/sales-orders/:id
func (h *SalesOrderHandler) Delete(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	if err := h.orders.Delete(c.Request.Context(), id); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"deleted": true})
}

// UpdateStatus PUT /api/v1/sales-orders/:id/status
func (h *SalesOrderHandler) UpdateStatus(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var in dto.SalesOrderStatusInput
	if !bindJSON(c, &in) {
		return
	}
	m, err := h.orders.UpdateStatus(c.Request.Context(), id, in)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, m)
}

// Changes GET /api/v1/sales-orders/:id/changes
//
// 变更历史按序号倒序：最近一次改了什么，是翻旧账时的第一个问题。
func (h *SalesOrderHandler) Changes(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	list, err := h.orders.ListChanges(c.Request.Context(), id)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, list)
}

// Change POST /api/v1/sales-orders/:id/changes
//
// 已确认订单的改价 / 改量 / 改交期都走这里，不提供"直接 PUT"的旁路：
// 改得动却说不清谁改的，比改不动更糟。
func (h *SalesOrderHandler) Change(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var in dto.SalesOrderChangeInput
	if !bindJSON(c, &in) {
		return
	}
	m, err := h.orders.Change(c.Request.Context(), id, in)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, m)
}
