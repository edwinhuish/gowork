package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/response"
	"github.com/edwinhuish/gowork/internal/service"
)

// PartnerAddressHandler 处理 /partners/:id/addresses 下的地址读写。
//
// 只挂在 /partners 下、不复制到 /customers 与 /suppliers：
// 地址本身没有角色语义，能打开某个主体就能看它的地址。
// 复制三份的代价是三处要同步改，而收益只是 URL 看起来整齐一点。
type PartnerAddressHandler struct {
	addresses service.PartnerAddressService
}

func NewPartnerAddressHandler(addresses service.PartnerAddressService) *PartnerAddressHandler {
	return &PartnerAddressHandler{addresses: addresses}
}

// List GET /partners/:id/addresses?type=invoice
func (h *PartnerAddressHandler) List(c *gin.Context) {
	partnerID, ok := parseID(c)
	if !ok {
		return
	}
	var q dto.PartnerAddressQuery
	if !bindQuery(c, &q) {
		return
	}
	rows, err := h.addresses.List(c.Request.Context(), partnerID, q)
	if err != nil {
		response.Fail(c, err)
		return
	}
	if rows == nil {
		// 空切片而不是 null：前端拿到 null 会多一层判空，而"没有地址"是正常状态。
		rows = []model.PartnerAddress{}
	}
	response.OK(c, rows)
}

// Create POST /partners/:id/addresses
func (h *PartnerAddressHandler) Create(c *gin.Context) {
	partnerID, ok := parseID(c)
	if !ok {
		return
	}
	var in dto.PartnerAddressInput
	if !bindJSON(c, &in) {
		return
	}
	m, err := h.addresses.Create(c.Request.Context(), partnerID, in)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.Created(c, m)
}

// Update PUT /partners/:id/addresses/:addressId
func (h *PartnerAddressHandler) Update(c *gin.Context) {
	partnerID, ok := parseID(c)
	if !ok {
		return
	}
	addressID, ok := parseUintParam(c, "addressId", "地址")
	if !ok {
		return
	}
	var in dto.PartnerAddressInput
	if !bindJSON(c, &in) {
		return
	}
	m, err := h.addresses.Update(c.Request.Context(), partnerID, addressID, in)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, m)
}

// Delete DELETE /partners/:id/addresses/:addressId
func (h *PartnerAddressHandler) Delete(c *gin.Context) {
	partnerID, ok := parseID(c)
	if !ok {
		return
	}
	addressID, ok := parseUintParam(c, "addressId", "地址")
	if !ok {
		return
	}
	if err := h.addresses.Delete(c.Request.Context(), partnerID, addressID); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, nil)
}
