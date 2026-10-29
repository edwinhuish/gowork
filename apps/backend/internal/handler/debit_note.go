package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/pkg/response"
	"github.com/edwinhuish/gowork/internal/service"
)

// DebitNoteHandler 是借项通知单的入口（T-52b3）。
type DebitNoteHandler struct {
	notes service.DebitNoteService
}

func NewDebitNoteHandler(notes service.DebitNoteService) *DebitNoteHandler {
	return &DebitNoteHandler{notes: notes}
}

// List GET /api/v1/debit-notes?payable_id=&partner_id=
func (h *DebitNoteHandler) List(c *gin.Context) {
	var q dto.DebitNoteQuery
	if !bindQuery(c, &q) {
		return
	}
	res, err := h.notes.List(c.Request.Context(), q)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, res)
}

// Create POST /api/v1/debit-notes
func (h *DebitNoteHandler) Create(c *gin.Context) {
	var in dto.DebitNoteInput
	if !bindJSON(c, &in) {
		return
	}
	m, err := h.notes.Create(c.Request.Context(), in)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.Created(c, m)
}
