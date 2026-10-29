package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/pkg/response"
	"github.com/edwinhuish/gowork/internal/service"
)

type ContractHandler struct {
	contracts service.ContractService
}

func NewContractHandler(contracts service.ContractService) *ContractHandler {
	return &ContractHandler{contracts: contracts}
}

// List GET /api/v1/contracts
func (h *ContractHandler) List(c *gin.Context) {
	var q dto.ContractQuery
	if !bindQuery(c, &q) {
		return
	}
	res, err := h.contracts.List(c.Request.Context(), q)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, res)
}

// Get GET /api/v1/contracts/:id
func (h *ContractHandler) Get(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	m, err := h.contracts.Get(c.Request.Context(), id)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, m)
}

// Generate POST /api/v1/contracts
//
// 创建合同的唯一入口就是"从来源单据生成"：合同不存货物明细，
// 没有来源单据时金额与客户都无从填写，放开空白新建只会造出对不上的单子。
func (h *ContractHandler) Generate(c *gin.Context) {
	var in dto.ContractGenerateInput
	if !bindJSON(c, &in) {
		return
	}
	m, err := h.contracts.Generate(c.Request.Context(), in)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.Created(c, m)
}

// Update PUT /api/v1/contracts/:id
func (h *ContractHandler) Update(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var in dto.ContractUpdateInput
	if !bindJSON(c, &in) {
		return
	}
	m, err := h.contracts.Update(c.Request.Context(), id, in)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, m)
}

// Delete DELETE /api/v1/contracts/:id
func (h *ContractHandler) Delete(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	if err := h.contracts.Delete(c.Request.Context(), id); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"deleted": true})
}

// UpdateStatus PUT /api/v1/contracts/:id/status
//
// 状态流转单独一个接口而不是塞进 PUT：编辑条款与把合同变成承诺是两件事，
// 混在一起之后"保存一下"顺手就把合同确认了。
func (h *ContractHandler) UpdateStatus(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var in dto.ContractStatusInput
	if !bindJSON(c, &in) {
		return
	}
	m, err := h.contracts.UpdateStatus(c.Request.Context(), id, in)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, m)
}

// InvoiceTerm POST /api/v1/contracts/:id/terms/:termId/invoice
//
// 给合同某一期开票（T-61）：生成一张应收并回写该期，重复开返回 409。
func (h *ContractHandler) InvoiceTerm(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	termID, ok := parseUintParam(c, "termId", "分期")
	if !ok {
		return
	}
	r, err := h.contracts.InvoiceTerm(c.Request.Context(), id, termID)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.Created(c, r)
}
