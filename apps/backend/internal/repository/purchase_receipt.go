package repository

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/errs"
	"github.com/edwinhuish/gowork/internal/pkg/money"
)

// PurchaseReceiptRepository 读写采购收货单（T-51a）。
//
// 不提供「按行汇总已收量」的 SQL：那要 raw query + SUM，而 SUM 的结果要
// 扫进 money.Decimal。宁可把一张采购单的收货单连明细一起取回来，
// 在 service 里逐行相加——一张采购单不会有几百张收货单。
type PurchaseReceiptRepository interface {
	ListByOrder(ctx context.Context, orderID uint) ([]model.PurchaseReceipt, error)
	FindByID(ctx context.Context, id uint) (*model.PurchaseReceipt, error)
	CreateWithItems(ctx context.Context, m *model.PurchaseReceipt, items []model.PurchaseReceiptItem) error
	UpdateFields(ctx context.Context, id uint, fields map[string]any) error
	DeleteByID(ctx context.Context, id uint) error
	// AddReturnedQty 累加某收货行的已退数量（T-52b3）。
	//
	// 用原子自增而不是"读出来加再写回"：两次退货并发时后者会丢一次，
	// 而丢掉的那次正是让累计退货超过收货量的那一次。
	AddReturnedQty(ctx context.Context, itemID uint, qty money.Decimal) error
}

type purchaseReceiptRepo struct {
	*Base[model.PurchaseReceipt]
}

func NewPurchaseReceiptRepository(db *gorm.DB) PurchaseReceiptRepository {
	return &purchaseReceiptRepo{Base: NewBase[model.PurchaseReceipt](db)}
}

func (r *purchaseReceiptRepo) ListByOrder(
	ctx context.Context, orderID uint,
) ([]model.PurchaseReceipt, error) {
	rows := make([]model.PurchaseReceipt, 0, 8)
	err := r.conn(ctx).
		Preload("Items", func(db *gorm.DB) *gorm.DB { return db.Order("id ASC") }).
		Where("purchase_order_id = ?", orderID).
		Order("id DESC").
		Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("查询采购单 %d 的收货单: %w", orderID, err)
	}
	return rows, nil
}

func (r *purchaseReceiptRepo) FindByID(
	ctx context.Context, id uint,
) (*model.PurchaseReceipt, error) {
	var m model.PurchaseReceipt
	err := r.conn(ctx).
		Preload("Items", func(db *gorm.DB) *gorm.DB { return db.Order("id ASC") }).
		First(&m, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errs.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("查询收货单 %d: %w", id, err)
	}
	return &m, nil
}

// CreateWithItems 在一个事务里建主表与明细：只有主表的收货单会让在途量算不出来。
func (r *purchaseReceiptRepo) CreateWithItems(
	ctx context.Context, m *model.PurchaseReceipt, items []model.PurchaseReceiptItem,
) error {
	return r.conn(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(m).Error; err != nil {
			return fmt.Errorf("新增收货单: %w", err)
		}
		for i := range items {
			items[i].ReceiptID = m.ID
		}
		if err := tx.Create(&items).Error; err != nil {
			return fmt.Errorf("新增收货单明细: %w", err)
		}
		return nil
	})
}

func (r *purchaseReceiptRepo) UpdateFields(
	ctx context.Context, id uint, fields map[string]any,
) error {
	return r.Base.UpdateFields(ctx, id, fields)
}

func (r *purchaseReceiptRepo) DeleteByID(ctx context.Context, id uint) error {
	return r.Base.DeleteByID(ctx, id)
}

func (r *purchaseReceiptRepo) AddReturnedQty(
	ctx context.Context, itemID uint, qty money.Decimal,
) error {
	res := r.conn(ctx).Model(&model.PurchaseReceiptItem{}).
		Where("id = ?", itemID).
		Update("returned_qty", gorm.Expr("returned_qty + ?", qty))
	if res.Error != nil {
		return fmt.Errorf("累加收货行 %d 的已退数量: %w", itemID, res.Error)
	}
	return nil
}
