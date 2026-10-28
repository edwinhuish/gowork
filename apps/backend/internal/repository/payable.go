package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/errs"
)

// PayableFilter 是应付单的查询条件。
type PayableFilter struct {
	PartnerID *uint
	Status    *model.SettlementStatus
	// OverdueBefore 取到期日早于该日的单，供账龄（T-22）按桶取数。
	OverdueBefore *time.Time
}

func (f PayableFilter) scopes() []Scope {
	scopes := make([]Scope, 0, 3)
	if f.PartnerID != nil {
		pid := *f.PartnerID
		scopes = append(scopes, func(db *gorm.DB) *gorm.DB {
			return db.Where("partner_id = ?", pid)
		})
	}
	if f.Status != nil {
		st := *f.Status
		scopes = append(scopes, func(db *gorm.DB) *gorm.DB {
			return db.Where("status = ?", st)
		})
	}
	if f.OverdueBefore != nil {
		d := *f.OverdueBefore
		scopes = append(scopes, func(db *gorm.DB) *gorm.DB {
			return db.Where("due_date < ?", d)
		})
	}
	return scopes
}

type PayableRepository interface {
	Create(ctx context.Context, m *model.Payable) error
	Update(ctx context.Context, id uint, fields map[string]any) error
	Delete(ctx context.Context, id uint) error
	FindByID(ctx context.Context, id uint) (*model.Payable, error)
	List(ctx context.Context, f PayableFilter, p Pagination) ([]model.Payable, int64, error)
	// SumAmountBySource 汇总某来源已开出的应付金额（原币），规则同应收。
	SumAmountBySource(
		ctx context.Context, sourceType model.SettlementSource, sourceID uint, excludeID uint,
	) (float64, error)
}

type payableRepo struct {
	*Base[model.Payable]
}

func NewPayableRepository(db *gorm.DB) PayableRepository {
	return &payableRepo{Base: NewBase[model.Payable](db)}
}

func (r *payableRepo) Create(ctx context.Context, m *model.Payable) error {
	return r.Base.Create(ctx, m)
}

func (r *payableRepo) Update(ctx context.Context, id uint, fields map[string]any) error {
	return r.Base.UpdateFields(ctx, id, fields)
}

func (r *payableRepo) Delete(ctx context.Context, id uint) error {
	return r.Base.DeleteByID(ctx, id)
}

func (r *payableRepo) FindByID(ctx context.Context, id uint) (*model.Payable, error) {
	var m model.Payable
	err := r.DB().WithContext(ctx).Preload("Partner").First(&m, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errs.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("查询应付单 id=%d: %w", id, err)
	}
	return &m, nil
}

// List 按到期日升序：与应收一致，这张表是"哪天该付钱"的清单。
func (r *payableRepo) List(
	ctx context.Context, f PayableFilter, p Pagination,
) ([]model.Payable, int64, error) {
	scopes := append(f.scopes(), func(db *gorm.DB) *gorm.DB {
		return db.Preload("Partner")
	})
	return r.PaginateOrdered(ctx, p, "due_date ASC, id ASC", scopes...)
}

func (r *payableRepo) SumAmountBySource(
	ctx context.Context, sourceType model.SettlementSource, sourceID uint, excludeID uint,
) (float64, error) {
	var sum float64
	q := r.DB().WithContext(ctx).
		Model(&model.Payable{}).
		Where("source_type = ? AND source_id = ? AND status <> ?",
			sourceType, sourceID, model.SettlementStatusVoid)
	if excludeID != 0 {
		q = q.Where("id <> ?", excludeID)
	}
	if err := q.Select("COALESCE(SUM(amount), 0)").Scan(&sum).Error; err != nil {
		return 0, fmt.Errorf("汇总来源应付 source=%d/%d: %w", sourceType, sourceID, err)
	}
	return sum, nil
}
