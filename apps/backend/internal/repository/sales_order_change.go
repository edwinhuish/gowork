package repository

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/errs"
)

// ItemPatch 是对一条订单明细的原地更新。
//
// 为什么是"原地更新"而不是像编辑草稿那样删旧行重建：
// 出货分配与以销定采的挂接都按明细行 id 引用，重建会让这些引用指向不存在的行，
// 表现是"这票货出的行突然从单子上消失了"。草稿可以重建（那时还没有引用），
// 已确认的订单不行。
type ItemPatch struct {
	ItemID uint
	Fields map[string]any
}

// SalesOrderChangeRepository 是订单变更记录的仓储。
type SalesOrderChangeRepository interface {
	// NextSeq 取这张订单的下一个变更序号。
	//
	// 用 MAX(seq)+1 而不是计数：变更记录不会被删，但即使将来允许删，
	// 序号也不该回退——回退会让"第 2 次变更"指向两条不同的记录。
	NextSeq(ctx context.Context, salesOrderID uint) (uint, error)
	// Apply 在一个事务里写变更单、变更明细、改订单行、更新订单头。
	//
	// 四件事必须同生共死：只写了变更单没改订单，历史与实际不符；
	// 只改了订单没写变更单，就是"偷偷改了承诺"。
	Apply(
		ctx context.Context,
		change *model.SalesOrderChange,
		lines []model.SalesOrderChangeLine,
		patches []ItemPatch,
		orderFields map[string]any,
	) error
	// ListByOrder 按序号倒序返回这张订单的变更历史。
	ListByOrder(ctx context.Context, salesOrderID uint) ([]model.SalesOrderChange, error)
}

type salesOrderChangeRepo struct {
	db *gorm.DB
}

func NewSalesOrderChangeRepository(db *gorm.DB) SalesOrderChangeRepository {
	return &salesOrderChangeRepo{db: db}
}

func (r *salesOrderChangeRepo) NextSeq(ctx context.Context, salesOrderID uint) (uint, error) {
	var maxSeq uint
	err := dbFor(ctx, r.db).Unscoped().
		Model(&model.SalesOrderChange{}).
		Where("sales_order_id = ?", salesOrderID).
		Select("COALESCE(MAX(seq), 0)").
		Scan(&maxSeq).Error
	if err != nil {
		return 0, fmt.Errorf("查询订单 %d 的变更序号: %w", salesOrderID, err)
	}
	return maxSeq + 1, nil
}

func (r *salesOrderChangeRepo) Apply(
	ctx context.Context,
	change *model.SalesOrderChange,
	lines []model.SalesOrderChangeLine,
	patches []ItemPatch,
	orderFields map[string]any,
) error {
	return dbFor(ctx, r.db).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(change).Error; err != nil {
			return fmt.Errorf("写入变更单: %w", err)
		}
		for i := range lines {
			lines[i].ChangeID = change.ID
		}
		if len(lines) > 0 {
			if err := tx.Create(&lines).Error; err != nil {
				return fmt.Errorf("写入变更明细: %w", err)
			}
		}

		for _, p := range patches {
			res := tx.Model(&model.SalesOrderItem{}).
				Where("id = ? AND sales_order_id = ?", p.ItemID, change.SalesOrderID).
				Updates(p.Fields)
			if res.Error != nil {
				return fmt.Errorf("更新订单行 %d: %w", p.ItemID, res.Error)
			}
			if res.RowsAffected == 0 {
				// 行不属于这张订单，或已不存在。两种都说明这份变更的输入是过期的。
				return errs.ErrConflict.WithMsg(
					"订单明细已被改动，请刷新后重新提交变更")
			}
		}

		q := tx.Model(&model.SalesOrder{}).Where("id = ?", change.SalesOrderID)
		expected, hasLock := takeOptimisticLock(orderFields)
		if hasLock {
			q = q.Where("updated_at = ?", expected)
		}
		res := q.Updates(orderFields)
		if res.Error != nil {
			return fmt.Errorf("更新订单头: %w", res.Error)
		}
		if res.RowsAffected == 0 {
			if hasLock {
				return errs.ErrConflict.WithMsg("这张订单已被他人修改，请刷新后重试")
			}
			return errs.ErrNotFound
		}
		return nil
	})
}

func (r *salesOrderChangeRepo) ListByOrder(
	ctx context.Context, salesOrderID uint,
) ([]model.SalesOrderChange, error) {
	var rows []model.SalesOrderChange
	err := dbFor(ctx, r.db).
		Preload("Lines", func(db *gorm.DB) *gorm.DB { return db.Order("id ASC") }).
		Where("sales_order_id = ?", salesOrderID).
		Order("seq DESC").
		Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("查询订单 %d 的变更历史: %w", salesOrderID, err)
	}
	return rows, nil
}

var _ = errors.Is
