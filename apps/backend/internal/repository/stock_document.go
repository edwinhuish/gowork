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

// StockDocumentFilter 是出入库单的查询条件。
type StockDocumentFilter struct {
	Keyword   string
	Type      *model.StockDocType
	Status    *model.DocumentStatus
	PartnerID *uint
}

func (f StockDocumentFilter) scopes() []Scope {
	scopes := make([]Scope, 0, 4)
	if f.Keyword != "" {
		scopes = append(scopes, likeAny(f.Keyword, "doc_no", "source_doc_no", "remark"))
	}
	if f.Type != nil {
		t := *f.Type
		scopes = append(scopes, func(db *gorm.DB) *gorm.DB { return db.Where("type = ?", t) })
	}
	if f.Status != nil {
		st := *f.Status
		scopes = append(scopes, func(db *gorm.DB) *gorm.DB { return db.Where("status = ?", st) })
	}
	if f.PartnerID != nil {
		pid := *f.PartnerID
		scopes = append(scopes, func(db *gorm.DB) *gorm.DB { return db.Where("partner_id = ?", pid) })
	}
	return scopes
}

// StockDocumentRepository 管出入库单与它的明细。
type StockDocumentRepository interface {
	List(ctx context.Context, f StockDocumentFilter, p Pagination) ([]model.StockDocument, int64, error)
	FindByID(ctx context.Context, id uint) (*model.StockDocument, error)
	CreateWithItems(ctx context.Context, doc *model.StockDocument, items []model.StockDocumentItem) error
	UpdateWithItems(ctx context.Context, id uint, fields map[string]any, items []model.StockDocumentItem) error
	Update(ctx context.Context, id uint, fields map[string]any) error
	Delete(ctx context.Context, id uint) error
	// CurrentQty 返回某个产品当前的库存（没有快照视为 0），盘点确认时用来取账面数。
	CurrentQty(ctx context.Context, productID uint) (money.Decimal, error)
	// ExistsBySource 报告某类单据是否已经由这个来源单号生成过。
	// 出货确认据此挡住重复出库（T-40）：同一票货只能出一张销售出库单。
	ExistsBySource(ctx context.Context, t model.StockDocType, sourceDocNo string) (bool, error)
	// ExistsReversal 判断这张单是否已经被红冲过（T-52c）。一张单只允许红冲一次：
	// 冲两次等于把库存改两遍，而对账时找不到依据。
	ExistsReversal(ctx context.Context, id uint) (bool, error)
	// Post 把单据置为已确认、回写账面数、写入流水并更新快照，全部在一个事务里。
	//
	// 拆成多次调用就会出现"单据已确认但库存没动"的中间态，
	// 而这类中间态只能靠人工对账发现。
	Post(ctx context.Context, id uint, postedAt time.Time, items []model.StockDocumentItem,
		movements []model.StockMovement) error
}

type stockDocumentRepo struct {
	*Base[model.StockDocument]
}

func NewStockDocumentRepository(db *gorm.DB) StockDocumentRepository {
	return &stockDocumentRepo{Base: NewBase[model.StockDocument](db)}
}

func preloadItems(db *gorm.DB) *gorm.DB {
	return db.Order("id ASC")
}

func (r *stockDocumentRepo) List(
	ctx context.Context, f StockDocumentFilter, p Pagination,
) ([]model.StockDocument, int64, error) {
	return r.PaginateOrderedWithPreload(
		ctx, p, "doc_date DESC, id DESC",
		[]string{"Items", "Partner"}, f.scopes()...)
}

func (r *stockDocumentRepo) FindByID(ctx context.Context, id uint) (*model.StockDocument, error) {
	var d model.StockDocument
	err := r.conn(ctx).
		Preload("Partner").
		Preload("Items", preloadItems).
		First(&d, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errs.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("查询出入库单 id=%d: %w", id, err)
	}
	return &d, nil
}

func (r *stockDocumentRepo) CreateWithItems(
	ctx context.Context, doc *model.StockDocument, items []model.StockDocumentItem,
) error {
	return r.conn(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(doc).Error; err != nil {
			return fmt.Errorf("写入出入库单: %w", err)
		}
		for i := range items {
			items[i].DocumentID = doc.ID
		}
		if len(items) > 0 {
			if err := tx.Create(&items).Error; err != nil {
				return fmt.Errorf("写入出入库明细: %w", err)
			}
		}
		return nil
	})
}

func (r *stockDocumentRepo) UpdateWithItems(
	ctx context.Context, id uint, fields map[string]any, items []model.StockDocumentItem,
) error {
	return r.conn(ctx).Transaction(func(tx *gorm.DB) error {
		if len(fields) > 0 {
			q := tx.Model(&model.StockDocument{}).Where("id = ?", id)
			// 与 Base.UpdateFields 同一套乐观锁：明细整体替换之后无从恢复。
			expected, hasLock := takeOptimisticLock(fields)
			if hasLock {
				q = q.Where("updated_at = ?", expected)
			}
			res := q.Updates(fields)
			if res.Error != nil {
				return fmt.Errorf("更新出入库单 id=%d: %w", id, res.Error)
			}
			if res.RowsAffected == 0 {
				if hasLock {
					return errs.ErrConflict.WithMsg("这条记录已被他人修改，请刷新后重试")
				}
				return errs.ErrNotFound
			}
		}
		// 明细整组替换：草稿阶段改几行的成本远低于维护增量 diff，
		// 而"哪一行是哪一行"对使用者没有意义。
		if err := tx.Where("document_id = ?", id).Delete(&model.StockDocumentItem{}).Error; err != nil {
			return fmt.Errorf("清理出入库明细: %w", err)
		}
		for i := range items {
			items[i].DocumentID = id
		}
		if len(items) > 0 {
			if err := tx.Create(&items).Error; err != nil {
				return fmt.Errorf("写入出入库明细: %w", err)
			}
		}
		return nil
	})
}

func (r *stockDocumentRepo) Update(ctx context.Context, id uint, fields map[string]any) error {
	return r.UpdateFields(ctx, id, fields)
}

func (r *stockDocumentRepo) Delete(ctx context.Context, id uint) error {
	return r.conn(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Delete(&model.StockDocument{}, id)
		if res.Error != nil {
			return fmt.Errorf("删除出入库单 id=%d: %w", id, res.Error)
		}
		if res.RowsAffected == 0 {
			return errs.ErrNotFound
		}
		// 明细跟单据一起走：留着孤儿明细，将来按产品统计时会把已删单据的行算进来。
		if err := tx.Where("document_id = ?", id).Delete(&model.StockDocumentItem{}).Error; err != nil {
			return fmt.Errorf("删除出入库明细: %w", err)
		}
		return nil
	})
}

func (r *stockDocumentRepo) CurrentQty(
	ctx context.Context, productID uint,
) (money.Decimal, error) {
	var row struct {
		Qty money.Decimal
	}
	err := r.conn(ctx).
		Table("inv_stock_snapshot").
		Select("qty").
		Where("product_id = ? AND deleted_at = 0", productID).
		Limit(1).
		Scan(&row).Error
	if err != nil {
		return money.Zero(), fmt.Errorf("查询当前库存 product_id=%d: %w", productID, err)
	}
	return row.Qty, nil
}

func (r *stockDocumentRepo) ExistsBySource(
	ctx context.Context, t model.StockDocType, sourceDocNo string,
) (bool, error) {
	if sourceDocNo == "" {
		return false, nil
	}
	var count int64
	err := r.conn(ctx).Model(&model.StockDocument{}).
		Where("type = ? AND source_doc_no = ?", t, sourceDocNo).
		Count(&count).Error
	if err != nil {
		return false, fmt.Errorf("检查来源单号 %s 是否已出库: %w", sourceDocNo, err)
	}
	return count > 0, nil
}

func (r *stockDocumentRepo) Post(
	ctx context.Context, id uint, postedAt time.Time, items []model.StockDocumentItem,
	movements []model.StockMovement,
) error {
	return r.conn(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&model.StockDocument{}).Where("id = ?", id).Updates(map[string]any{
			"status":    model.DocumentStatusConfirmed,
			"posted_at": postedAt,
		})
		if res.Error != nil {
			return fmt.Errorf("确认出入库单 id=%d: %w", id, res.Error)
		}
		if res.RowsAffected == 0 {
			return errs.ErrNotFound
		}

		// 回写账面数（只有盘点单有）：确认之后再看这张单，
		// 必须能回答"当时账上是多少、实盘是多少"，否则差异没有依据。
		for _, it := range items {
			if it.BookQty == nil {
				continue
			}
			err := tx.Model(&model.StockDocumentItem{}).
				Where("id = ?", it.ID).
				Update("book_qty", *it.BookQty).Error
			if err != nil {
				return fmt.Errorf("回写盘点账面数 id=%d: %w", it.ID, err)
			}
		}

		if len(movements) > 0 {
			// 仓库先归一化再写库：流水落 0、快照落默认仓的话，
			// 一次全量重算就会得出与增量不同的数（T-56）。
			for i := range movements {
				normalizeWarehouse(tx, &movements[i])
			}
			if err := tx.Create(&movements).Error; err != nil {
				return fmt.Errorf("写入库存流水: %w", err)
			}
			for _, m := range movements {
				if err := addToSnapshot(tx, m.WarehouseID, m.ProductID, m.SignedQty(), m.ID, m.OccurredAt); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

func (r *stockDocumentRepo) ExistsReversal(ctx context.Context, id uint) (bool, error) {
	var count int64
	err := r.conn(ctx).Model(&model.StockDocument{}).
		Where("reversal_of_id = ?", id).
		Count(&count).Error
	if err != nil {
		return false, fmt.Errorf("检查单据 %d 是否已被红冲: %w", id, err)
	}
	return count > 0, nil
}
