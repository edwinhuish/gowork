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

// ReceivableService 管应收单：由销售订单生成、锁汇率、支持分期。
//
// **一张订单可以开出多张应收**（30% 订金 / 70% 尾款），每张带自己的到期日与号。
// 不把分期做进单内：核销（T-21）按单进行，分期在单内就得再指定"核销第几期"，
// 而 F2 描述的核销对分期一无所知。来源总额与已开票金额的关系由这里守——
// 数据库上加不了这个约束，它要跨表 SUM，做不成 CHECK。
type ReceivableService interface {
	Create(ctx context.Context, in dto.ReceivableInput) (*model.Receivable, error)
	Update(ctx context.Context, id uint, in dto.ReceivableInput) (*model.Receivable, error)
	Delete(ctx context.Context, id uint) error
	Get(ctx context.Context, id uint) (*model.Receivable, error)
	List(ctx context.Context, q dto.ReceivableQuery) (*dto.PageResult[model.Receivable], error)
}

type receivableService struct {
	repo     repository.ReceivableRepository
	orders   repository.SalesOrderRepository
	partners repository.PartnerRepository
	docNos   DocNoService
	clock    func() time.Time
	log      *slog.Logger
}

type ReceivableOption func(*receivableService)

func WithReceivableClock(now func() time.Time) ReceivableOption {
	return func(s *receivableService) { s.clock = now }
}

func WithReceivableLogger(l *slog.Logger) ReceivableOption {
	return func(s *receivableService) { s.log = l }
}

func NewReceivableService(
	repo repository.ReceivableRepository,
	orders repository.SalesOrderRepository,
	partners repository.PartnerRepository,
	docNos DocNoService,
	opts ...ReceivableOption,
) ReceivableService {
	s := &receivableService{
		repo:     repo,
		orders:   orders,
		partners: partners,
		docNos:   docNos,
		clock:    time.Now,
		log:      slog.Default(),
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *receivableService) Create(
	ctx context.Context, in dto.ReceivableInput,
) (*model.Receivable, error) {
	terms, err := s.buildTerms(ctx, in, 0)
	if err != nil {
		return nil, err
	}
	docNo, err := s.docNos.Next(ctx, model.DocTypeReceivable)
	if err != nil {
		return nil, err
	}

	m := &model.Receivable{Settlement: model.Settlement{
		DocNo:        docNo,
		PartnerID:    terms.PartnerID,
		SourceType:   model.SettlementSourceSalesOrder,
		SourceID:     in.SourceID,
		SourceDocNo:  terms.SourceDocNo,
		CurrencyCode: terms.CurrencyCode,
		ExchangeRate: terms.ExchangeRate,
		Amount:       terms.Amount,
		AmountBase:   terms.AmountBase,
		DueDate:      terms.DueDate,
		Status:       model.SettlementStatusOpen,
		Remark:       in.Remark,
	}}
	if err := s.repo.Create(ctx, m); err != nil {
		return nil, err
	}
	s.log.Info("新增应收单", "doc_no", m.DocNo, "source", m.SourceDocNo,
		"amount", m.Amount, "currency", m.CurrencyCode)
	return s.repo.FindByID(ctx, m.ID)
}

func (s *receivableService) Update(
	ctx context.Context, id uint, in dto.ReceivableInput,
) (*model.Receivable, error) {
	existing, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	// 换来源单据等于这张单描述的是另一笔生意，两边的开票金额都会因此对不上。
	if in.SourceID != 0 && in.SourceID != existing.SourceID {
		return nil, errs.BadRequest("应收单不能改挂到另一张订单，请在新订单下新建")
	}
	if existing.Status == model.SettlementStatusVoid {
		return nil, errs.ErrConflict.WithMsg("应收单已作废，不能再改")
	}
	if existing.SettledAmount.Cmp(money.Zero()) > 0 {
		return nil, errs.ErrConflict.WithMsg(
			fmt.Sprintf("应收单已有收款核销（%s %s），金额与到期日不能再改",
				existing.CurrencyCode, existing.SettledAmount))
	}

	terms, err := s.buildTerms(ctx, in, existing.ID)
	if err != nil {
		return nil, err
	}

	status := existing.Status
	if in.Status != nil {
		// 只接受"作废"。部分结清与已结清由核销（T-21）推导，
		// 手工设成"已结清"会造出一张没有收款记录的已收单据。
		if model.SettlementStatus(*in.Status) != model.SettlementStatusVoid {
			return nil, errs.BadRequest("应收单手改状态只支持作废（90）；结清状态由收款核销推导")
		}
		status = model.SettlementStatusVoid
	}

	fields := map[string]any{
		"amount":        terms.Amount,
		"amount_base":   terms.AmountBase,
		"currency_code": terms.CurrencyCode,
		"exchange_rate": terms.ExchangeRate,
		"due_date":      terms.DueDate,
		"status":        status,
		"remark":        in.Remark,
	}
	if in.UpdatedAt != nil {
		fields[repository.FieldUpdatedAt] = *in.UpdatedAt
	}
	if err := s.repo.Update(ctx, id, fields); err != nil {
		return nil, err
	}
	return s.repo.FindByID(ctx, id)
}

// Delete 只放行"还没收到过一分钱"的单。
//
// 收过款的单删掉，核销记录就成了指向不存在单据的孤儿；那种情况该作废而不是删除。
func (s *receivableService) Delete(ctx context.Context, id uint) error {
	existing, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return err
	}
	if existing.SettledAmount.Cmp(money.Zero()) > 0 {
		return errs.ErrConflict.WithMsg("已有收款核销的应收单不能删除，请改为作废")
	}
	return s.repo.Delete(ctx, id)
}

func (s *receivableService) Get(ctx context.Context, id uint) (*model.Receivable, error) {
	return s.repo.FindByID(ctx, id)
}

func (s *receivableService) List(
	ctx context.Context, q dto.ReceivableQuery,
) (*dto.PageResult[model.Receivable], error) {
	q.PageQuery.Normalize()

	filter := repository.ReceivableFilter{PartnerID: q.PartnerID, SourceID: q.SourceID}
	if q.SourceType != nil {
		st := model.SettlementSource(*q.SourceType)
		filter.SourceType = &st
	}
	if q.Status != nil {
		st := model.SettlementStatus(*q.Status)
		filter.Status = &st
	}
	if q.Overdue != nil && *q.Overdue == 1 {
		today := model.DateOnly(s.clock())
		filter.OverdueBefore = &today
	}
	page := repository.Pagination{Page: q.Page, PageSize: q.PageSize}

	list, total, err := s.repo.List(ctx, filter, page)
	if err != nil {
		return nil, err
	}
	return dto.NewPageResult(list, total, q.Page, q.PageSize), nil
}

// receivableTerms 是校验之后的应收字段。
type receivableTerms struct {
	PartnerID    uint
	SourceDocNo  string
	CurrencyCode string
	ExchangeRate money.Decimal
	Amount       money.Decimal
	AmountBase   money.Decimal
	DueDate      time.Time
}

func (s *receivableService) buildTerms(
	ctx context.Context, in dto.ReceivableInput, excludeID uint,
) (receivableTerms, error) {
	var out receivableTerms

	// 只能由销售订单生成。出货批次作为来源要先把"一票装了哪些订单行、各多少钱"
	// 算出来，而拼柜可能跨币种——那条路等 T-22 需要按票算毛利时再开，
	// 现在硬加上去只会得到一个"只在单币种整票时才对"的金额。
	if model.SettlementSource(in.SourceType) != model.SettlementSourceSalesOrder {
		return out, errs.BadRequest("应收只能由销售订单生成（source_type=10）")
	}

	order, err := s.orders.FindByID(ctx, in.SourceID)
	if err != nil {
		if errors.Is(err, errs.ErrNotFound) {
			return out, errs.BadRequest("来源销售订单不存在")
		}
		return out, err
	}
	if order.Status == model.DocumentStatusDraft || order.Status == model.DocumentStatusCancelled {
		return out, errs.BadRequest(
			fmt.Sprintf("销售订单 %s 是%s，不能开应收", order.DocNo, order.Status.Label()))
	}
	if order.AmountTotal.Cmp(money.Zero()) <= 0 {
		return out, errs.BadRequest(fmt.Sprintf("销售订单 %s 金额为 0，不能开应收", order.DocNo))
	}

	// 累计不得超过订单总额。作废的单不占额度（见仓储注释），
	// 否则一次开错就再也开不出正确的那张。
	issued, err := s.repo.SumAmountBySource(
		ctx, model.SettlementSourceSalesOrder, order.ID, excludeID)
	if err != nil {
		return out, err
	}
	remaining := order.AmountTotal.Sub(issued).Round(4)
	if remaining.Cmp(money.Zero()) <= 0 {
		return out, errs.ErrConflict.WithMsg(
			fmt.Sprintf("销售订单 %s 已全额开票（%s %s）", order.DocNo, order.CurrencyCode, issued))
	}

	amount := in.Amount
	if amount.IsZero() {
		amount = remaining
	}
	if amount.IsNegative() {
		return out, errs.BadRequest("应收金额不能为负；要冲减请作废这张单重开")
	}
	amount = amount.Round(4)
	if amount.Cmp(remaining) > 0 {
		return out, errs.BadRequest(
			fmt.Sprintf("金额超出未开票余额：%s 最多可开 %s", order.DocNo, remaining))
	}

	// 币种与汇率**从订单锁定**，不取当日汇率（F3 的硬规则）。
	dueDate, err := s.resolveDueDate(ctx, in.DueDate, order.OrderDate, order.PartnerID)
	if err != nil {
		return out, err
	}

	out = receivableTerms{
		PartnerID:    order.PartnerID,
		SourceDocNo:  order.DocNo,
		CurrencyCode: order.CurrencyCode,
		ExchangeRate: order.ExchangeRate,
		Amount:       amount,
		AmountBase:   amount.Mul(order.ExchangeRate).Round(4),
		DueDate:      dueDate,
	}
	return out, nil
}

// resolveDueDate 决定到期日：显式给了就用，没给就用「单据日 + 对方账期」。
//
// 账期为 0 表示即期结清，到期日就等于单据日——这不是"没设置"，
// 而是最常见的一种约定，所以不能拿它当错误。
func (s *receivableService) resolveDueDate(
	ctx context.Context, raw string, base time.Time, partnerID uint,
) (time.Time, error) {
	if strings.TrimSpace(raw) != "" {
		d, err := parseDateOnly(raw)
		if err != nil {
			return time.Time{}, errs.BadRequest("到期日格式应为 YYYY-MM-DD")
		}
		return d, nil
	}
	p, err := s.partners.FindByID(ctx, partnerID)
	if err != nil {
		return time.Time{}, err
	}
	// 业务日期统一走 model.DateOnly：它是"哪一天"，不是某个时刻。
	// 直接拿库里的时间加天数，会在时区边界上漂一天。
	return model.DateOnly(base).AddDate(0, 0, p.CreditDays), nil
}
