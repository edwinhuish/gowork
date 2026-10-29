package service

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/errs"
	"github.com/edwinhuish/gowork/internal/repository"
)

// DebitNoteService 管借项通知单（T-52b3）。
//
// 与贷项对称：贷项冲客户的应收，借项冲供应商的应付。采购退货减了库存
// 却减不了应付时，账上永远显示还欠供应商这批货的钱，而仓库里货已经退回去了。
type DebitNoteService interface {
	List(ctx context.Context, q dto.DebitNoteQuery) (*dto.PageResult[model.DebitNote], error)
	Create(ctx context.Context, in dto.DebitNoteInput) (*model.DebitNote, error)
}

type debitNoteService struct {
	notes    repository.DebitNoteRepository
	payables repository.PayableRepository
	docNos   DocNoService
	log      *slog.Logger
}

type DebitNoteOption func(*debitNoteService)

func WithDebitNoteLogger(l *slog.Logger) DebitNoteOption {
	return func(s *debitNoteService) { s.log = l }
}

func NewDebitNoteService(
	notes repository.DebitNoteRepository,
	payables repository.PayableRepository,
	docNos DocNoService,
	opts ...DebitNoteOption,
) DebitNoteService {
	s := &debitNoteService{notes: notes, payables: payables, docNos: docNos, log: slog.Default()}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *debitNoteService) List(
	ctx context.Context, q dto.DebitNoteQuery,
) (*dto.PageResult[model.DebitNote], error) {
	q.PageQuery.Normalize()
	list, total, err := s.notes.List(ctx, q)
	if err != nil {
		return nil, err
	}
	return dto.NewPageResult(list, total, q.Page, q.PageSize), nil
}

// Create 开一张借项。
//
// 上限是「未结清额」而不是「应付额」：已经付出去的钱只能走退款，
// 不能靠借项抹掉。
func (s *debitNoteService) Create(
	ctx context.Context, in dto.DebitNoteInput,
) (*model.DebitNote, error) {
	if !in.Amount.IsPositive() {
		return nil, errs.BadRequest("冲减金额必须大于 0")
	}
	m, err := s.payables.FindByID(ctx, in.PayableID)
	if err != nil {
		return nil, err
	}
	if m.Status == model.SettlementStatusVoid {
		return nil, errs.ErrConflict.WithMsg(m.DocNo + " 已作废，不能再开借项")
	}

	debited, err := s.notes.SumByPayable(ctx, m.ID)
	if err != nil {
		return nil, err
	}
	available := m.Amount.Sub(m.SettledAmount).Sub(debited)
	if in.Amount.Cmp(available) > 0 {
		return nil, errs.ErrConflict.WithMsg(
			"冲减金额超过未结清额：应付 " + m.DocNo + " 未结清 " + available.String() +
				" " + m.CurrencyCode + "，本次 " + in.Amount.String())
	}

	issuedAt := model.DateOnly(time.Now())
	if strings.TrimSpace(in.IssuedDate) != "" {
		d, err := parseDateOnly(in.IssuedDate)
		if err != nil {
			return nil, err
		}
		issuedAt = d
	}
	docNo, err := s.docNos.Next(ctx, model.DocTypeDebitNote)
	if err != nil {
		return nil, err
	}

	note := &model.DebitNote{
		DocNo:        docNo,
		PayableID:    m.ID,
		Amount:       in.Amount,
		CurrencyCode: m.CurrencyCode,
		ExchangeRate: m.ExchangeRate,
		AmountBase:   in.Amount.Mul(m.ExchangeRate).Round(4),
		IssuedDate:   issuedAt,
		Reason:       strings.TrimSpace(in.Reason),
		Remark:       strings.TrimSpace(in.Remark),
	}
	if err := s.notes.Create(ctx, note); err != nil {
		return nil, err
	}
	s.log.Info("新增借项单", "id", note.ID, "doc_no", note.DocNo,
		"payable", m.DocNo, "amount", note.Amount)
	return note, nil
}
