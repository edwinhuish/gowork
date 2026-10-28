// Package response 统一 HTTP 响应格式。
//
// 所有接口返回 {"code":0,"message":"ok","data":...,"request_id":"..."}，
// code != 0 时 data 为空，前端按 code 分支处理。
package response

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/edwinhuish/gowork/internal/pkg/errs"
)

// RequestIDKey 是请求 ID 在 gin.Context 中的键。
const RequestIDKey = "request_id"

type Body struct {
	Code      int    `json:"code"`
	Message   string `json:"message"`
	Data      any    `json:"data,omitempty"`
	RequestID string `json:"request_id,omitempty"`
}

func requestID(c *gin.Context) string {
	if v, ok := c.Get(RequestIDKey); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// OK 返回成功响应。
func OK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, Body{Code: 0, Message: "ok", Data: data, RequestID: requestID(c)})
}

// Created 返回创建成功响应。
func Created(c *gin.Context, data any) {
	c.JSON(http.StatusCreated, Body{Code: 0, Message: "created", Data: data, RequestID: requestID(c)})
}

// Fail 输出错误并终止后续 handler。非业务错误统一折叠为 500，避免泄露内部细节。
func Fail(c *gin.Context, err error) {
	e, ok := errs.From(err)
	if !ok {
		e = errs.Wrap(err, errs.CodeInternal, "服务器内部错误")
	}
	c.AbortWithStatusJSON(e.Code.HTTPStatus(), Body{
		Code:      int(e.Code),
		Message:   e.Msg,
		RequestID: requestID(c),
	})
}
