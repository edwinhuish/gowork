// Package handler 是 HTTP 适配层。
//
// 只做三件事：解析参数、调用 service、写响应。
// 任何业务判断都不应出现在这里。
package handler

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"

	"github.com/edwinhuish/gowork/internal/pkg/errs"
	"github.com/edwinhuish/gowork/internal/pkg/response"
)

// parseID 解析路径上的 :id 参数。
func parseID(c *gin.Context) (uint, bool) {
	raw := c.Param("id")
	id, err := strconv.ParseUint(raw, 10, 64)
	if err != nil || id == 0 {
		response.Fail(c, errs.BadRequest("无效的资源 ID"))
		return 0, false
	}
	return uint(id), true
}

// bindJSON 绑定并校验请求体，失败时已写出响应。
func bindJSON(c *gin.Context, dst any) bool {
	if err := c.ShouldBindJSON(dst); err != nil {
		response.Fail(c, errs.BadRequest(validationMessage(err)))
		return false
	}
	return true
}

// bindQuery 绑定并校验查询参数，失败时已写出响应。
func bindQuery(c *gin.Context, dst any) bool {
	if err := c.ShouldBindQuery(dst); err != nil {
		response.Fail(c, errs.BadRequest(validationMessage(err)))
		return false
	}
	return true
}

// validationMessage 把校验错误翻译成可定位的中文提示。
func validationMessage(err error) string {
	var ve validator.ValidationErrors
	if errors.As(err, &ve) && len(ve) > 0 {
		fe := ve[0]
		field := fe.Field()
		switch fe.Tag() {
		case "required":
			return fmt.Sprintf("参数 %s 不能为空", field)
		case "email":
			return fmt.Sprintf("参数 %s 不是合法的邮箱", field)
		case "max":
			return fmt.Sprintf("参数 %s 长度不能超过 %s", field, fe.Param())
		case "min":
			return fmt.Sprintf("参数 %s 长度不能少于 %s", field, fe.Param())
		case "gte":
			return fmt.Sprintf("参数 %s 不能小于 %s", field, fe.Param())
		default:
			return fmt.Sprintf("参数 %s 校验失败(%s)", field, fe.Tag())
		}
	}
	return "请求参数格式不正确"
}
