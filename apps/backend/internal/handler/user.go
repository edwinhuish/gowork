package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/edwinhuish/gowork/internal/pkg/response"
	"github.com/edwinhuish/gowork/internal/service"
)

// UserHandler 是账号目录的只读入口（T-32 的"转给谁"选择器要用）。
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
