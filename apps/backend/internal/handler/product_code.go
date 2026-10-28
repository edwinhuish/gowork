package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/pkg/response"
	"github.com/edwinhuish/gowork/internal/service"
)

// ProductCodeHandler 管产品的别名编号（旧 SKU / 客户物料号 / 供应商货号）。
//
// 独立资源而不是嵌在 /products/:id 下面：前端的产品详情页拿它当一个面板，
// 与产品本身的读写互不干扰；编号也不需要跟着产品表单一起全量提交。
type ProductCodeHandler struct {
	codes service.ProductCodeService
}

func NewProductCodeHandler(codes service.ProductCodeService) *ProductCodeHandler {
	return &ProductCodeHandler{codes: codes}
}

// List GET /api/v1/product-codes?product_id=
func (h *ProductCodeHandler) List(c *gin.Context) {
	var q dto.ProductCodeQuery
	if !bindQuery(c, &q) {
		return
	}
	list, err := h.codes.List(c.Request.Context(), q)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, list)
}

// Create POST /api/v1/product-codes
func (h *ProductCodeHandler) Create(c *gin.Context) {
	var in dto.ProductCodeInput
	if !bindJSON(c, &in) {
		return
	}
	m, err := h.codes.Create(c.Request.Context(), in)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.Created(c, m)
}

// Delete DELETE /api/v1/product-codes/:id
func (h *ProductCodeHandler) Delete(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	if err := h.codes.Delete(c.Request.Context(), id); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"deleted": true})
}
