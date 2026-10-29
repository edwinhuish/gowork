package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/pkg/response"
	"github.com/edwinhuish/gowork/internal/service"
)

// PaymentHandler 是收付款单的 HTTP 入口。
//
// 没有单独的"核销"接口：核销行随收付款单一起创建、一起删除（见 service 的说明），
// 所以这里的五个动作就是全部——不给 PUT /:id/allocations 之类的就地修改留口子。
type PaymentHandler struct {
	payments service.PaymentService
}

func NewPaymentHandler(payments service.PaymentService) *PaymentHandler {
	return &PaymentHandler{payments: payments}
}

// List GET /api/v1/payments
func (h *PaymentHandler) List(c *gin.Context) {
	var q dto.PaymentQuery
	if !bindQuery(c, &q) {
		return
	}
	res, err := h.payments.List(c.Request.Context(), q)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, res)
}

// Get GET /api/v1/payments/:id
func (h *PaymentHandler) Get(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	m, err := h.payments.Get(c.Request.Context(), id)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, m)
}

// Create POST /api/v1/payments
func (h *PaymentHandler) Create(c *gin.Context) {
	var in dto.PaymentInput
	if !bindJSON(c, &in) {
		return
	}
	m, err := h.payments.Create(c.Request.Context(), in)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.Created(c, m)
}

// Update PUT /api/v1/payments/:id
//
// 只改日期、方式、水单号、备注；金额与核销明细不给改，录错了删掉重录。
func (h *PaymentHandler) Update(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var in dto.PaymentUpdateInput
	if !bindJSON(c, &in) {
		return
	}
	m, err := h.payments.Update(c.Request.Context(), id, in)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, m)
}

// RemoveAllocation DELETE /api/v1/payments/:id/allocations/:allocationId
//
// 撤销单条核销：把目标单据的已核销额减回去并删掉这一行，收付款单本身不动。
func (h *PaymentHandler) RemoveAllocation(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	allocationID, ok := parseUintParam(c, "allocationId", "核销")
	if !ok {
		return
	}
	m, err := h.payments.RemoveAllocation(c.Request.Context(), id, allocationID)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, m)
}

// Delete DELETE /api/v1/payments/:id
//
// 删除会把目标单据的已核销金额减回去，与核销在同一个事务里完成。
func (h *PaymentHandler) Delete(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	if err := h.payments.Delete(c.Request.Context(), id); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"deleted": true})
}
