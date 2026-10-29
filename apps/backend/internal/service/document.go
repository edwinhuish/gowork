package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/errs"
	"github.com/edwinhuish/gowork/internal/pkg/money"
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

// parseOptionalDate 解析可留空的业务日期：空串返回 nil，表示"不限"。
//
// 列表筛选里 from / to 都是可选的，用 time.Time 的零值当"不限"会筛出空结果——
// 零值是公元 1 年，任何记录都晚于它，"结束日期"那一侧就会全被过滤掉。
func parseOptionalDate(raw string) (*time.Time, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	d, err := parseDateOnly(raw)
	if err != nil {
		return nil, err
	}
	return &d, nil
}

// parseDateOrDefault 解析可留空的业务日期，留空取 now 当天。
//
// 单据日期、流水发生日都是"留空即今天"，且都只取日期部分——
// 三处各写一遍的话，总有一处会忘了截时间，于是同一天的单据落在两个统计月份。
func parseDateOrDefault(raw string, now time.Time) (time.Time, error) {
	if strings.TrimSpace(raw) == "" {
		return model.DateOnly(now), nil
	}
	return parseDateOnly(raw)
}

// resolveCurrency 定下单据的币种、当日汇率与两侧金额小数位。
//
// 收在一处：报价单与订单都要"币种留空即本币、按当日汇率折本币、按各自小数位舍入"，
// 分开写迟早出现"报价单折 2 位、订单折 4 位"的差异，对账时差额没人解释得清。
func resolveCurrency(
	ctx context.Context, currencies CurrencyService, raw string, on time.Time,
) (code string, rate money.Decimal, curDP, baseDP int, err error) {
	baseCode, err := currencies.BaseCode(ctx)
	if err != nil {
		return "", money.Zero(), 0, 0, err
	}
	code = strings.ToUpper(strings.TrimSpace(raw))
	if code == "" {
		code = baseCode
	}
	// 折 1 个单位顺带完成了"币种是否存在"与"当日汇率是否已录"两项校验：
	// 缺汇率时给 400 而不是把 1 当汇率写进单据，那会让本币金额凭空多一个汇率倍数。
	// 折算结果不用，要的是它取出的那条汇率本身。
	_, rate, err = currencies.ToBaseDecimal(ctx, money.FromInt(1), code, on)
	if err != nil {
		return "", money.Zero(), 0, 0, err
	}
	curDP, err = currencies.Decimals(ctx, code)
	if err != nil {
		return "", money.Zero(), 0, 0, err
	}
	baseDP, err = currencies.Decimals(ctx, baseCode)
	if err != nil {
		return "", money.Zero(), 0, 0, err
	}
	return code, rate, curDP, baseDP, nil
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
