package service

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/pkg/errs"
	"github.com/edwinhuish/gowork/internal/pkg/money"
	"github.com/edwinhuish/gowork/internal/repository"
)

// CreditService 管客户信用额度：敞口怎么算、什么时候拦、谁能放行。
//
// 额度为 0 视为不限，沿用 partner.CreditLimit 上的既有约定。这个约定的代价是
// 无法表达"这个客户一分钱都不赊"——那种诉求应该写"只接受前 T/T"，
// 而不是把额度设成 0 再猜系统会怎么理解。
type CreditService interface {
	// Summary 给出主体当前的额度与占用，供下单页提示。
	Summary(ctx context.Context, partnerID uint) (*dto.CreditSummary, error)
	// CheckConfirm 在订单确认前校验额度：超额返回 409，admin 可强制放行。
	CheckConfirm(ctx context.Context, in CreditCheck) error
}

// CreditCheck 是一次"这笔钱要占额度"的申请。
type CreditCheck struct {
	PartnerID uint
	// DocNo 是订单号，只用于留痕：看清是哪张单放行的。
	DocNo string
	// AmountBase 是本单本币金额（订单上锁定的合计）。
	AmountBase money.Decimal
	// Force 强制放行。传了也未必有用——还要看调用者是不是 admin。
	Force bool
}

type creditService struct {
	credits  repository.CreditRepository
	partners repository.PartnerRepository
	log      *slog.Logger
}

type CreditOption func(*creditService)

func WithCreditLogger(l *slog.Logger) CreditOption {
	return func(s *creditService) { s.log = l }
}

func NewCreditService(
	credits repository.CreditRepository,
	partners repository.PartnerRepository,
	opts ...CreditOption,
) CreditService {
	s := &creditService{credits: credits, partners: partners, log: slog.Default()}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *creditService) Summary(
	ctx context.Context, partnerID uint,
) (*dto.CreditSummary, error) {
	partner, err := s.partners.FindByID(ctx, partnerID)
	if err != nil {
		return nil, err
	}
	exp, err := s.credits.Exposure(ctx, partnerID)
	if err != nil {
		return nil, err
	}
	return buildCreditSummary(partner.ID, partner.CreditLimit, exp), nil
}

// CheckConfirm 判断这一单能不能确认。
//
// 只拦"确认"这一步，不拦建草稿：草稿可以改、可以删，不占额度；在录入阶段就拦，
// 业务员连算价都做不了，最后只会把额度调高来绕过。
func (s *creditService) CheckConfirm(ctx context.Context, in CreditCheck) error {
	partner, err := s.partners.FindByID(ctx, in.PartnerID)
	if err != nil {
		return err
	}
	if partner.CreditLimit.Cmp(money.Zero()) <= 0 {
		return nil
	}

	exp, err := s.credits.Exposure(ctx, in.PartnerID)
	if err != nil {
		return err
	}
	total := exp.Total().Add(in.AmountBase).Round(4)
	over := total.Sub(partner.CreditLimit).Round(4)
	// 刚好用满额度是允许的：over 为 0 时不拦。
	if over.Cmp(money.Zero()) <= 0 {
		return nil
	}

	msg := fmt.Sprintf(
		"超过客户信用额度：额度 %s，已用敞口 %s，本单 %s（超出 %s）；如需放行请由管理员强制放行",
		partner.CreditLimit, exp.Total(), in.AmountBase, over)

	if !in.Force {
		return errs.ErrConflict.WithMsg(msg)
	}

	actor, ok := ActorFrom(ctx)
	if !ok || !actor.IsAdmin() {
		return errs.ErrForbidden.WithMsg("只有管理员可以强制放行信用额度")
	}

	// 先落应用日志：T-33 的审计表还没落地，而"谁放行了这一单"此刻必须有人记得。
	s.log.Warn("信用额度强制放行",
		"doc_no", in.DocNo,
		"partner_id", in.PartnerID,
		"partner_code", partner.Code,
		"credit_limit", partner.CreditLimit,
		"exposure", exp.Total().Round(4),
		"amount", in.AmountBase,
		"over", over,
		"actor_id", actor.UserID,
		"actor", actor.Username,
	)
	return nil
}

func buildCreditSummary(
	partnerID uint, limit money.Decimal, exp repository.CreditExposure,
) *dto.CreditSummary {
	exposure := exp.Total().Round(4)
	out := &dto.CreditSummary{
		PartnerID:      partnerID,
		CreditLimit:    limit,
		OpenReceivable: exp.OpenReceivableBase.Round(4),
		UnbilledOrder:  exp.UnbilledOrderBase.Round(4),
		Exposure:       exposure,
	}
	if limit.Cmp(money.Zero()) <= 0 {
		out.Unlimited = true
		return out
	}
	// 超出时给负数：让"已经超了多少"在界面上直接可见，而不是又不给数。
	out.Available = limit.Sub(exposure).Round(4)
	return out
}
