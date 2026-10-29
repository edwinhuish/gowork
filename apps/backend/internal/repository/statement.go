package repository

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/money"
)

// StatementEntry 是对账单上的一行原始发生额（未加符号）。
type StatementEntry struct {
	Kind         string
	DocNo        string
	DocDate      time.Time
	Amount       money.Decimal
	CurrencyCode string
	Remark       string
}

// StatementRepository 出客户对账单的原始行（T-53）。
//
// 用一次 UNION 而不是三次查询再合并：三类单据的日期字段不同，
// 在 SQL 里排好序比在 Go 里排更省事，也不会因为漏排而让期初算错。
type StatementRepository interface {
	ListPartnerEntries(
		ctx context.Context, partnerID uint, from, to *time.Time,
	) ([]StatementEntry, error)
	// PartnerCurrencies 列出该客户涉及过的币种，供对账单选默认币种。
	PartnerCurrencies(ctx context.Context, partnerID uint) ([]string, error)
}

type statementRepo struct {
	*Base[model.Receivable]
}

func NewStatementRepository(db *gorm.DB) StatementRepository {
	return &statementRepo{Base: NewBase[model.Receivable](db)}
}

func (r *statementRepo) ListPartnerEntries(
	ctx context.Context, partnerID uint, from, to *time.Time,
) ([]StatementEntry, error) {
	// 应收：开票日（created_at）；贷项：开具日（issued_date）；收款：到账日（pay_date）。
	// 三者的业务含义不同但都是"这笔账发生的那天"，对账单按它排序。
	sql := `
SELECT 'receivable' AS kind, r.doc_no AS doc_no, r.created_at AS doc_date,
       r.amount AS amount, r.currency_code AS currency_code, '' AS remark
  FROM biz_receivable r
 WHERE r.partner_id = ? AND r.deleted_at = 0 AND r.status <> ?
UNION ALL
SELECT 'credit_note', c.doc_no, c.issued_date, c.amount, c.currency_code, c.reason
  FROM biz_credit_note c
  JOIN biz_receivable r2 ON r2.id = c.receivable_id
 WHERE r2.partner_id = ? AND c.deleted_at = 0
UNION ALL
SELECT 'payment', p.doc_no, p.pay_date, p.amount, p.currency_code, p.remark
  FROM biz_payment p
 WHERE p.partner_id = ? AND p.deleted_at = 0 AND p.direction = ?`

	args := []any{
		partnerID, model.SettlementStatusVoid,
		partnerID,
		partnerID, model.PaymentDirectionIn,
	}
	if from != nil {
		sql = "SELECT * FROM (" + sql + ") t WHERE t.doc_date >= ?"
		args = append(args, *from)
	}
	if to != nil {
		// 第二层包一次，避免两个时间条件互相覆盖 WHERE 里的第一个。
		sql = "SELECT * FROM (" + sql + ") t2 WHERE t2.doc_date < ?"
		args = append(args, to.AddDate(0, 0, 1))
	}
	sql += " ORDER BY doc_date ASC"

	rows := make([]StatementEntry, 0, 64)
	if err := r.conn(ctx).Raw(sql, args...).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("查询主体 %d 的对账明细: %w", partnerID, err)
	}
	return rows, nil
}

func (r *statementRepo) PartnerCurrencies(
	ctx context.Context, partnerID uint,
) ([]string, error) {
	var out []string
	err := r.conn(ctx).Model(&model.Receivable{}).
		Where("partner_id = ? AND deleted_at = 0", partnerID).
		Distinct().Order("currency_code ASC").
		Pluck("currency_code", &out).Error
	if err != nil {
		return nil, fmt.Errorf("查询主体 %d 的币种: %w", partnerID, err)
	}
	return out, nil
}
