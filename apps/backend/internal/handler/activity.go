package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/pkg/response"
	"github.com/edwinhuish/gowork/internal/service"
)

type ActivityHandler struct {
	activities service.ActivityService
}

func NewActivityHandler(activities service.ActivityService) *ActivityHandler {
	return &ActivityHandler{activities: activities}
}

// List GET /api/v1/activities?partner_id=12&due=1
func (h *ActivityHandler) List(c *gin.Context) {
	var q dto.ActivityQuery
	if !bindQuery(c, &q) {
		return
	}
	res, err := h.activities.List(c.Request.Context(), q)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, res)
}

// Get GET /api/v1/activities/:id
func (h *ActivityHandler) Get(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	m, err := h.activities.Get(c.Request.Context(), id)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, m)
}

// Create POST /api/v1/activities
func (h *ActivityHandler) Create(c *gin.Context) {
	var in dto.ActivityInput
	if !bindJSON(c, &in) {
		return
	}
	m, err := h.activities.Create(c.Request.Context(), in)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.Created(c, m)
}

// Update PUT /api/v1/activities/:id
func (h *ActivityHandler) Update(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var in dto.ActivityInput
	if !bindJSON(c, &in) {
		return
	}
	m, err := h.activities.Update(c.Request.Context(), id, in)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, m)
}

// Delete DELETE /api/v1/activities/:id
func (h *ActivityHandler) Delete(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	if err := h.activities.Delete(c.Request.Context(), id); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"deleted": true})
}
