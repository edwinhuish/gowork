package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/errs"
	"github.com/edwinhuish/gowork/internal/pkg/response"
	"github.com/edwinhuish/gowork/internal/service"
)

// PartnerHandler 同时服务四个路由前缀：
//
//	/partners   统一主体，可跨角色操作
//	/customers  等价于只看客户角色，兼容改造前的页面与链接
//	/suppliers  等价于只看供应商角色
//	/forwarders 等价于只看货代角色
//
// 每个方法都接收一个 scope：非 nil 时，该入口只能看到、只能创建该角色的主体。
// 用参数而不是复制三套 handler，是因为逻辑完全一致，唯一的差别就是这个过滤。
//
// 为什么保留两个兼容入口而不是让前端直接改用 /partners：改造要能分批上线，
// 后端先合表、前端后改菜单，中间这段时间旧页面必须照常工作。
type PartnerHandler struct {
	partners service.PartnerService
}

func NewPartnerHandler(partners service.PartnerService) *PartnerHandler {
	return &PartnerHandler{partners: partners}
}

// List GET /partners | /customers | /suppliers
func (h *PartnerHandler) List(scope *model.PartnerRole) gin.HandlerFunc {
	return func(c *gin.Context) {
		var q dto.PartnerQuery
		if !bindQuery(c, &q) {
			return
		}
		// 只在调用方没显式指定角色时套用作用域：
		// /suppliers?role=1 这种组合没有意义，但也不该被静默改写，
		// 让它按字面意思返回空列表，比"看起来生效了其实没生效"更容易发现。
		if scope != nil && q.Role == nil {
			r := int8(*scope)
			q.Role = &r
		}

		res, err := h.partners.List(c.Request.Context(), q)
		if err != nil {
			response.Fail(c, err)
			return
		}
		response.OK(c, res)
	}
}

// Get GET /partners/:id | /customers/:id | /suppliers/:id
func (h *PartnerHandler) Get(scope *model.PartnerRole) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := parseID(c)
		if !ok {
			return
		}

		m, err := h.partners.Get(c.Request.Context(), id)
		if err != nil {
			response.Fail(c, err)
			return
		}
		// 作用域外的记录一律当作不存在。这不是权限判定，而是"这个入口只服务这个角色"：
		// 返回 404 而不是 403，避免通过状态码反推出某个 id 是不是客户。
		if scope != nil && !m.Role.Has(*scope) {
			response.Fail(c, errs.ErrNotFound)
			return
		}
		response.OK(c, m)
	}
}

// Create POST /partners | /customers | /suppliers
func (h *PartnerHandler) Create(scope *model.PartnerRole) gin.HandlerFunc {
	return func(c *gin.Context) {
		var in dto.PartnerInput
		if !bindJSON(c, &in) {
			return
		}
		if scope != nil && in.Role == nil {
			r := int8(*scope)
			in.Role = &r
		}

		m, err := h.partners.Create(c.Request.Context(), in)
		if err != nil {
			response.Fail(c, err)
			return
		}
		response.Created(c, m)
	}
}

// Update PUT /partners/:id | /customers/:id | /suppliers/:id
func (h *PartnerHandler) Update(scope *model.PartnerRole) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := parseID(c)
		if !ok {
			return
		}
		var in dto.PartnerInput
		if !bindJSON(c, &in) {
			return
		}

		if scope != nil {
			existing, err := h.partners.Get(c.Request.Context(), id)
			if err != nil {
				response.Fail(c, err)
				return
			}
			if !existing.Role.Has(*scope) {
				response.Fail(c, errs.ErrNotFound)
				return
			}
			// 从角色入口进来不允许改角色。否则在客户页面上编辑一次，
			// 就能把一个"客供两用"主体的供应商身份抹掉 —— 而那个页面上
			// 根本没有这个选项，用户不可能预期到这种副作用。
			in.Role = nil
		}

		m, err := h.partners.Update(c.Request.Context(), id, in)
		if err != nil {
			response.Fail(c, err)
			return
		}
		response.OK(c, m)
	}
}

// Delete DELETE /partners/:id | /customers/:id | /suppliers/:id
func (h *PartnerHandler) Delete(scope *model.PartnerRole) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := parseID(c)
		if !ok {
			return
		}

		if scope != nil {
			existing, err := h.partners.Get(c.Request.Context(), id)
			if err != nil {
				response.Fail(c, err)
				return
			}
			if !existing.Role.Has(*scope) {
				response.Fail(c, errs.ErrNotFound)
				return
			}
		}

		if err := h.partners.Delete(c.Request.Context(), id); err != nil {
			response.Fail(c, err)
			return
		}
		response.OK(c, gin.H{"deleted": true})
	}
}
