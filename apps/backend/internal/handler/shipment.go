package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/pkg/errs"
	"github.com/edwinhuish/gowork/internal/pkg/response"
	"github.com/edwinhuish/gowork/internal/service"
)

type ShipmentHandler struct {
	shipments service.ShipmentService
}

func NewShipmentHandler(shipments service.ShipmentService) *ShipmentHandler {
	return &ShipmentHandler{shipments: shipments}
}

// List GET /api/v1/shipments
func (h *ShipmentHandler) List(c *gin.Context) {
	var q dto.ShipmentQuery
	if !bindQuery(c, &q) {
		return
	}
	res, err := h.shipments.List(c.Request.Context(), q)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, res)
}

// Get GET /api/v1/shipments/:id
func (h *ShipmentHandler) Get(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	m, err := h.shipments.Get(c.Request.Context(), id)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, m)
}

// Create POST /api/v1/shipments
func (h *ShipmentHandler) Create(c *gin.Context) {
	var in dto.ShipmentInput
	if !bindJSON(c, &in) {
		return
	}
	m, err := h.shipments.Create(c.Request.Context(), in)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.Created(c, m)
}

// Update PUT /api/v1/shipments/:id
func (h *ShipmentHandler) Update(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var in dto.ShipmentInput
	if !bindJSON(c, &in) {
		return
	}
	m, err := h.shipments.Update(c.Request.Context(), id, in)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, m)
}

// Delete DELETE /api/v1/shipments/:id
func (h *ShipmentHandler) Delete(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	if err := h.shipments.Delete(c.Request.Context(), id); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"deleted": true})
}

// Candidates GET /api/v1/shipment-candidates
//
// 独立前缀而不是 /shipments/candidates：后者会被 /shipments/:id 抢走，
// 表现是"新建出货单选行时永远 400 无效的资源 ID"。
func (h *ShipmentHandler) Candidates(c *gin.Context) {
	// exclude_shipment_id 只在编辑页传：把这一票自己占的数量还给自己。
	var exclude uint
	if raw := c.Query("exclude_shipment_id"); raw != "" {
		id, err := strconv.ParseUint(raw, 10, 64)
		if err != nil {
			response.Fail(c, errs.BadRequest("exclude_shipment_id 不合法"))
			return
		}
		exclude = uint(id)
	}
	list, err := h.shipments.Candidates(c.Request.Context(), c.Query("keyword"), exclude)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, list)
}

// UpdateStatus PUT /api/v1/shipments/:id/status
func (h *ShipmentHandler) UpdateStatus(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var in dto.ShipmentStatusInput
	if !bindJSON(c, &in) {
		return
	}
	m, err := h.shipments.UpdateStatus(c.Request.Context(), id, in)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, m)
}

// Reissue POST /api/v1/shipments/:id/reissue
//
// 改证重开：号码不变、版本加一。没有入参——版本号是服务端算的，
// 让前端传"新版本"就等于允许跳版或倒退。
func (h *ShipmentHandler) Reissue(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	m, err := h.shipments.Reissue(c.Request.Context(), id)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, m)
}

// UpdateLogistics PUT /api/v1/shipments/:id/logistics
//
// 独立入口：提单号与船期是订舱之后才知道的，那时出货单早已确认、不可再改。
func (h *ShipmentHandler) UpdateLogistics(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var in dto.ShipmentLogisticsInput
	if !bindJSON(c, &in) {
		return
	}
	m, err := h.shipments.UpdateLogistics(c.Request.Context(), id, in)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, m)
}
