package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/pkg/response"
	"github.com/edwinhuish/gowork/internal/service"
)

// PurchaseReceiptHandler 是采购收货单的入口（T-51a）。
//
// 两个前缀：建单与列表挂在采购单下（`/purchase-orders/:id/receipts`，
// 因为收货永远属于某张采购单），确认与删除挂在收货单自己的前缀下
// （那时已经只需 id，不必再带上采购单）。
type PurchaseReceiptHandler struct {
	receipts service.PurchaseReceiptService
}

func NewPurchaseReceiptHandler(receipts service.PurchaseReceiptService) *PurchaseReceiptHandler {
	return &PurchaseReceiptHandler{receipts: receipts}
}

// Summary GET /api/v1/purchase-orders/:id/receipts
//
// 返回收货单列表与每行已收量；在途量由前端用「订购 − 已收」算，
// 口径只有一份（后端给的已收量）。
func (h *PurchaseReceiptHandler) Summary(c *gin.Context) {
	orderID, ok := parseID(c)
	if !ok {
		return
	}
	res, err := h.receipts.Summary(c.Request.Context(), orderID)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, res)
}

// Create POST /api/v1/purchase-orders/:id/receipts
func (h *PurchaseReceiptHandler) Create(c *gin.Context) {
	orderID, ok := parseID(c)
	if !ok {
		return
	}
	var in dto.PurchaseReceiptInput
	if !bindJSON(c, &in) {
		return
	}
	m, err := h.receipts.Create(c.Request.Context(), orderID, in)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.Created(c, m)
}

// Get GET /api/v1/purchase-receipts/:id
func (h *PurchaseReceiptHandler) Get(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	m, err := h.receipts.Get(c.Request.Context(), id)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, m)
}

// Confirm PUT /api/v1/purchase-receipts/:id/confirm
//
// 确认会生成并确认一张采购入库单（同一事务）。重复确认返回 409。
func (h *PurchaseReceiptHandler) Confirm(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	m, err := h.receipts.Confirm(c.Request.Context(), id)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, m)
}

// Delete DELETE /api/v1/purchase-receipts/:id
func (h *PurchaseReceiptHandler) Delete(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	if err := h.receipts.Delete(c.Request.Context(), id); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, nil)
}
