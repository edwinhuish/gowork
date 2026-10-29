package middleware

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/edwinhuish/gowork/internal/config"
	"github.com/edwinhuish/gowork/internal/infra/cache"
	"github.com/edwinhuish/gowork/internal/pkg/response"
	"github.com/edwinhuish/gowork/internal/service"
)

// idemFixture 装一台只有两条路由的引擎：一条会数自己被执行了几次。
type idemFixture struct {
	engine *gin.Engine
	calls  *int32
}

func newIdemFixture(t *testing.T, status int) idemFixture {
	t.Helper()
	gin.SetMode(gin.TestMode)

	c, err := cache.New(config.Cache{Driver: config.CacheMemory}, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatalf("建缓存: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	var calls int32
	engine := gin.New()
	engine.Use(func(gc *gin.Context) {
		// 直接放一个身份进去，省掉真鉴权：幂等键要用它做隔离维度。
		gc.Set(principalKey, &service.Principal{UserID: 7, Username: "tester"})
		gc.Next()
	}, Idempotency(c, slog.New(slog.DiscardHandler)))

	write := func(gc *gin.Context) {
		n := atomic.AddInt32(&calls, 1)
		response.OK(gc, gin.H{"seq": n})
	}
	engine.POST("/api/v1/things", write)
	if status >= 400 {
		engine.POST("/api/v1/boom", func(gc *gin.Context) {
			atomic.AddInt32(&calls, 1)
			gc.JSON(http.StatusInternalServerError, gin.H{"code": 50001})
		})
	}
	engine.GET("/api/v1/things", write)

	return idemFixture{engine: engine, calls: &calls}
}

func (f idemFixture) post(t *testing.T, path, key string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, nil)
	if key != "" {
		req.Header.Set(IdempotencyKeyHeader, key)
	}
	w := httptest.NewRecorder()
	f.engine.ServeHTTP(w, req)
	return w
}

// 同一个键打两次，第二次必须是回放：业务只执行一次，响应一模一样。
func TestIdempotencyReplaysSameKey(t *testing.T) {
	f := newIdemFixture(t, 200)

	first := f.post(t, "/api/v1/things", "abc-1")
	second := f.post(t, "/api/v1/things", "abc-1")

	if got := atomic.LoadInt32(f.calls); got != 1 {
		t.Fatalf("业务被执行了 %d 次，期望 1 次", got)
	}
	if first.Body.String() != second.Body.String() {
		t.Errorf("回放的响应体不一致:\n%s\n%s", first.Body.String(), second.Body.String())
	}
	if second.Header().Get(IdempotentReplayHeader) != "true" {
		t.Error("回放应当带上 Idempotent-Replay 头")
	}
	if first.Header().Get(IdempotentReplayHeader) != "" {
		t.Error("第一次执行不该被标成回放")
	}
}

// 不同的键是不同操作，互不影响。
func TestIdempotencyDifferentKeysExecute(t *testing.T) {
	f := newIdemFixture(t, 200)

	f.post(t, "/api/v1/things", "k1")
	f.post(t, "/api/v1/things", "k2")

	if got := atomic.LoadInt32(f.calls); got != 2 {
		t.Fatalf("不同键应执行 2 次，实际 %d 次", got)
	}
}

// 不带键就按普通请求处理：不是所有调用方都会生成键，不能因此拦住它们。
func TestIdempotencyWithoutKeyPassesThrough(t *testing.T) {
	f := newIdemFixture(t, 200)

	f.post(t, "/api/v1/things", "")
	f.post(t, "/api/v1/things", "")

	if got := atomic.LoadInt32(f.calls); got != 2 {
		t.Fatalf("无键时两次都应执行，实际 %d 次", got)
	}
}

// 5xx 不缓存：服务端临时故障时重试应当真的重试，而不是把错误答案钉住一天。
func TestIdempotencyDoesNotCacheServerError(t *testing.T) {
	f := newIdemFixture(t, 500)

	f.post(t, "/api/v1/boom", "boom-1")
	second := f.post(t, "/api/v1/boom", "boom-1")

	if got := atomic.LoadInt32(f.calls); got != 2 {
		t.Fatalf("5xx 之后重试应重新执行，实际执行 %d 次", got)
	}
	if second.Header().Get(IdempotentReplayHeader) != "" {
		t.Error("5xx 不该被回放")
	}
}

// GET 本来幂等，不进这套逻辑。
func TestIdempotencyIgnoresNonPost(t *testing.T) {
	f := newIdemFixture(t, 200)

	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/things", nil)
		req.Header.Set(IdempotencyKeyHeader, "get-1")
		w := httptest.NewRecorder()
		f.engine.ServeHTTP(w, req)
	}

	if got := atomic.LoadInt32(f.calls); got != 2 {
		t.Fatalf("GET 不该被合并，实际执行 %d 次", got)
	}
}
