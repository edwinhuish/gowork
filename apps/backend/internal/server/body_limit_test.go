package server

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/edwinhuish/gowork/internal/config"
	"github.com/edwinhuish/gowork/internal/infra/cache"
)

// 上限必须挂在**引擎**上，而不是某个路由上：漏挂一处就等于那类接口可以被打爆，
// 而"哪几处漏了"在被打之前没人会核对。
//
// 这里打的是登录接口——它不需要令牌，是最容易被匿名打爆的一个。
func TestEngineRejectsOversizedBody(t *testing.T) {
	c, err := cache.New(config.Cache{Driver: config.CacheMemory}, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatalf("建缓存: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	// 空 Handlers 就够：超限请求在中间件里就被拦下，不会走到 handler。
	engine := New(&config.Config{}, slog.New(slog.DiscardHandler), nil, nil, c, Handlers{})

	huge := strings.Repeat("a", int(bodyLimitBytes)+1)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(huge))
	req.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("超过上限的请求应 413，得到 %d（body=%s）", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "过大") {
		t.Errorf("提示应说明体积问题，得到 %s", w.Body.String())
	}
}

// 正常大小的请求不能被上限误伤。
func TestEngineAcceptsNormalBody(t *testing.T) {
	c, err := cache.New(config.Cache{Driver: config.CacheMemory}, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatalf("建缓存: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	engine := New(&config.Config{}, slog.New(slog.DiscardHandler), nil, nil, c, Handlers{})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login",
		strings.NewReader(`{"username":"admin","password":"admin123"}`))
	req.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(w, req)

	// 没有真 service 会 panic 吗？——这里只断言"没被 413 拦掉"，
	// 至于后续是 500 还是别的，不是本用例关心的事。
	if w.Code == http.StatusRequestEntityTooLarge {
		t.Fatalf("正常请求不该被体积上限拦下，得到 413")
	}
}
