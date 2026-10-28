package service

import (
	"fmt"

	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/errs"
)

// checkDocumentTransit 校验单据状态跳转。
//
// 收成一处而不是各模块自己写 if：状态机规则一旦有第二份实现，两份就会分叉，
// 而分叉的表现是"同样一次取消，报价单允许、订单拒绝"，用户无从判断哪个是对的。
//
// 返回 409 而不是 400：请求本身合法，是当前状态不允许，前端据此提示"请先确认订单"。
func checkDocumentTransit(from, to model.DocumentStatus) error {
	if !to.IsValid() {
		return errs.BadRequest(fmt.Sprintf("未知的单据状态：%d", int8(to)))
	}
	if !from.CanTransitTo(to) {
		return errs.ErrConflict.WithMsg(
			fmt.Sprintf("单据状态不能从「%s」变为「%s」", from.Label(), to.Label()))
	}
	return nil
}
