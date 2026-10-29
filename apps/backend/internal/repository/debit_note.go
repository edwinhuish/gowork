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

// DebitNoteRepository 读写借项通知单（T-52b3）。
type DebitNoteRepository interface {
	List(ctx context.Context, q dto.DebitNoteQuery) ([]model.DebitNote, int64, error)
	FindByID(ctx context.Context, id uint) (*model.DebitNote, error)
	Create(ctx context.Context, m *model.DebitNote) error
	// SumByPayable 汇总某张应付单已被冲减的金额。
	SumByPayable(ctx context.Context, payableID uint) (money.Decimal, error)
}

type debitNoteRepo struct {
	*Base[model.DebitNote]
}

func NewDebitNoteRepository(db *gorm.DB) DebitNoteRepository {
	return &debitNoteRepo{Base: NewBase[model.DebitNote](db)}
}

func (r *debitNoteRepo) List(
	ctx context.Context, q dto.DebitNoteQuery,
) ([]model.DebitNote, int64, error) {
	scopes := make([]Scope, 0, 3)
	if q.PayableID != nil {
		scopes = append(scopes, func(db *gorm.DB) *gorm.DB {
			return db.Where("payable_id = ?", *q.PayableID)
		})
	}
	if q.PartnerID != nil {
		scopes = append(scopes, func(db *gorm.DB) *gorm.DB {
			return db.Where("payable_id IN (?)",
				db.Model(&model.Payable{}).Select("id").Where("partner_id = ?", *q.PartnerID))
		})
	}
	return r.PaginateOrdered(ctx, Pagination{Page: q.Page, PageSize: q.PageSize},
		"issued_date DESC, id DESC", scopes...)
}

func (r *debitNoteRepo) FindByID(ctx context.Context, id uint) (*model.DebitNote, error) {
	var m model.DebitNote
	err := r.conn(ctx).First(&m, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errs.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("查询借项单 %d: %w", id, err)
	}
	return &m, nil
}

func (r *debitNoteRepo) Create(ctx context.Context, m *model.DebitNote) error {
	return r.Base.Create(ctx, m)
}

func (r *debitNoteRepo) SumByPayable(
	ctx context.Context, payableID uint,
) (money.Decimal, error) {
	var rows []model.DebitNote
	err := r.conn(ctx).Select("amount").Where("payable_id = ?", payableID).Find(&rows).Error
	if err != nil {
		return money.Zero(), fmt.Errorf("汇总应付单 %d 的借项: %w", payableID, err)
	}
	sum := money.Zero()
	for _, row := range rows {
		sum = sum.Add(row.Amount)
	}
	return sum, nil
}
