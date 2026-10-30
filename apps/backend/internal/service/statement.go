package service

import (
	"context"
	"log/slog"
	"sort"
	"strings"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/pkg/errs"
	"github.com/edwinhuish/gowork/internal/pkg/money"
	"github.com/edwinhuish/gowork/internal/repository"
)

// StatementService 出客户对账单（T-53）。
//
// 对账单不是"应收列表换个排版"：列表按单看（这一张还欠多少），
// 对账单按时间看（这一段发生了什么、余额怎么变成现在这个数）。
// 每月催款发出去的就是后者。
type StatementService interface {
	Statement(ctx context.Context, q dto.StatementQuery) (*dto.StatementResult, error)
}

type statementService struct {
	entries  repository.StatementRepository
	partners repository.PartnerRepository
	log      *slog.Logger
}

type StatementOption func(*statementService)

func WithStatementLogger(l *slog.Logger) StatementOption {
	return func(s *statementService) { s.log = l }
}

func NewStatementService(
	entries repository.StatementRepository,
	partners repository.PartnerRepository,
	opts ...StatementOption,
) StatementService {
	s := &statementService{entries: entries, partners: partners, log: slog.Default()}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *statementService) Statement(
	ctx context.Context, q dto.StatementQuery,
) (*dto.StatementResult, error) {
	partner, err := s.partners.FindByID(ctx, q.PartnerID)
	if err != nil {
		return nil, err
	}

	var from, to string
	if strings.TrimSpace(q.From) != "" {
		d, err := parseDateOnly(q.From)
		if err != nil {
			return nil, err
		}
		from = d.Format("2006-01-02")
	}
	if strings.TrimSpace(q.To) != "" {
		d, err := parseDateOnly(q.To)
		if err != nil {
			return nil, err
		}
		to = d.Format("2006-01-02")
	}
	if from != "" && to != "" && from > to {
		return nil, errs.BadRequest("账期开始日不能晚于结束日")
	}

	// 一次取全量、在内存里切期初与本期：两次查询要写两套过滤条件，
	// 而"期初"与"本期"必须来自同一份数据，否则边界那一天会被算两次或漏算。
	all, err := s.entries.ListPartnerEntries(ctx, q.PartnerID, nil, nil)
	if err != nil {
		return nil, err
	}

	// 币种：显式传 > 该客户第一个币种 > 空（此时全部当作同一币种，
	// 只有在客户从来没有应收时才会走到这里）。
	currency := strings.TrimSpace(q.Currency)
	if currency == "" {
		codes, err := s.entries.PartnerCurrencies(ctx, q.PartnerID)
		if err != nil {
			return nil, err
		}
		if len(codes) > 0 {
			currency = codes[0]
		}
	}

	out := &dto.StatementResult{
		PartnerID:   partner.ID,
		PartnerName: partner.Name,
		Currency:    currency,
		From:        from,
		To:          to,
		Opening:     money.Zero(),
		Debit:       money.Zero(),
		Credit:      money.Zero(),
		Lines:       make([]dto.StatementLine, 0, len(all)),
	}

	for _, e := range all {
		if currency != "" && e.CurrencyCode != currency {
			// 跨币种客户按币种分别对：把美元和人民币加在一起得到的余额没有意义。
			continue
		}
		day := e.DocDate.Format("2006-01-02")
		signed := signedAmount(e.Kind, e.Amount)

		// 期初：账期开始日之前的全部发生额之和。
		if from != "" && day < from {
			out.Opening = out.Opening.Add(signed)
			continue
		}
		if to != "" && day > to {
			continue
		}

		out.Lines = append(out.Lines, dto.StatementLine{
			Kind:    e.Kind,
			DocNo:   e.DocNo,
			DocDate: day,
			Amount:  e.Amount,
			Signed:  signed,
			Remark:  e.Remark,
		})
		if signed.IsNegative() {
			out.Credit = out.Credit.Add(signed.Abs())
		} else {
			out.Debit = out.Debit.Add(signed)
		}
	}

	out.Opening = out.Opening.Round(4)
	out.Debit = out.Debit.Round(4)
	out.Credit = out.Credit.Round(4)
	out.Closing = out.Opening.Add(out.Debit).Sub(out.Credit).Round(4)

	// 按日期升序；同一天先开票、再贷项、最后收款，单号兜底。
	//
	// 只按单号排的话，同一天的单据谁在前取决于单号首字母（PM- 排在 RC- 之前），
	// 余额列于是先跳到 −400 再回到 600——客户看到"我什么时候欠你负四百"，
	// 然后开始怀疑整张单子。单号兜底只是为了两次调用行序一致：
	// 对账单是会发给客户的，每次打开行序都不一样会让人以为数据在变。
	sort.SliceStable(out.Lines, func(i, j int) bool {
		if out.Lines[i].DocDate != out.Lines[j].DocDate {
			return out.Lines[i].DocDate < out.Lines[j].DocDate
		}
		ri, rj := statementKindRank(out.Lines[i].Kind), statementKindRank(out.Lines[j].Kind)
		if ri != rj {
			return ri < rj
		}
		return out.Lines[i].DocNo < out.Lines[j].DocNo
	})

	s.log.Info("生成客户对账单", "partner_id", partner.ID, "currency", currency,
		"lines", len(out.Lines), "closing", out.Closing)
	return out, nil
}

// statementKindRank 是同日发生额的先后：先算账（开票），再冲减，最后收款。
//
// 这个顺序不是审美问题：余额列是逐行累加的，先收款后开票会让它中途变成负数。
func statementKindRank(kind string) int {
	switch kind {
	case "receivable":
		return 1
	case "credit_note":
		return 2
	case "payment":
		return 3
	default:
		return 4
	}
}

// signedAmount 给一笔发生额定方向：应收为正，贷项与收款为负。
//
// 用符号而不是"正负两列"：期末余额就是逐行累加，客户能照着单子自己验算。
func signedAmount(kind string, amount money.Decimal) money.Decimal {
	switch kind {
	case "receivable":
		return amount
	case "credit_note", "payment":
		return amount.Neg()
	default:
		return amount
	}
}
