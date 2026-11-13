package middleware

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// Content-Length 就超了：不必把数据读进来，直接 413。
func TestBodyLimitRejectsByContentLength(t *testing.T) {
	gin.SetMode(gin.TestMode)
	reached := false
	e := gin.New()
	e.Use(BodyLimit(16))
	e.POST("/x", func(c *gin.Context) {
		reached = true
		c.String(http.StatusOK, "ok")
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(strings.Repeat("a", 100)))
	e.ServeHTTP(w, req)

	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("应为 413，得到 %d（body=%s）", w.Code, w.Body.String())
	}
	if reached {
		t.Error("超限请求不该进入 handler——那意味着数据已经被读进来了")
	}
	if !strings.Contains(w.Body.String(), "请求体过大") {
		t.Errorf("提示应说明是体积问题，得到 %s", w.Body.String())
	}
}

// 没有 Content-Length（chunked）时只能靠读的时候拦：读错误要能被识别成"太大"。
func TestBodyLimitCatchesChunkedOverflow(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var detected bool
	e := gin.New()
	e.Use(BodyLimit(16))
	e.POST("/x", func(c *gin.Context) {
		buf := make([]byte, 64)
		_, err := c.Request.Body.Read(buf)
		detected = HandleBodyTooLarge(c, err)
	})

	req := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(strings.Repeat("a", 100)))
	req.ContentLength = -1 // 模拟 chunked
	w := httptest.NewRecorder()
	e.ServeHTTP(w, req)

	if !detected {
		t.Error("chunked 超限时读错误应被识别为 MaxBytesError")
	}
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("应写出 413，得到 %d", w.Code)
	}
}

func TestBodyLimitAllowsNormalRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	e := gin.New()
	e.Use(BodyLimit(1024))
	e.POST("/x", func(c *gin.Context) {
		var body map[string]string
		if err := c.ShouldBindJSON(&body); err != nil {
			c.String(http.StatusBadRequest, err.Error())
			return
		}
		c.String(http.StatusOK, body["k"])
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(`{"k":"v"}`))
	req.Header.Set("Content-Type", "application/json")
	e.ServeHTTP(w, req)

	if w.Code != http.StatusOK || w.Body.String() != "v" {
		t.Fatalf("正常请求应通过，得到 %d / %s", w.Code, w.Body.String())
	}
}

// 不是"太大"的错误不能被误判成 413，否则所有解析错误都会变成体积提示。
func TestHandleBodyTooLargeIgnoresOtherErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	if HandleBodyTooLarge(c, errors.New("unexpected end of JSON input")) {
		t.Error("普通解析错误不该被当成体积超限")
	}
	if w.Code != http.StatusOK {
		t.Errorf("不该写出响应，得到 %d", w.Code)
	}
}
