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

// CreditNoteService 管贷项通知单（T-52b）。
//
// 它记录"这张发票上的这一部分客户不用付了"：退货、折让、短装。
// 在此之前只能把原应收作废重开——单号变了，客户手上那份对不上。
type CreditNoteService interface {
	List(ctx context.Context, q dto.CreditNoteQuery) (*dto.PageResult[model.CreditNote], error)
	Create(ctx context.Context, in dto.CreditNoteInput) (*model.CreditNote, error)
}

type creditNoteService struct {
	notes       repository.CreditNoteRepository
	receivables repository.ReceivableRepository
	docNos      DocNoService
	log         *slog.Logger
}

type CreditNoteOption func(*creditNoteService)

func WithCreditNoteLogger(l *slog.Logger) CreditNoteOption {
	return func(s *creditNoteService) { s.log = l }
}

func NewCreditNoteService(
	notes repository.CreditNoteRepository,
	receivables repository.ReceivableRepository,
	docNos DocNoService,
	opts ...CreditNoteOption,
) CreditNoteService {
	s := &creditNoteService{notes: notes, receivables: receivables, docNos: docNos, log: slog.Default()}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *creditNoteService) List(
	ctx context.Context, q dto.CreditNoteQuery,
) (*dto.PageResult[model.CreditNote], error) {
	q.PageQuery.Normalize()
	list, total, err := s.notes.List(ctx, q)
	if err != nil {
		return nil, err
	}
	return dto.NewPageResult(list, total, q.Page, q.PageSize), nil
}

// Create 开一张贷项。
//
// 上限是「未结清额」而不是「开票额」：已经收到的钱只能退款，不能靠贷项抹掉。
// 允许冲到超过未结清额，等于让一张单出现负余额，而账龄分桶是按剩余额切的，
// 负余额会落进"未到期"那一桶里，看起来像还有钱要收。
func (s *creditNoteService) Create(
	ctx context.Context, in dto.CreditNoteInput,
) (*model.CreditNote, error) {
	if !in.Amount.IsPositive() {
		return nil, errs.BadRequest("冲减金额必须大于 0")
	}
	m, err := s.receivables.FindByID(ctx, in.ReceivableID)
	if err != nil {
		return nil, err
	}
	if m.Status == model.SettlementStatusVoid {
		return nil, errs.ErrConflict.WithMsg(m.DocNo + " 已作废，不能再开贷项")
	}

	credited, err := s.notes.SumByReceivable(ctx, m.ID)
	if err != nil {
		return nil, err
	}
	available := m.Amount.Sub(m.SettledAmount).Sub(credited)
	if in.Amount.Cmp(available) > 0 {
		return nil, errs.ErrConflict.WithMsg(
			"冲减金额超过未结清额：应收 " + m.DocNo + " 未结清 " + available.String() +
				" " + m.CurrencyCode + "，本次 " + in.Amount.String())
	}

	issuedAt, err := s.issuedDate(in.IssuedDate)
	if err != nil {
		return nil, err
	}
	docNo, err := s.docNos.Next(ctx, model.DocTypeCreditNote)
	if err != nil {
		return nil, err
	}

	note := &model.CreditNote{
		DocNo:        docNo,
		ReceivableID: m.ID,
		Amount:       in.Amount,
		// 币种与汇率随原单：贷项不是一笔新的汇兑事件，它只是原发票的减项。
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
	s.log.Info("新增贷项单", "id", note.ID, "doc_no", note.DocNo,
		"receivable", m.DocNo, "amount", note.Amount)
	return note, nil
}

func (s *creditNoteService) issuedDate(raw string) (time.Time, error) {
	if strings.TrimSpace(raw) == "" {
		return model.DateOnly(time.Now()), nil
	}
	return parseDateOnly(raw)
}
