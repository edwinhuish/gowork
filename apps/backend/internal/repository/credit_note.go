package repository

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/errs"
	"github.com/edwinhuish/gowork/internal/pkg/money"
)

// CreditNoteRepository 读写贷项通知单（T-52b）。
type CreditNoteRepository interface {
	List(ctx context.Context, q dto.CreditNoteQuery) ([]model.CreditNote, int64, error)
	FindByID(ctx context.Context, id uint) (*model.CreditNote, error)
	Create(ctx context.Context, m *model.CreditNote) error
	// SumByReceivable 汇总某张应收单已被冲减的金额。
	//
	// 不走 SQL 的 SUM：那个结果要扫进 money.Decimal，而聚合列的类型推断
	// 在三库上并不一致。一张应收单的贷项不会有几百条，取回来自己加更稳。
	SumByReceivable(ctx context.Context, receivableID uint) (money.Decimal, error)
}

type creditNoteRepo struct {
	*Base[model.CreditNote]
}

func NewCreditNoteRepository(db *gorm.DB) CreditNoteRepository {
	return &creditNoteRepo{Base: NewBase[model.CreditNote](db)}
}

func (r *creditNoteRepo) List(
	ctx context.Context, q dto.CreditNoteQuery,
) ([]model.CreditNote, int64, error) {
	scopes := make([]Scope, 0, 3)
	if q.ReceivableID != nil {
		scopes = append(scopes, func(db *gorm.DB) *gorm.DB {
			return db.Where("receivable_id = ?", *q.ReceivableID)
		})
	}
	if q.PartnerID != nil {
		// 主体不在贷项表上：经应收单间接关联。用子查询而不是 join，
		// 免得同一张贷项因为 join 出多条。
		scopes = append(scopes, func(db *gorm.DB) *gorm.DB {
			return db.Where("receivable_id IN (?)",
				db.Model(&model.Receivable{}).Select("id").Where("partner_id = ?", *q.PartnerID))
		})
	}
	return r.PaginateOrdered(ctx, Pagination{Page: q.Page, PageSize: q.PageSize},
		"issued_date DESC, id DESC", scopes...)
}

func (r *creditNoteRepo) FindByID(ctx context.Context, id uint) (*model.CreditNote, error) {
	var m model.CreditNote
	err := r.conn(ctx).First(&m, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errs.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("查询贷项单 %d: %w", id, err)
	}
	return &m, nil
}

func (r *creditNoteRepo) Create(ctx context.Context, m *model.CreditNote) error {
	return r.Base.Create(ctx, m)
}

func (r *creditNoteRepo) SumByReceivable(
	ctx context.Context, receivableID uint,
) (money.Decimal, error) {
	var rows []model.CreditNote
	err := r.conn(ctx).Select("amount").Where("receivable_id = ?", receivableID).Find(&rows).Error
	if err != nil {
		return money.Zero(), fmt.Errorf("汇总应收单 %d 的贷项: %w", receivableID, err)
	}
	sum := money.Zero()
	for _, row := range rows {
		sum = sum.Add(row.Amount)
	}
	return sum, nil
}
