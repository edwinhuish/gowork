package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestRequestTimeoutSetsDeadline(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var hasDeadline bool
	var err error

	e := gin.New()
	e.Use(RequestTimeout(50 * time.Millisecond))
	e.GET("/x", func(c *gin.Context) {
		_, hasDeadline = c.Request.Context().Deadline()
		// 模拟一份慢查询：ctx 到点后立即返回，模拟仓储层对取消的反应。
		<-c.Request.Context().Done()
		err = c.Request.Context().Err()
	})

	w := httptest.NewRecorder()
	e.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/x", nil))

	if !hasDeadline {
		t.Fatal("handler 里的 ctx 应带截止时间")
	}
	if err != context.DeadlineExceeded {
		t.Errorf("到点后 ctx 应以 DeadlineExceeded 结束，得到 %v", err)
	}
}

// d <= 0 表示不设限：某些部署想让长任务跑完。
func TestRequestTimeoutDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var hasDeadline bool

	e := gin.New()
	e.Use(RequestTimeout(0))
	e.GET("/x", func(c *gin.Context) {
		_, hasDeadline = c.Request.Context().Deadline()
	})

	e.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/x", nil))
	if hasDeadline {
		t.Error("d <= 0 时不该给 ctx 加截止时间")
	}
}
