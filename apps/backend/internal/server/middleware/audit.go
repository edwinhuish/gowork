package middleware

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/pkg/response"
	"github.com/edwinhuish/gowork/internal/service"
)

// auditBodyLimit 是请求体落库前的截断长度（字符）。
//
// 4000 足够装下一张单据的表头与几行明细；再大就不该塞进日志表——
// 那种体量本身说明该去看单据，而不是看日志。
const auditBodyLimit = 4000

// auditSensitiveHints 是"整段值不落库"的键名片段。
//
// 按片段匹配而不是精确匹配：`new_password`、`oldPassword`、`jwt_token`
// 这些写法一定都会出现，漏一个就等于把密码抄进了数据库。
var auditSensitiveHints = []string{"password", "passwd", "token", "secret", "authorization", "credential"}

// Audit 把写操作落进审计表（T-33）。
//
// 为什么在中间件而不是每个 service 里各调一次：漏一处就等于那类单据没有留痕，
// 而"哪几类忘了记"在出事之前没人会去核对。放在这里，新增任何写接口自动被覆盖，
// 代价是只能看到请求层面的东西——所以它记的是**提交了什么**，不是数据库前后 diff。
// 真 diff 要在每个 service 里读旧值，成本高且必然漏；而"谁在什么时候对哪张单
// 提交了什么、结果如何"已经能回答留痕要回答的问题。
//
// 挂在鉴权之后：拿不到身份就没法回答"是谁"。登录接口不在这个组里，
// 密码因此永远不会进入日志。
func Audit(audit service.AuditService, log *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !isWriteMethod(c.Request.Method) {
			c.Next()
			return
		}

		// 请求体只能读一次：先取出来，再把 handler 要的那份放回去。
		body := auditRequestBody(c)
		c.Next()

		entry := dto.AuditEntry{
			Method:    c.Request.Method,
			Action:    auditAction(c),
			Entity:    auditEntity(c),
			EntityID:  auditEntityID(c),
			Path:      c.Request.URL.Path,
			Route:     c.FullPath(),
			Status:    c.Writer.Status(),
			RequestID: auditRequestID(c),
			IP:        c.ClientIP(),
			Body:      body,
		}
		if p, ok := PrincipalFrom(c); ok {
			entry.ActorID = p.UserID
			entry.ActorName = p.Username
			entry.ActorRole = p.Role
		}

		// 记不下来不能让业务失败——请求已经写进库并返回给用户了，
		// 此时回滚等于"操作成功却告诉用户失败"。改为在外层日志留一条 warn，
		// 让"审计缺了一条"这件事本身可见。
		if err := audit.Record(c.Request.Context(), entry); err != nil {
			log.Warn("审计记录未落库",
				"entity", entry.Entity, "action", entry.Action,
				"path", entry.Path, "status", entry.Status,
				"request_id", entry.RequestID, "err", err)
		}
	}
}

func isWriteMethod(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	}
	return false
}

// auditRequestBody 拿出请求体，脱敏后截断。
//
// multipart 一律不读：附件上传可能是几十兆，把它读进内存再存进日志表没有意义。
func auditRequestBody(c *gin.Context) string {
	if strings.HasPrefix(c.ContentType(), "multipart/") {
		return fmt.Sprintf("multipart/form-data（%d 字节，未落库）", c.Request.ContentLength)
	}
	if c.Request.Body == nil {
		return ""
	}

	raw, err := io.ReadAll(c.Request.Body)
	// 无论读成没读成，都要把 body 放回去，否则 handler 拿到的是一具空壳。
	c.Request.Body = io.NopCloser(bytes.NewReader(raw))
	if err != nil {
		return ""
	}
	if len(raw) == 0 {
		return ""
	}

	var payload any
	if err := json.Unmarshal(raw, &payload); err != nil {
		// 不是 JSON（例如纯文本备注），原样截断保留。
		return truncateAudit(string(raw))
	}
	scrubAuditValue(payload)
	out, err := json.Marshal(payload)
	if err != nil {
		return truncateAudit(string(raw))
	}
	return truncateAudit(string(out))
}

// scrubAuditValue 递归把敏感键的值替换掉。
func scrubAuditValue(value any) {
	switch typed := value.(type) {
	case map[string]any:
		for key, item := range typed {
			if isSensitiveKey(key) {
				typed[key] = "***"
				continue
			}
			scrubAuditValue(item)
		}
	case []any:
		for _, item := range typed {
			scrubAuditValue(item)
		}
	}
}

func isSensitiveKey(key string) bool {
	lower := strings.ToLower(key)
	for _, hint := range auditSensitiveHints {
		if strings.Contains(lower, hint) {
			return true
		}
	}
	return false
}

func truncateAudit(s string) string {
	runes := []rune(s)
	if len(runes) <= auditBodyLimit {
		return s
	}
	// 按字符截断而不是按字节：截在多字节字符中间会得到非法 UTF-8，
	// 存进库时被驱动替换成问号，读出来就是乱码。
	return string(runes[:auditBodyLimit]) + "…（已截断）"
}

// auditEntity 取资源名：`/api/v1/sales-orders/:id/status` → `sales-orders`。
func auditEntity(c *gin.Context) string {
	segments := auditSegments(c)
	if len(segments) == 0 {
		// 路由没匹配上（理论上不会走到这里），兜底成 unknown 而不是空串：
		// 空串在按资源筛选时等于人间蒸发。
		return "unknown"
	}
	return segments[0]
}

// auditAction 取业务动作：`:id` 之后那一段（status / merge / changes…），
// 没有就按方法归成 create / update / delete。
func auditAction(c *gin.Context) string {
	segments := auditSegments(c)
	// 资源之后的第一段不是 :id，说明它本身就是动作（如 stock-snapshots/recalc）。
	if len(segments) >= 2 {
		if segments[1] != ":id" {
			return segments[1]
		}
		if len(segments) >= 3 {
			return segments[2]
		}
	}

	switch c.Request.Method {
	case http.MethodPost:
		return "create"
	case http.MethodPut, http.MethodPatch:
		return "update"
	case http.MethodDelete:
		return "delete"
	}
	return strings.ToLower(c.Request.Method)
}

func auditEntityID(c *gin.Context) uint {
	raw := c.Param("id")
	if raw == "" {
		return 0
	}
	id, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		return 0
	}
	return uint(id)
}

// auditSegments 把路由模式拆成段，去掉 /api/v1 前缀。
func auditSegments(c *gin.Context) []string {
	full := strings.Trim(c.FullPath(), "/")
	if full == "" {
		return nil
	}
	full = strings.TrimPrefix(full, "api/v1/")
	return strings.Split(full, "/")
}

func auditRequestID(c *gin.Context) string {
	v, ok := c.Get(response.RequestIDKey)
	if !ok {
		return ""
	}
	rid, _ := v.(string)
	return rid
}
