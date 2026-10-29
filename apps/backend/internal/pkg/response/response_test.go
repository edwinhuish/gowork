package response

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/edwinhuish/gowork/internal/pkg/errs"
)

// captureLog 把默认 logger 临时换成写内存的，返回日志缓冲与还原函数。
func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return &buf
}

func newContext(t *testing.T, method, path string) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(method, path, nil)
	c.Set(RequestIDKey, "req-test-1")
	return c, w
}

func decodeBody(t *testing.T, w *httptest.ResponseRecorder) Body {
	t.Helper()
	var body Body
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("响应不是合法 JSON: %v, raw=%s", err, w.Body.String())
	}
	return body
}

// 未知错误折叠成 500：响应体不能泄露内部细节，但日志必须带上真实原因。
// 这条用例守的是「表被改名 / 缺列 / 连接断开」这类只能靠日志定位的故障。
func TestFailLogsUnderlyingErrorForInternal(t *testing.T) {
	buf := captureLog(t)
	c, w := newContext(t, http.MethodGet, "/api/v1/dashboard/summary")

	Fail(c, errors.New("no such table: md_party"))

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("状态码 = %d, 期望 500", w.Code)
	}

	body := decodeBody(t, w)
	if body.Message != "服务器内部错误" {
		t.Errorf("message = %q, 期望统一文案", body.Message)
	}
	if body.RequestID != "req-test-1" {
		t.Errorf("request_id = %q, 期望回传 req-test-1", body.RequestID)
	}
	if got := w.Body.String(); bytes.Contains([]byte(got), []byte("md_party")) {
		t.Errorf("响应体泄露了内部错误细节: %s", got)
	}

	logged := buf.String()
	for _, want := range []string{"no such table: md_party", "/api/v1/dashboard/summary", "req-test-1", "level=ERROR"} {
		if !bytes.Contains([]byte(logged), []byte(want)) {
			t.Errorf("日志里缺少 %q，实际日志: %s", want, logged)
		}
	}
}

// errs.Wrap 产生的 5xx 同样要留底：它的 Error() 带着底层错误。
func TestFailLogsWrappedInternalError(t *testing.T) {
	buf := captureLog(t)
	c, w := newContext(t, http.MethodPost, "/api/v1/partners")

	Fail(c, errs.Wrap(errors.New("database is locked"), errs.CodeInternal, "保存失败"))

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("状态码 = %d, 期望 500", w.Code)
	}
	if body := decodeBody(t, w); body.Message != "保存失败" {
		t.Errorf("message = %q, 期望保留业务文案", body.Message)
	}
	if !bytes.Contains(buf.Bytes(), []byte("database is locked")) {
		t.Errorf("日志里缺少底层错误，实际日志: %s", buf.String())
	}
}

// 4xx 是用户输入问题，不是故障，不该刷进错误日志。
func TestFailKeepsClientErrorQuiet(t *testing.T) {
	buf := captureLog(t)
	c, w := newContext(t, http.MethodPost, "/api/v1/partners")

	Fail(c, errs.BadRequest("编码不能为空"))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("状态码 = %d, 期望 400", w.Code)
	}
	if body := decodeBody(t, w); body.Message != "编码不能为空" {
		t.Errorf("message = %q, 期望原样透出", body.Message)
	}
	if buf.Len() != 0 {
		t.Errorf("4xx 不应写错误日志，实际日志: %s", buf.String())
	}
}

// 请求超时不是"服务器内部错误"：代码没坏，只是这次没算完。
// 报 500 会让人去翻代码，而正确动作通常是重试或缩小查询范围。
func TestFailMapsDeadlineToGatewayTimeout(t *testing.T) {
	c, w := newContext(t, http.MethodGet, "/api/v1/reports/sales-dashboard")

	Fail(c, context.DeadlineExceeded)

	if w.Code != http.StatusGatewayTimeout {
		t.Fatalf("状态码 = %d, 期望 504", w.Code)
	}
	body := decodeBody(t, w)
	if body.Code != int(errs.CodeTimeout) {
		t.Errorf("业务码 = %d, 期望 %d", body.Code, errs.CodeTimeout)
	}
	if body.Message != errs.ErrTimeout.Msg {
		t.Errorf("message = %q, 期望 %q", body.Message, errs.ErrTimeout.Msg)
	}
}

// 上游把超时包成业务错误时，仍按超时报——否则一次超时会显示成"保存失败"。
func TestFailMapsWrappedDeadlineToGatewayTimeout(t *testing.T) {
	c, w := newContext(t, http.MethodGet, "/api/v1/reports/aging")

	Fail(c, errs.Wrap(context.DeadlineExceeded, errs.CodeInternal, "查询失败"))

	if w.Code != http.StatusGatewayTimeout {
		t.Fatalf("状态码 = %d, 期望 504", w.Code)
	}
}
