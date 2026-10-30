// Package service 是业务层。
//
// 它只依赖 repository 接口、cache 接口和 dto，不认识 gin，也不写 SQL：
// 换 HTTP 框架、换数据库都不需要改这一层。
package service

import (
	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/errs"
)

// 下面两个 helper 是给"还保留启用状态的两个实体"用的：
// 账号的启用 / 禁用，与币种的归档标记（同一个字段，币种那边叫归档）。
//
// 其余主数据的启用状态已经拿掉了——它们原本就具备完整的软删除链路，
// 多一层 state 只会让"这条记录到底算不算还在"有两个答案。
var errInvalidEnableStatus = errs.BadRequest("status 只能是 1（启用）或 0（停用）")

// normEnableStatus 把可选的启用状态入参归一化，并拒绝越界值。
//
// 入参 nil 表示调用方没提交这个字段，退回默认值 def；提交了就必须合法。
// 不做"非法值当默认值"的静默纠正——那会让前端传错的字段一直藏着。
func normEnableStatus(p dto.EnableStatusInput, def model.EnableStatus) (model.EnableStatus, error) {
	if p == nil {
		return def, nil
	}
	st := model.EnableStatus(*p)
	if st != model.EnableStatusOn && st != model.EnableStatusOff {
		return 0, errInvalidEnableStatus
	}
	return st, nil
}

// enableStatusPtr 把 dto 的可选状态转成仓储层用的枚举指针。
//
// dto 刻意不 import model（保持契约层零依赖），所以两者的指针类型不同，
// 这里必须显式转一次。列表筛选不校验取值范围：传 status=5 的含义是
// "没有这种状态的记录"，返回空列表比报错更符合筛选语义。
func enableStatusPtr(p dto.EnableStatusInput) *model.EnableStatus {
	if p == nil {
		return nil
	}
	st := model.EnableStatus(*p)
	return &st
}
