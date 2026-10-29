package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/pkg/response"
	"github.com/edwinhuish/gowork/internal/service"
)

// ClauseHandler 是条款模板库的出口（T-27）。
//
// 它是**主数据**，所以挂在 /clauses 而不是 /contracts/clauses：
// 模板不属于任何一份合同，合同只在生成时抄它一份。
type ClauseHandler struct {
	clauses service.ClauseService
}

func NewClauseHandler(clauses service.ClauseService) *ClauseHandler {
	return &ClauseHandler{clauses: clauses}
}

// List GET /api/v1/clauses
func (h *ClauseHandler) List(c *gin.Context) {
	var q dto.ClauseQuery
	if !bindQuery(c, &q) {
		return
	}
	res, err := h.clauses.List(c.Request.Context(), q)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, res)
}

// Create POST /api/v1/clauses
func (h *ClauseHandler) Create(c *gin.Context) {
	var in dto.ClauseInput
	if !bindJSON(c, &in) {
		return
	}
	res, err := h.clauses.Create(c.Request.Context(), in)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, res)
}

// Update PUT /api/v1/clauses/:id
//
// 改模板不影响已经生成的合同：那些合同存的是快照。
func (h *ClauseHandler) Update(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var in dto.ClauseInput
	if !bindJSON(c, &in) {
		return
	}
	res, err := h.clauses.Update(c.Request.Context(), id, in)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, res)
}

// Delete DELETE /api/v1/clauses/:id
func (h *ClauseHandler) Delete(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	if err := h.clauses.Delete(c.Request.Context(), id); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"id": id})
}

// Restore POST /api/v1/clauses/:id/restore
func (h *ClauseHandler) Restore(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	res, err := h.clauses.Restore(c.Request.Context(), id)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, res)
}
