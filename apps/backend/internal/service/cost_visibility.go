package service

import (
	"context"

	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/money"
)

// canSeeCost 报告当前身份能否看到成本与毛利（T-36 字段级权限）。
//
// 规则与数据范围同一套：管理员与主管能看，业务员看不到。
// 认不出身份（内部调用、迁移任务）时**放行**——那些调用不是为了"某个人看"，
// 而是系统自己在算，把它们当成业务员会让内部计算拿到一堆 0。
func canSeeCost(ctx context.Context) bool {
	actor, ok := ActorFrom(ctx)
	if !ok {
		return true
	}
	return model.RoleSeesAllData(actor.Role)
}

// redactCost 把成本类金额清零，并挂上"被隐藏"的标记。
//
// 隐藏发生在**服务端**：只在前端不渲染，等于把数字发给了不该看到的人，
// 随手打开开发者工具就能拿到，那不是权限，是界面效果。
//
// 清成 0 而不是省略字段：金额是数字，省略会让前端拿到 undefined 再显示成 NaN，
// 而 0 配合 CostHidden 能被界面读成"看不到"，也不会让任何求和逻辑崩掉。
func redactCost(ctx context.Context, value money.Decimal) money.Decimal {
	if canSeeCost(ctx) {
		return value
	}
	return money.Zero()
}

// redactProductCost 按身份清掉产品的成本价（列表与详情共用）。
func redactProductCost(ctx context.Context, products ...*model.Product) {
	if canSeeCost(ctx) {
		return
	}
	for _, p := range products {
		if p == nil {
			continue
		}
		p.CostPrice = money.Zero()
		p.CostHidden = true
	}
}
