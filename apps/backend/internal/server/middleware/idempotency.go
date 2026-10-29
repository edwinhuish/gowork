package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/edwinhuish/gowork/internal/infra/cache"
	"github.com/edwinhuish/gowork/internal/pkg/errs"
	"github.com/edwinhuish/gowork/internal/pkg/response"
)

// IdempotencyKeyHeader 是客户端给"这一次写操作"分配的唯一键。
//
// 只在请求带上它时生效，不强制：生成一个 UUID 是客户端的责任，
// 服务端凭空编一个就等于每次请求都是新操作，幂等无从谈起。
const IdempotencyKeyHeader = "Idempotency-Key"

// IdempotentReplayHeader 标记这次响应是回放的，不是重新执行的。
// 客户端据此区分"我的请求真的被处理了"与"这是我上次那次的结果"。
const IdempotentReplayHeader = "Idempotent-Replay"

const (
	// 结果的保留时间：覆盖"点一次、网络抖十秒、再点一次"这类场景即可。
	// 太长会把缓存当数据库用，太短则重试落在窗口外。
	idemResultTTL = 24 * time.Hour
	// 执行权的锁：请求内部正常几秒内结束，一分钟足够；过期就允许重试。
	idemLockTTL = time.Minute
	// 只缓存小响应。PDF、导出这类会落到这个上限之外，它们本来也不该被回放。
	idemMaxBody = 256 << 10
)

// Idempotency 让带 Idempotency-Key 的 POST 只真正执行一次。
//
// 为什么只做 POST：PUT / DELETE 本身是幂等的，同一个请求重复发结果一样；
// 真正会重复建单的是"网络超时后又点了一次提交"。
//
// 键按"用户 + 方法 + 路径 + 客户端键"隔离：不隔离用户，A 的键能回放出 B 的响应；
// 不隔离路径，同一个键打两个接口会互相干扰。
func Idempotency(c cache.Cache, log *slog.Logger) gin.HandlerFunc {
	return func(gc *gin.Context) {
		raw := strings.TrimSpace(gc.GetHeader(IdempotencyKeyHeader))
		if raw == "" || gc.Request.Method != http.MethodPost {
			gc.Next()
			return
		}
		p, ok := PrincipalFrom(gc)
		if !ok {
			// 登录前没有身份，键只能全局共享，串号风险比省下的重复提交更大。
			gc.Next()
			return
		}

		// 缓存里的键用请求上下文，但写结果时不能跟着请求一起取消：
		// 请求结束时 ctx 已 Done，那一步 Set 会静默失败。
		reqCtx := gc.Request.Context()
		writeCtx := context.WithoutCancel(reqCtx)

		scope := fmt.Sprintf("idem:%s:%s:%d:%s", gc.Request.Method, gc.FullPath(), p.UserID, raw)
		resultKey, lockKey := scope+":result", scope+":lock"

		if cached, err := c.Get(reqCtx, resultKey); err == nil && cached != "" {
			if replay(gc, cached) {
				return
			}
			// 缓存内容坏掉时按未命中处理：宁可多执行一次，也不能让请求卡死。
		}

		n, err := c.Incr(reqCtx, lockKey, idemLockTTL)
		if err != nil {
			// 缓存不可用时放行：幂等是加分项，不该成为所有写操作的故障点。
			log.Warn("幂等键不可用，按普通请求处理", "err", err)
			gc.Next()
			return
		}
		if n > 1 {
			// 前一次还在这条路上：它的结果还没写进缓存，直接重放答案会是个空响应。
			response.Fail(gc, errs.ErrConflict.WithMsg(
				"同一个 Idempotency-Key 的请求正在处理，请勿重复提交"))
			return
		}

		rec := &bodyRecorder{ResponseWriter: gc.Writer}
		gc.Writer = rec
		gc.Next()

		// 5xx 不缓存：那是临时故障，重试应当真的重试，而不是把错误答案钉住一天。
		if rec.status >= http.StatusInternalServerError || rec.overflow || rec.status == 0 {
			_ = c.Del(writeCtx, lockKey)
			return
		}
		if payload, ok := rec.payload(); ok {
			if err := c.Set(writeCtx, resultKey, payload, idemResultTTL); err != nil {
				log.Warn("幂等结果写入失败", "err", err)
			}
		}
		_ = c.Del(writeCtx, lockKey)
	}
}

// idemResult 是缓存里那条记录的形态：状态码 + Content-Type + 原始响应体。
type idemResult struct {
	Status int    `json:"status"`
	Type   string `json:"type"`
	Body   string `json:"body"`
}

// replay 回放缓存里的响应，返回是否成功回放。
func replay(gc *gin.Context, cached string) bool {
	var r idemResult
	if err := json.Unmarshal([]byte(cached), &r); err != nil || r.Status == 0 {
		return false
	}
	gc.Header(IdempotentReplayHeader, "true")
	gc.Data(r.Status, r.Type, []byte(r.Body))
	gc.Abort()
	return true
}

// bodyRecorder 记下响应体，供写入缓存。
//
// 同时保留原始 Writer 的写入：客户端拿到的响应与没这个中间件时完全一致，
// 捕获只是旁路。
type bodyRecorder struct {
	gin.ResponseWriter
	body     bytes.Buffer
	status   int
	overflow bool
}

func (r *bodyRecorder) WriteHeader(code int) {
	if r.status == 0 {
		r.status = code
	}
	r.ResponseWriter.WriteHeader(code)
}

func (r *bodyRecorder) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	if !r.overflow {
		if r.body.Len()+len(b) > idemMaxBody {
			// 超限就放弃捕获并清空，别让一个巨大的响应把内存撑住。
			r.overflow = true
			r.body.Reset()
		} else {
			r.body.Write(b)
		}
	}
	return r.ResponseWriter.Write(b)
}

// payload 把响应序列化成可缓存的形式；不是 JSON 的（附件、PDF）不缓存。
func (r *bodyRecorder) payload() (string, bool) {
	ct := r.Header().Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		return "", false
	}
	raw, err := json.Marshal(idemResult{Status: r.status, Type: ct, Body: r.body.String()})
	if err != nil {
		return "", false
	}
	return string(raw), true
}
