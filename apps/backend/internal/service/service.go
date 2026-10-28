// Package service 是业务层。
//
// 它只依赖 repository 接口、cache 接口和 dto，不认识 gin，也不写 SQL：
// 换 HTTP 框架、换数据库都不需要改这一层。
package service

import "github.com/edwinhuish/gowork/internal/pkg/errs"

var errInvalidStatus = errs.BadRequest("status 只能是 0 或 1")

// normStatus 把可选的 status 入参归一化，并拒绝越界值。
func normStatus(p *int8, def int8) (int8, error) {
	if p == nil {
		return def, nil
	}
	if *p != 0 && *p != 1 {
		return 0, errInvalidStatus
	}
	return *p, nil
}
