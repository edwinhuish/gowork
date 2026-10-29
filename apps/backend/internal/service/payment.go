package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/errs"
	"github.com/edwinhuish/gowork/internal/pkg/money"
	"github.com/edwinhuish/gowork/internal/repository"
)

// PaymentService 管收付款单与核销。
//
// **核销不是冲销。** 核销回答"这笔钱还的是哪张单"，冲销是财务的红字冲回。
// 所以核销行只增不改：录错了删掉整张收付款单重录，目标单据的已核销金额
// 跟着回滚——而不是在核销行上改成负数。
//
// 超收判定与目标状态回写都在仓储的事务里做（见 repository/payment.go）：
// 它们要读"目标当前已核销多少"并在同一事务里改它，放到这里做就会在
// 检查与写入之间开一个窗口。
type PaymentService interface {
	Create(ctx context.Context, in dto.PaymentInput) (*model.Payment, error)
	Update(ctx context.Context, id uint, in dto.PaymentUpdateInput) (*model.Payment, error)
	Delete(ctx context.Context, id uint) error
	Get(ctx context.Context, id uint) (*model.Payment, error)
	List(ctx context.Context, q dto.PaymentQuery) (*dto.PageResult[model.Payment], error)
}

type paymentService struct {
	repo        repository.PaymentRepository
	receivables repository.ReceivableRepository
	payables    repository.PayableRepository
	currencies  CurrencyService
	docNos      DocNoService
	// partners 只用来判数据范围（T-32）：收付款的归属跟着对方主体走。
	partners repository.PartnerRepository
	clock    func() time.Time
	log      *slog.Logger
}

type PaymentOption func(*paymentService)

func WithPaymentClock(now func() time.Time) PaymentOption {
	return func(s *paymentService) { s.clock = now }
}

func WithPaymentLogger(l *slog.Logger) PaymentOption {
	return func(s *paymentService) { s.log = l }
}

func NewPaymentService(
	repo repository.PaymentRepository,
	receivables repository.ReceivableRepository,
	payables repository.PayableRepository,
	currencies CurrencyService,
	docNos DocNoService,
	partners repository.PartnerRepository,
	opts ...PaymentOption,
) PaymentService {
	s := &paymentService{
		repo:        repo,
		receivables: receivables,
		payables:    payables,
		currencies:  currencies,
		docNos:      docNos,
		partners:    partners,
		clock:       time.Now,
		log:         slog.Default(),
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *paymentService) Create(
	ctx context.Context, in dto.PaymentInput,
) (*model.Payment, error) {
	dir := model.PaymentDirection(in.Direction)
	if !dir.IsValid() {
		return nil, errs.BadRequest("方向不合法：10=收款，20=付款")
	}

	payDate, err := s.resolvePayDate(in.PayDate)
	if err != nil {
		return nil, err
	}

	// 收款只能核应收、付款只能核应付。方向与目标错配是录单时最常见的错，
	// 而它一旦落库，账龄报表会把一笔付出去的钱算成收回来。
	target := model.SettlementTargetReceivable
	if dir == model.PaymentDirectionOut {
		target = model.SettlementTargetPayable
	}

	currency := strings.ToUpper(strings.TrimSpace(in.CurrencyCode))
	partnerID := in.PartnerID
	allocations := make([]model.PaymentAllocation, 0, len(in.Allocations))
	allocSum := money.Zero()

	for _, a := range in.Allocations {
		if model.SettlementTarget(a.TargetType) != target {
			return nil, errs.BadRequest(fmt.Sprintf(
				"%s只能核销%s", dir.Label(), target.Label()))
		}
		docNo, cur, pid, err := s.targetInfo(ctx, target, a.TargetID)
		if err != nil {
			return nil, err
		}
		if currency == "" {
			currency = cur
		}
		if cur != currency {
			return nil, errs.BadRequest(fmt.Sprintf(
				"同一笔收付款的核销目标币种必须一致：%s 是 %s，其余是 %s",
				docNo, cur, currency))
		}
		if partnerID == 0 {
			partnerID = pid
		}
		if pid != partnerID {
			return nil, errs.BadRequest(
				fmt.Sprintf("核销目标的对方主体不一致：%s 不属于同一个往来单位", docNo))
		}

		amount := a.Amount.Round(4)
		if amount.Cmp(money.Zero()) <= 0 {
			return nil, errs.BadRequest("核销金额必须大于 0；要撤销核销请删掉整张收付款单")
		}
		allocSum = allocSum.Add(amount)
		allocations = append(allocations, model.PaymentAllocation{
			TargetType:  target,
			TargetID:    a.TargetID,
			TargetDocNo: docNo,
			Amount:      amount,
		})
	}
	allocSum = allocSum.Round(4)

	if partnerID == 0 {
		return nil, errs.BadRequest("请指定往来单位，或至少核销一张单据")
	}

	// 金额留空 = 全额核销。不核销的预收款必须显式给金额，
	// 否则会得到一张 0 元的收款单。
	amount := in.Amount.Round(4)
	if amount.IsZero() {
		if allocSum.IsZero() {
			return nil, errs.BadRequest(fmt.Sprintf(
				"%s金额不能为 0：预收 / 预付不核销时必须显式填写金额", dir.Label()))
		}
		amount = allocSum
	}
	// 核销合计不得超过本单金额。定点之后逐位比较：多核一分钱也是多核。
	if amount.Cmp(allocSum) < 0 {
		return nil, errs.BadRequest(fmt.Sprintf(
			"核销合计 %s 超过%s金额 %s", allocSum, dir.Label(), amount))
	}

	if currency == "" {
		baseCode, err := s.currencies.BaseCode(ctx)
		if err != nil {
			return nil, err
		}
		currency = baseCode
	}

	// 汇率取**到账日**的，不是应收开单时锁的：两者的差就是汇兑损益。
	amountBase, rate, err := s.currencies.ToBaseDecimal(ctx, amount, currency, payDate)
	if err != nil {
		return nil, err
	}
	amountBase = amountBase.Round(4)
	// 每行按本单汇率折本币，逐行舍入后与单据本币相差不超过几行末位——
	// 汇兑损益逐笔算，所以这里不能把单据本币按行数摊回去。
	for i := range allocations {
		allocations[i].AmountBase = allocations[i].Amount.Mul(rate).Round(4)
	}

	docNo, err := s.docNos.Next(ctx, model.DocTypePayment)
	if err != nil {
		return nil, err
	}

	m := &model.Payment{
		DocNo:        docNo,
		Direction:    dir,
		PartnerID:    partnerID,
		PayDate:      payDate,
		CurrencyCode: currency,
		ExchangeRate: rate,
		Amount:       amount,
		AmountBase:   amountBase,
		Method:       model.SettlementMethod(strings.TrimSpace(in.Method)),
		Reference:    strings.TrimSpace(in.Reference),
		Remark:       in.Remark,
		Allocations:  allocations,
	}
	if err := s.repo.Create(ctx, m); err != nil {
		return nil, err
	}
	s.log.Info("新增收付款", "doc_no", m.DocNo, "direction", m.Direction.Label(),
		"amount", m.Amount, "currency", m.CurrencyCode, "allocations", len(m.Allocations))
	return s.repo.FindByID(ctx, m.ID)
}

func (s *paymentService) Update(
	ctx context.Context, id uint, in dto.PaymentUpdateInput,
) (*model.Payment, error) {
	if _, err := s.repo.FindByID(ctx, id); err != nil {
		return nil, err
	}
	fields := map[string]any{
		"method":    strings.TrimSpace(in.Method),
		"reference": strings.TrimSpace(in.Reference),
		"remark":    in.Remark,
	}
	if strings.TrimSpace(in.PayDate) != "" {
		d, err := parseDateOnly(in.PayDate)
		if err != nil {
			return nil, errs.BadRequest("到账日格式应为 YYYY-MM-DD")
		}
		fields["pay_date"] = d
	}
	if in.UpdatedAt != nil {
		fields[repository.FieldUpdatedAt] = *in.UpdatedAt
	}
	if err := s.repo.Update(ctx, id, fields); err != nil {
		return nil, err
	}
	return s.repo.FindByID(ctx, id)
}

// Delete 会连同核销一起回滚：目标单据的已核销金额减去这张单核销的部分。
// 这就是"核销不是冲销"的另一面——撤销是删除，不是再记一笔负数。
func (s *paymentService) Delete(ctx context.Context, id uint) error {
	return s.repo.Delete(ctx, id)
}

func (s *paymentService) Get(ctx context.Context, id uint) (*model.Payment, error) {
	m, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	// 越权 403 而不是 404：这张收付款确实存在，只是不归你看。
	if err := ensurePartnerVisible(ctx, s.partners, m.PartnerID); err != nil {
		return nil, err
	}
	return m, nil
}

func (s *paymentService) List(
	ctx context.Context, q dto.PaymentQuery,
) (*dto.PageResult[model.Payment], error) {
	q.PageQuery.Normalize()

	// 范围由身份决定：业务员只看自己（与未分配）对方的收付款。
	filter := repository.PaymentFilter{PartnerID: q.PartnerID, OwnerID: ownerScope(ctx)}
	if q.Direction != nil {
		d := model.PaymentDirection(*q.Direction)
		filter.Direction = &d
	}
	page := repository.Pagination{Page: q.Page, PageSize: q.PageSize}

	list, total, err := s.repo.List(ctx, filter, page)
	if err != nil {
		return nil, err
	}
	return dto.NewPageResult(list, total, q.Page, q.PageSize), nil
}

func (s *paymentService) resolvePayDate(raw string) (time.Time, error) {
	if strings.TrimSpace(raw) == "" {
		return model.DateOnly(s.clock()), nil
	}
	d, err := parseDateOnly(raw)
	if err != nil {
		return time.Time{}, errs.BadRequest("到账日格式应为 YYYY-MM-DD")
	}
	return d, nil
}

// targetInfo 取核销目标的单号、币种与对方主体，用于填核销行与做一致性校验。
//
// 这里查一次只是为了填字段与给出可读的错误信息；额度的最终判定在仓储的
// 事务里再做一次——那一次的读数才是决定性的。
func (s *paymentService) targetInfo(
	ctx context.Context, target model.SettlementTarget, id uint,
) (docNo, currency string, partnerID uint, err error) {
	switch target {
	case model.SettlementTargetReceivable:
		var m *model.Receivable
		m, err = s.receivables.FindByID(ctx, id)
		if err != nil {
			if errors.Is(err, errs.ErrNotFound) {
				return "", "", 0, errs.BadRequest(fmt.Sprintf("应收单 %d 不存在", id))
			}
			return "", "", 0, err
		}
		return m.DocNo, m.CurrencyCode, m.PartnerID, nil
	case model.SettlementTargetPayable:
		var m *model.Payable
		m, err = s.payables.FindByID(ctx, id)
		if err != nil {
			if errors.Is(err, errs.ErrNotFound) {
				return "", "", 0, errs.BadRequest(fmt.Sprintf("应付单 %d 不存在", id))
			}
			return "", "", 0, err
		}
		return m.DocNo, m.CurrencyCode, m.PartnerID, nil
	default:
		return "", "", 0, errs.BadRequest("核销对象类型不合法：10=应收单，20=应付单")
	}
}
