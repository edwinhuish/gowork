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
	// credits 挂在主体上：信用额度是 A5 商务条件里的一个字段，
	// 读它的入口跟着主体走，不另开一个 /credit-limits 资源。
	credits service.CreditService
}

func NewPartnerHandler(partners service.PartnerService, credits service.CreditService) *PartnerHandler {
	return &PartnerHandler{partners: partners, credits: credits}
}

// Credit GET /partners/:id/credit
//
// 返回额度、未结清应收、未开票订单与敞口。下单页选中客户后调它做提示，
// 真正的拦截在订单确认时由 service 做——这里只负责显示。
func (h *PartnerHandler) Credit(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	res, err := h.credits.Summary(c.Request.Context(), id)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, res)
}

// Merge POST /partners/:id/merge
//
// 只挂在 /partners 下，不像增删改那样再复制到 /customers 与 /suppliers：
// 合并往往跨角色（客户并进"客供两用"的那条），从"客户"入口发起会让人以为
// 它只处理客户侧的数据。
func (h *PartnerHandler) Merge(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var in dto.PartnerMergeInput
	if !bindJSON(c, &in) {
		return
	}
	res, err := h.partners.Merge(c.Request.Context(), id, in)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, res)
}

// Duplicates GET /partner-duplicates
//
// 全库查重的分组结果。挂在 /partners 之外是有原因的：gin 的路由树不允许
// 同一层既有 :id 又有静态段，/partners/duplicates 会和 /partners/:id 冲突并 panic。
// 与 /shipment-candidates 是同一个处理方式。
func (h *PartnerHandler) Duplicates(c *gin.Context) {
	list, err := h.partners.Duplicates(c.Request.Context())
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, list)
}

// DuplicateCandidates GET /partner-duplicates/candidates
//
// 新建 / 编辑主体时随输入异步查一次，返回"可能是同一家"的已有主体与命中原因。
// 只提示不阻断：同名公司在不同国家确实存在，拦下来等于让人把询盘丢掉。
func (h *PartnerHandler) DuplicateCandidates(c *gin.Context) {
	var q dto.PartnerDuplicateQuery
	if !bindQuery(c, &q) {
		return
	}
	list, err := h.partners.DuplicateCandidates(c.Request.Context(), q)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, list)
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
// Restore POST /api/v1/partners/:id/restore
//
// 只挂在 /partners 下，不像增删改那样再复制一份到 /customers 与 /suppliers：
// 恢复只认 id，角色已经在记录里，多挂两个入口只会多出"从哪个入口恢复"的问题。
func (h *PartnerHandler) Restore(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	m, err := h.partners.Restore(c.Request.Context(), id)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, m)
}

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
