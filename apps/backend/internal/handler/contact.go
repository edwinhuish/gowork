package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/pkg/response"
	"github.com/edwinhuish/gowork/internal/service"
)

type ContactHandler struct {
	contacts service.ContactService
}

func NewContactHandler(contacts service.ContactService) *ContactHandler {
	return &ContactHandler{contacts: contacts}
}

// List GET /api/v1/contacts?partner_id=12
func (h *ContactHandler) List(c *gin.Context) {
	var q dto.ContactQuery
	if !bindQuery(c, &q) {
		return
	}
	res, err := h.contacts.List(c.Request.Context(), q)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, res)
}

// Get GET /api/v1/contacts/:id
func (h *ContactHandler) Get(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	m, err := h.contacts.Get(c.Request.Context(), id)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, m)
}

// Create POST /api/v1/contacts
func (h *ContactHandler) Create(c *gin.Context) {
	var in dto.ContactInput
	if !bindJSON(c, &in) {
		return
	}
	m, err := h.contacts.Create(c.Request.Context(), in)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.Created(c, m)
}

// Update PUT /api/v1/contacts/:id
func (h *ContactHandler) Update(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var in dto.ContactInput
	if !bindJSON(c, &in) {
		return
	}
	m, err := h.contacts.Update(c.Request.Context(), id, in)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, m)
}

// Restore PUT /api/v1/contacts/:id/restore
func (h *ContactHandler) Restore(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	m, err := h.contacts.Restore(c.Request.Context(), id)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, m)
}

// Delete DELETE /api/v1/contacts/:id
func (h *ContactHandler) Delete(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	if err := h.contacts.Delete(c.Request.Context(), id); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"deleted": true})
}
