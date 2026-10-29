package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/errs"
	"github.com/edwinhuish/gowork/internal/pkg/money"
)

// ReceivableFilter 是应收单的查询条件。
type ReceivableFilter struct {
	PartnerID  *uint
	SourceType *model.SettlementSource
	SourceID   *uint
	Status     *model.SettlementStatus
	// OverdueBefore 取到期日早于该日的单，供账龄（T-22）按桶取数。
	// 用"早于"而不是"等于"：账龄关心的是"已经到期多久"。
	OverdueBefore *time.Time
	// OwnerID 只看归属该业务员的客户的应收（含未分配客户的）。
	OwnerID *uint
}

func (f ReceivableFilter) scopes() []Scope {
	scopes := make([]Scope, 0, 5)
	if f.PartnerID != nil {
		pid := *f.PartnerID
		scopes = append(scopes, func(db *gorm.DB) *gorm.DB {
			return db.Where("partner_id = ?", pid)
		})
	}
	if f.SourceType != nil {
		st := *f.SourceType
		scopes = append(scopes, func(db *gorm.DB) *gorm.DB {
			return db.Where("source_type = ?", st)
		})
	}
	if f.SourceID != nil {
		sid := *f.SourceID
		scopes = append(scopes, func(db *gorm.DB) *gorm.DB {
			return db.Where("source_id = ?", sid)
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
	if f.OwnerID != nil {
		scopes = append(scopes, OwnerScope("partner_id", *f.OwnerID))
	}
	return scopes
}

type ReceivableRepository interface {
	Create(ctx context.Context, m *model.Receivable) error
	Update(ctx context.Context, id uint, fields map[string]any) error
	Delete(ctx context.Context, id uint) error
	FindByID(ctx context.Context, id uint) (*model.Receivable, error)
	List(ctx context.Context, f ReceivableFilter, p Pagination) ([]model.Receivable, int64, error)
	// SumAmountBySource 汇总某来源已开出的应收金额（原币）。
	//
	// 创建与改金额时用它卡"累计不得超过来源总额"。excludeID 传当前正在改的那张，
	// 否则改金额时会把它自己的旧值也算进去，表现为"总是超额"。
	// 作废的单不计入：作废的意思是这笔钱不收了，它还占着额度就永远开不出新单。
	SumAmountBySource(
		ctx context.Context, sourceType model.SettlementSource, sourceID uint, excludeID uint,
	) (money.Decimal, error)
	// Settlements 批量取几张应收单的收款现状，key 是应收单 id（T-61）。
	//
	// 合同分期要显示「已收 / 未收」，而这些数只有应收单知道：核销额在单上，
	// 贷项在另一张表。批量取而不是逐张查——一份合同三五期，逐张就是五个来回，
	// 而详情页每次打开都要算一遍。
	Settlements(ctx context.Context, ids []uint) (map[uint]ReceivableSettlement, error)
}

// ReceivableSettlement 是一张应收单的收款现状。
//
// Credited 是已开贷项：退货冲掉的钱不会再收，所以「未收」要把它减掉——
// 与核销校验、信用敞口、账龄、回款率、客户 360 是同一个口径。
type ReceivableSettlement struct {
	DocNo    string
	Amount   money.Decimal
	Settled  money.Decimal
	Credited money.Decimal
}

type receivableRepo struct {
	*Base[model.Receivable]
}

func NewReceivableRepository(db *gorm.DB) ReceivableRepository {
	return &receivableRepo{Base: NewBase[model.Receivable](db)}
}

func (r *receivableRepo) Create(ctx context.Context, m *model.Receivable) error {
	return r.Base.Create(ctx, m)
}

func (r *receivableRepo) Update(ctx context.Context, id uint, fields map[string]any) error {
	return r.Base.UpdateFields(ctx, id, fields)
}

func (r *receivableRepo) Delete(ctx context.Context, id uint) error {
	return r.Base.DeleteByID(ctx, id)
}

func (r *receivableRepo) FindByID(ctx context.Context, id uint) (*model.Receivable, error) {
	var m model.Receivable
	err := r.conn(ctx).Preload("Partner").First(&m, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errs.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("查询应收单 id=%d: %w", id, err)
	}
	return &m, nil
}

// List 按到期日升序：这张表的读法是"哪笔先到期"，不是"我刚录了什么"。
func (r *receivableRepo) List(
	ctx context.Context, f ReceivableFilter, p Pagination,
) ([]model.Receivable, int64, error) {
	scopes := append(f.scopes(), func(db *gorm.DB) *gorm.DB {
		return db.Preload("Partner")
	})
	return r.PaginateOrdered(ctx, p, "due_date ASC, id ASC", scopes...)
}

func (r *receivableRepo) SumAmountBySource(
	ctx context.Context, sourceType model.SettlementSource, sourceID uint, excludeID uint,
) (money.Decimal, error) {
	// 扫进结构体而不是标量：SUM 出来的列要落到 money.Decimal 上，
	// 它靠 Scan 实现收数，走 gorm 的字段映射比裸标量稳。
	var row struct{ Total money.Decimal }
	q := r.conn(ctx).
		Model(&model.Receivable{}).
		Where("source_type = ? AND source_id = ? AND status <> ?",
			sourceType, sourceID, model.SettlementStatusVoid)
	if excludeID != 0 {
		q = q.Where("id <> ?", excludeID)
	}
	if err := q.Select("COALESCE(SUM(amount), 0) AS total").Scan(&row).Error; err != nil {
		return money.Zero(), fmt.Errorf("汇总来源应收 source=%d/%d: %w", sourceType, sourceID, err)
	}
	return row.Total, nil
}

// Settlements 批量取应收单的收款现状（T-61）。
//
// 贷项用子查询取，不另发一条语句再在 Go 里拼：贷项表按 receivable_id 建过索引，
// 一次查询就能把「开了多少票、收了多少、退了多少」三件数一起带回来。
func (r *receivableRepo) Settlements(
	ctx context.Context, ids []uint,
) (map[uint]ReceivableSettlement, error) {
	out := make(map[uint]ReceivableSettlement, len(ids))
	if len(ids) == 0 {
		return out, nil
	}

	var rows []struct {
		ID       uint
		DocNo    string
		Amount   money.Decimal
		Settled  money.Decimal
		Credited money.Decimal
	}
	err := r.conn(ctx).Model(&model.Receivable{}).
		Select("id, doc_no, amount, settled_amount AS settled, "+
			creditSubquery("biz_receivable")+" AS credited").
		Where("id IN ?", ids).
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("查询应收收款现状: %w", err)
	}
	for _, row := range rows {
		out[row.ID] = ReceivableSettlement{
			DocNo:    row.DocNo,
			Amount:   row.Amount,
			Settled:  row.Settled,
			Credited: row.Credited,
		}
	}
	return out, nil
}
