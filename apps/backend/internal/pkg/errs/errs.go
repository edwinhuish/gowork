// Package errs 定义全局业务错误码与错误包装。
//
// 约定：错误码前三位与 HTTP 状态码对齐（40001 -> 400），
// 这样 handler 不需要为每个错误单独写状态码，前端也可以按前三位做统一分支。
package errs

import (
	"errors"
	"fmt"
	"net/http"
)

// Code 业务错误码。
type Code int

const (
	CodeOK Code = 0

	CodeInvalidParam Code = 40001
	CodeUnauthorized Code = 40101
	CodeTokenExpired Code = 40102
	CodeForbidden    Code = 40301
	CodeNotFound     Code = 40401
	CodeConflict     Code = 40901
	CodeInternal     Code = 50001
)

// HTTPStatus 把业务错误码映射为 HTTP 状态码。
func (c Code) HTTPStatus() int {
	switch c / 100 {
	case 400:
		return http.StatusBadRequest
	case 401:
		return http.StatusUnauthorized
	case 403:
		return http.StatusForbidden
	case 404:
		return http.StatusNotFound
	case 409:
		return http.StatusConflict
	case 500:
		return http.StatusInternalServerError
	}
	if c == CodeOK {
		return http.StatusOK
	}
	return http.StatusInternalServerError
}

// Error 是带业务码的错误。它实现了 Unwrap，可以继续用 errors.Is / errors.As 判断底层错误。
type Error struct {
	Code Code
	Msg  string
	err  error
}

func New(code Code, msg string) *Error {
	return &Error{Code: code, Msg: msg}
}

func (e *Error) Error() string {
	if e.err != nil {
		return fmt.Sprintf("%d: %s: %v", e.Code, e.Msg, e.err)
	}
	return fmt.Sprintf("%d: %s", e.Code, e.Msg)
}

func (e *Error) Unwrap() error { return e.err }

// WithMsg 返回一个替换了提示文案的副本，避免污染包级变量。
func (e *Error) WithMsg(msg string) *Error {
	cp := *e
	cp.Msg = msg
	return &cp
}

// Wrap 在保留业务码的同时挂上底层错误，用于日志追溯。
func Wrap(err error, code Code, msg string) *Error {
	return &Error{Code: code, Msg: msg, err: err}
}

// From 尝试把任意错误还原为 *Error，非业务错误返回 ok=false。
func From(err error) (*Error, bool) {
	var e *Error
	if errors.As(err, &e) {
		return e, true
	}
	return nil, false
}

var (
	ErrNotFound     = New(CodeNotFound, "资源不存在")
	ErrUnauthorized = New(CodeUnauthorized, "未登录或登录状态已失效")
	ErrTokenExpired = New(CodeTokenExpired, "登录已过期，请重新登录")
	ErrForbidden    = New(CodeForbidden, "没有操作权限")
	ErrInternal     = New(CodeInternal, "服务器内部错误")
	ErrConflict     = New(CodeConflict, "数据已存在或状态冲突")
)

// BadRequest 构造参数错误。
func BadRequest(msg string) *Error { return New(CodeInvalidParam, msg) }
