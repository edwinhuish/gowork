package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/pkg/response"
	"github.com/edwinhuish/gowork/internal/service"
)

// StockHandler 暴露库存流水与库存快照。
//
// 快照只有读与"重算"两个入口，没有任何写接口：能改快照的只有流水。
type StockHandler struct {
	stock service.StockService
}

func NewStockHandler(stock service.StockService) *StockHandler {
	return &StockHandler{stock: stock}
}

// ListMovements GET /api/v1/stock-movements
func (h *StockHandler) ListMovements(c *gin.Context) {
	var q dto.StockMovementQuery
	if !bindQuery(c, &q) {
		return
	}
	res, err := h.stock.ListMovements(c.Request.Context(), q)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, res)
}

// GetMovement GET /api/v1/stock-movements/:id
func (h *StockHandler) GetMovement(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	m, err := h.stock.GetMovement(c.Request.Context(), id)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, m)
}

// CreateMovement POST /api/v1/stock-movements
func (h *StockHandler) CreateMovement(c *gin.Context) {
	var in dto.StockMovementInput
	if !bindJSON(c, &in) {
		return
	}
	m, err := h.stock.CreateMovement(c.Request.Context(), in)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.Created(c, m)
}

// DeleteMovement DELETE /api/v1/stock-movements/:id
func (h *StockHandler) DeleteMovement(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	if err := h.stock.DeleteMovement(c.Request.Context(), id); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"deleted": true})
}

// ListSnapshots GET /api/v1/stock-snapshots
func (h *StockHandler) ListSnapshots(c *gin.Context) {
	var q dto.StockSnapshotQuery
	if !bindQuery(c, &q) {
		return
	}
	res, err := h.stock.ListSnapshots(c.Request.Context(), q)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, res)
}

// Recalc POST /api/v1/stock-snapshots/recalc
//
// 用 POST 而不是 GET：它会写库（覆盖快照）。
// 路径带着 snapshots 前缀也比 /stock-recalc 更贴着语义。
func (h *StockHandler) Recalc(c *gin.Context) {
	var in dto.StockRecalcInput
	// 允许空 body：不传 product_id 就是全量重算。
	if c.Request.ContentLength > 0 {
		if !bindJSON(c, &in) {
			return
		}
	}
	res, err := h.stock.Recalc(c.Request.Context(), in.ProductID)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, res)
}
