package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/service"
)

// stubAuth 只实现中间件用得上的 Verify，其余方法留空。
type stubAuth struct {
	principal *service.Principal
	err       error
}

func (s stubAuth) Login(context.Context, dto.LoginRequest) (*dto.LoginResponse, error) {
	return nil, nil
}

func (s stubAuth) Logout(context.Context, string) error { return nil }

func (s stubAuth) Verify(context.Context, string) (*service.Principal, error) {
	return s.principal, s.err
}

func (s stubAuth) HashPassword(string) (string, error) { return "", nil }

func serveAuth(t *testing.T, p *service.Principal) *httptest.ResponseRecorder {
	t.Helper()

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(Auth(stubAuth{principal: p}))
	r.GET("/x", func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer some-token")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestAuthWritesRenewedTokenHeader(t *testing.T) {
	w := serveAuth(t, &service.Principal{UserID: 1, RenewedToken: "new-token"})

	if w.Code != http.StatusOK {
		t.Fatalf("状态码 = %d, 期望 200", w.Code)
	}
	if got := w.Header().Get(RenewedTokenHeader); got != "new-token" {
		t.Fatalf("%s = %q, 期望 new-token", RenewedTokenHeader, got)
	}
}

func TestAuthOmitsHeaderWhenNotRenewed(t *testing.T) {
	w := serveAuth(t, &service.Principal{UserID: 1})

	// 没有续期时不该留下空的同名响应头，否则前端会拿空串去覆盖本地令牌。
	if _, ok := w.Header()[RenewedTokenHeader]; ok {
		t.Fatalf("未续期却出现了 %s 响应头", RenewedTokenHeader)
	}
}

func TestCORSExposesRenewedTokenHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(CORS(nil))
	r.GET("/x", func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/x", nil))

	// 跨域下自定义响应头默认对 JS 不可见，漏了暴露会让滑动续期静默失效。
	exposed := w.Header().Get("Access-Control-Expose-Headers")
	if !strings.Contains(exposed, RenewedTokenHeader) {
		t.Fatalf("Access-Control-Expose-Headers = %q, 未包含 %s", exposed, RenewedTokenHeader)
	}
}
