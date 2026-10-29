package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/pkg/response"
	"github.com/edwinhuish/gowork/internal/service"
)

// CreditNoteHandler 是贷项通知单的入口（T-52b）。
type CreditNoteHandler struct {
	notes service.CreditNoteService
}

func NewCreditNoteHandler(notes service.CreditNoteService) *CreditNoteHandler {
	return &CreditNoteHandler{notes: notes}
}

// List GET /api/v1/credit-notes?receivable_id=&partner_id=
func (h *CreditNoteHandler) List(c *gin.Context) {
	var q dto.CreditNoteQuery
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

// Create POST /api/v1/credit-notes
func (h *CreditNoteHandler) Create(c *gin.Context) {
	var in dto.CreditNoteInput
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
