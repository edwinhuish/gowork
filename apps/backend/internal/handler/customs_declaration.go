package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/pkg/response"
	"github.com/edwinhuish/gowork/internal/service"
)

type CustomsDeclarationHandler struct {
	declarations service.CustomsDeclarationService
}

func NewCustomsDeclarationHandler(
	declarations service.CustomsDeclarationService,
) *CustomsDeclarationHandler {
	return &CustomsDeclarationHandler{declarations: declarations}
}

// List GET /api/v1/customs-declarations?shipment_id=1
func (h *CustomsDeclarationHandler) List(c *gin.Context) {
	var q dto.CustomsDeclarationQuery
	if !bindQuery(c, &q) {
		return
	}
	res, err := h.declarations.List(c.Request.Context(), q)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, res)
}

// Get GET /api/v1/customs-declarations/:id
func (h *CustomsDeclarationHandler) Get(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	m, err := h.declarations.Get(c.Request.Context(), id)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, m)
}

// Create POST /api/v1/customs-declarations
func (h *CustomsDeclarationHandler) Create(c *gin.Context) {
	var in dto.CustomsDeclarationInput
	if !bindJSON(c, &in) {
		return
	}
	m, err := h.declarations.Create(c.Request.Context(), in)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.Created(c, m)
}

// Update PUT /api/v1/customs-declarations/:id
func (h *CustomsDeclarationHandler) Update(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var in dto.CustomsDeclarationInput
	if !bindJSON(c, &in) {
		return
	}
	m, err := h.declarations.Update(c.Request.Context(), id, in)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, m)
}

// Delete DELETE /api/v1/customs-declarations/:id
func (h *CustomsDeclarationHandler) Delete(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	if err := h.declarations.Delete(c.Request.Context(), id); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"deleted": true})
}
