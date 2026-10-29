package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/service"
)

// stubAudit 把中间件交来的记录留在内存里，供断言。
type stubAudit struct {
	entries []dto.AuditEntry
	err     error
}

func (s *stubAudit) Record(_ context.Context, entry dto.AuditEntry) error {
	s.entries = append(s.entries, entry)
	return s.err
}

func (s *stubAudit) List(
	context.Context, dto.AuditLogQuery,
) (*dto.PageResult[model.AuditLog], error) {
	return nil, nil
}

// serveAudited 起一个只挂审计中间件的最小引擎：路由模式与真实路由同名，
// 因为 entity / action 都是从 c.FullPath() 里拆出来的。
func serveAudited(
	t *testing.T, audit *stubAudit, method, path, contentType, body string,
	register func(r *gin.Engine),
) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.Use(func(c *gin.Context) {
		// 真实链路里由 Auth 写入；这里直接放一个身份。
		WithPrincipal(c, &service.Principal{UserID: 7, Username: "edwin", Role: "admin"})
		c.Next()
	})
	r.Use(Audit(audit, slog.New(slog.DiscardHandler)))
	register(r)

	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestAuditRecordsWriteWithScrubbedBody(t *testing.T) {
	audit := &stubAudit{}
	serveAudited(t, audit, http.MethodPost, "/api/v1/sales-orders", "application/json",
		`{"partner_id":3,"items":[{"qty":"100"}],"password":"hunter2","remark":"加急"}`,
		func(r *gin.Engine) {
			r.POST("/api/v1/sales-orders", func(c *gin.Context) {
				// handler 仍然要能读到请求体：中间件读完必须放回去。
				var in map[string]any
				if err := c.ShouldBindJSON(&in); err != nil {
					c.String(http.StatusBadRequest, "bind failed")
					return
				}
				c.String(http.StatusCreated, "ok")
			})
		})

	if len(audit.entries) != 1 {
		t.Fatalf("应记录 1 条，得到 %d", len(audit.entries))
	}
	e := audit.entries[0]
	if e.Entity != "sales-orders" || e.Action != "create" || e.EntityID != 0 {
		t.Errorf("实体/动作不对: %+v", e)
	}
	if e.ActorName != "edwin" || e.ActorID != 7 || e.ActorRole != "admin" {
		t.Errorf("操作人不对: %+v", e)
	}
	if e.Status != http.StatusCreated {
		t.Errorf("状态码应为 201，得到 %d", e.Status)
	}
	if strings.Contains(e.Body, "hunter2") {
		t.Errorf("密码不该落库: %s", e.Body)
	}
	if !strings.Contains(e.Body, "***") || !strings.Contains(e.Body, "加急") {
		t.Errorf("脱敏后应保留其余字段: %s", e.Body)
	}
}

func TestAuditActionFromRouteSuffix(t *testing.T) {
	audit := &stubAudit{}
	serveAudited(t, audit, http.MethodPut, "/api/v1/sales-orders/42/status", "application/json",
		`{"status":20}`, func(r *gin.Engine) {
			r.PUT("/api/v1/sales-orders/:id/status", func(c *gin.Context) {
				c.String(http.StatusOK, "ok")
			})
		})

	if len(audit.entries) != 1 {
		t.Fatalf("应记录 1 条，得到 %d", len(audit.entries))
	}
	e := audit.entries[0]
	// 状态流转要记成 status 而不是笼统的 update：翻日志的人找的就是这个动作。
	if e.Action != "status" || e.EntityID != 42 || e.Entity != "sales-orders" {
		t.Errorf("动作/实体不对: %+v", e)
	}
}

func TestAuditRecordsFailedWrite(t *testing.T) {
	audit := &stubAudit{}
	serveAudited(t, audit, http.MethodDelete, "/api/v1/partners/9", "", "",
		func(r *gin.Engine) {
			r.DELETE("/api/v1/partners/:id", func(c *gin.Context) {
				c.String(http.StatusConflict, "conflict")
			})
		})

	if len(audit.entries) != 1 {
		t.Fatalf("失败的操作同样是发生过的事，应记录 1 条，得到 %d", len(audit.entries))
	}
	if audit.entries[0].Status != http.StatusConflict {
		t.Errorf("应记下 409，得到 %d", audit.entries[0].Status)
	}
	if audit.entries[0].Action != "delete" {
		t.Errorf("动作应为 delete，得到 %q", audit.entries[0].Action)
	}
}

func TestAuditSkipsReads(t *testing.T) {
	audit := &stubAudit{}
	serveAudited(t, audit, http.MethodGet, "/api/v1/sales-orders", "", "",
		func(r *gin.Engine) {
			r.GET("/api/v1/sales-orders", func(c *gin.Context) { c.String(http.StatusOK, "ok") })
		})

	if len(audit.entries) != 0 {
		t.Fatalf("读操作不该进审计表，得到 %d 条", len(audit.entries))
	}
}

func TestAuditDoesNotReadMultipartBody(t *testing.T) {
	audit := &stubAudit{}
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("file", "license.pdf")
	if err != nil {
		t.Fatalf("构造 multipart 失败: %v", err)
	}
	if _, err := fw.Write([]byte("%PDF-1.7 fake")); err != nil {
		t.Fatalf("写 multipart 失败: %v", err)
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("收尾 multipart 失败: %v", err)
	}

	serveAudited(t, audit, http.MethodPost, "/api/v1/attachments", mw.FormDataContentType(),
		buf.String(), func(r *gin.Engine) {
			r.POST("/api/v1/attachments", func(c *gin.Context) {
				// 附件必须仍能被 handler 解析出来：中间件不能把 body 吃掉。
				if _, err := c.FormFile("file"); err != nil {
					c.String(http.StatusBadRequest, "lost file")
					return
				}
				c.String(http.StatusOK, "ok")
			})
		})

	if len(audit.entries) != 1 {
		t.Fatalf("应记录 1 条，得到 %d", len(audit.entries))
	}
	if !strings.Contains(audit.entries[0].Body, "multipart/form-data") {
		t.Errorf("multipart 应记成摘要而不是内容，得到 %q", audit.entries[0].Body)
	}
}

func TestAuditRecordFailureDoesNotBreakRequest(t *testing.T) {
	audit := &stubAudit{err: context.DeadlineExceeded}
	w := serveAudited(t, audit, http.MethodPost, "/api/v1/products", "application/json", `{"a":1}`,
		func(r *gin.Engine) {
			r.POST("/api/v1/products", func(c *gin.Context) { c.String(http.StatusOK, "ok") })
		})

	if w.Code != http.StatusOK {
		t.Fatalf("审计写失败不该影响业务响应，得到 %d", w.Code)
	}
}

// 请求体过大时按字符截断：截在多字节字符中间会写出非法 UTF-8，读出来就是乱码。
func TestAuditTruncatesLongBody(t *testing.T) {
	audit := &stubAudit{}
	long := strings.Repeat("合", auditBodyLimit+50)
	body, err := json.Marshal(map[string]string{"remark": long})
	if err != nil {
		t.Fatalf("构造请求体失败: %v", err)
	}
	serveAudited(t, audit, http.MethodPost, "/api/v1/products", "application/json", string(body),
		func(r *gin.Engine) {
			r.POST("/api/v1/products", func(c *gin.Context) { c.String(http.StatusOK, "ok") })
		})

	got := audit.entries[0].Body
	if !strings.HasSuffix(got, "（已截断）") {
		t.Fatalf("应标出已截断，实际结尾 %q", got[len(got)-12:])
	}
	if strings.ContainsRune(got, '\uFFFD') {
		t.Error("截断产生了非法 UTF-8（出现了替换字符）")
	}
}
