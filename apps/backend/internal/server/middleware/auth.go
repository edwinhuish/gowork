// Package middleware 提供 HTTP 中间件。
package middleware

import (
	"github.com/gin-gonic/gin"

	"github.com/edwinhuish/gowork/internal/pkg/errs"
	"github.com/edwinhuish/gowork/internal/pkg/response"
	"github.com/edwinhuish/gowork/internal/service"
)

const principalKey = "principal"

// RenewedTokenHeader 承载滑动续期后的新令牌。
//
// 走响应头而不是塞进响应体：{code,message,data} 是全局统一契约，
// 为一个横切的副作用去改每个接口的 data 结构不划算。
const RenewedTokenHeader = "X-Renewed-Token"

// WithPrincipal 把当前登录身份写入请求上下文。
func WithPrincipal(c *gin.Context, p *service.Principal) {
	c.Set(principalKey, p)
}

// PrincipalFrom 取出当前登录身份。
func PrincipalFrom(c *gin.Context) (*service.Principal, bool) {
	v, ok := c.Get(principalKey)
	if !ok {
		return nil, false
	}
	p, ok := v.(*service.Principal)
	return p, ok
}

// Auth 校验 Bearer 令牌，并把身份注入上下文。
func Auth(auth service.AuthService) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := extractToken(c)
		if token == "" {
			response.Fail(c, errs.ErrUnauthorized)
			return
		}

		p, err := auth.Verify(c.Request.Context(), token)
		if err != nil {
			response.Fail(c, err)
			return
		}

		if p.RenewedToken != "" {
			c.Header(RenewedTokenHeader, p.RenewedToken)
		}
		WithPrincipal(c, p)
		c.Next()
	}
}

// RequireAdmin 在 Auth 之后使用，限制仅管理员可访问。
func RequireAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		p, ok := PrincipalFrom(c)
		if !ok {
			response.Fail(c, errs.ErrUnauthorized)
			return
		}
		if !p.IsAdmin() {
			response.Fail(c, errs.ErrForbidden)
			return
		}
		c.Next()
	}
}

func extractToken(c *gin.Context) string {
	const prefix = "Bearer "
	v := c.GetHeader("Authorization")
	if len(v) <= len(prefix) || v[:len(prefix)] != prefix {
		return ""
	}
	return v[len(prefix):]
}
