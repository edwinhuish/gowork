package service

import (
	"context"

	"github.com/edwinhuish/gowork/internal/pkg/errs"
	"github.com/edwinhuish/gowork/internal/repository"
)

// ensurePartnerVisible 校验当前身份能否看到属于该客户的单据。
//
// 列表已经按范围过滤，但"猜到 id 直接请求详情"是另一条路：不校验的话，
// 业务员拿到别人那张单的 id 就能把整张单连明细一起读走。
//
// 查询失败一律往上抛而不是当成"看不见"：仓储故障与越权是两回事，
// 把它们混成一个 403 会让真正的故障永远查不出来。
func ensurePartnerVisible(
	ctx context.Context, partners repository.PartnerRepository, partnerID uint,
) error {
	owner := ownerScope(ctx)
	if owner == nil {
		return nil
	}
	p, err := partners.FindByID(ctx, partnerID)
	if err != nil {
		return err
	}
	// owner_id = 0 是未分配（公共池），对所有人可见，与列表口径一致。
	if p.OwnerID != 0 && p.OwnerID != *owner {
		return errs.ErrForbidden
	}
	return nil
}
