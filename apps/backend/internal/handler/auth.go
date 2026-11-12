package handler

import (
	"github.com/gin-gonic/gin"

	"gowork_server/internal/dto"
	"gowork_server/internal/pkg/errs"
	"gowork_server/internal/pkg/response"
	"gowork_server/internal/server/middleware"
	"gowork_server/internal/service"
)

type AuthHandler struct {
	auth service.AuthService
}

func NewAuthHandler(auth service.AuthService) *AuthHandler {
	return &AuthHandler{auth: auth}
}

func (h *AuthHandler) Login(c *gin.Context) {
	var req dto.LoginRequest
	if !bindJSON(c, &req) {
		return
	}

	res, err := h.auth.Login(c.Request.Context(), req)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, res)
}

func (h *AuthHandler) Logout(c *gin.Context) {
	p, ok := middleware.PrincipalFrom(c)
	if !ok {
		response.Fail(c, errs.ErrUnauthorized)
		return
	}
	if err := h.auth.Logout(c.Request.Context(), p.TokenID); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"logged_out": true})
}

func (h *AuthHandler) Profile(c *gin.Context) {
	p, ok := middleware.PrincipalFrom(c)
	if !ok {
		response.Fail(c, errs.ErrUnauthorized)
		return
	}
	response.OK(c, dto.UserInfo{
		ID:       p.UserID,
		Username: p.Username,
		Nickname: p.Nickname,
		Role:     p.Role,
	})
}
