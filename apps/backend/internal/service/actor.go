package service

import "context"

// actorKey 是登录身份在请求上下文里的键。
type actorKey struct{}

// WithActor 把当前登录身份挂到 context 上。
//
// 为什么 gin.Context 之外还要复制一份：gin 只陪到 handler，进了 service 就只剩
// context.Context。而"谁能强制放行信用额度""谁删了这张单"这类判断属于业务规则，
// 只能靠参数一路透传才不会丢——每加一处判断就改一次方法签名，很快就没人愿意加了。
func WithActor(ctx context.Context, p *Principal) context.Context {
	if p == nil {
		return ctx
	}
	return context.WithValue(ctx, actorKey{}, p)
}

// ActorFrom 取出当前登录身份。内部调用（定时任务、迁移、服务间调用）没有身份，
// 此时 ok=false，需要身份的判断应当拒绝而不是当作"管理员"。
func ActorFrom(ctx context.Context) (*Principal, bool) {
	p, ok := ctx.Value(actorKey{}).(*Principal)
	return p, ok && p != nil
}
