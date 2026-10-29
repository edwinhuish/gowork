package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/pkg/response"
	"github.com/edwinhuish/gowork/internal/service"
)

// UserHandler 是账号目录与账号维护入口（T-32 的选择器 / T-57 的维护页）。
//
// 不叫 AccountHandler：本项目里 User 就是"登录系统的操作者"，
// 与往来单位上的 Contact 是两回事（见术语表）。
type UserHandler struct {
	users service.UserService
}

func NewUserHandler(users service.UserService) *UserHandler {
	return &UserHandler{users: users}
}

// ListActive GET /api/v1/users
//
// 返回启用中的账号（id / username / nickname / role / status），密码字段从来不出库。
// 业务员调用返回 403：他既不能转移客户，也不需要知道公司有谁。
func (h *UserHandler) ListActive(c *gin.Context) {
	res, err := h.users.ListActive(c.Request.Context())
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, res)
}

// List GET /api/v1/user-accounts
//
// 与 /users 分开的理由不是命名好看：gin 同一层不允许静态段与 :id 并存
// （/users/:id 会被已注册的 /users 挡住），与 /partner-duplicates 同一处理。
// 两者语义也不同：/users 是"能派给谁"（只有启用的），这里是"公司里有哪些账号"。
func (h *UserHandler) List(c *gin.Context) {
	res, err := h.users.List(c.Request.Context())
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, res)
}

// Create POST /api/v1/user-accounts
func (h *UserHandler) Create(c *gin.Context) {
	var in dto.UserInput
	if !bindJSON(c, &in) {
		return
	}
	m, err := h.users.Create(c.Request.Context(), in)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.Created(c, m)
}

// Update PUT /api/v1/user-accounts/:id
//
// 三组字段都可选：昵称、角色与启用状态、密码。改了角色 / 状态 / 密码之后，
// 该账号已签发的令牌会立即失效，他下一次请求会拿到 401 并被要求重新登录。
func (h *UserHandler) Update(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var in dto.UserUpdateInput
	if !bindJSON(c, &in) {
		return
	}
	m, err := h.users.Update(c.Request.Context(), id, in)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, m)
}
