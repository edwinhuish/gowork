package service

import (
	"fmt"
	"strings"
	"time"

	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/errs"
)

// parseDateOnly 解析业务日期，只认 YYYY-MM-DD，顺手容忍前端带上来的完整时间串。
//
// 业务日期（汇率生效日、跟进发生日）不是时间点，统一截到 UTC 零点存，
// 见 model.DateOnly。三处各写一遍解析，迟早有一处忘了截时间。
func parseDateOnly(raw string) (time.Time, error) {
	s := strings.TrimSpace(raw)
	if len(s) > len(rateDateLayout) {
		s = s[:len(rateDateLayout)]
	}
	d, err := time.Parse(rateDateLayout, s)
	if err != nil {
		return time.Time{}, errs.BadRequest("日期格式应为 YYYY-MM-DD")
	}
	return model.DateOnly(d), nil
}

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
