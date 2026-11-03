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

// SalesOrderFilter 是销售订单列表的查询条件。
type SalesOrderFilter struct {
	// Keyword 匹配单号与备注；按客户找单走 partner_id。
	Keyword   string
	PartnerID *uint
	Status    *model.DocumentStatus
	// OverdueUnshipped 只留「交期已过、还没出完」的订单：跟单最关心的那一撮。
	//
	// 逾期相对某个截止日算，所以必须一起给 AsOf——用 time.Now() 兜底会让
	// 测试与跨时区的调用在午夜前后给出不同结果。
	OverdueUnshipped bool
	AsOf             time.Time
	// OwnerID 只看归属该业务员的客户的单（含未分配客户的单）。
	OwnerID *uint
}

func (f SalesOrderFilter) scopes() []Scope {
	scopes := make([]Scope, 0, 3)
	if f.Keyword != "" {
		scopes = append(scopes, likeAny(f.Keyword, "doc_no", "remark"))
	}
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
	if f.OverdueUnshipped {
		asOf := f.AsOf
		// 「逾期未出」= 交期已过 + 还有行没出完。
		//
		// 只看承诺过的单（已确认 / 备货中 / 已出货）：草稿的交期是我们自己填的意向，
		// 把它算成逾期会让列表永远挂着一堆没对客户承诺过的单；已完成与已取消是终态，
		// 归档的单不该再出现在"待处理"里。
		//
		// 已出货仍然参与：分批出货时，一票发走之后剩下的量还是欠客户的。
		//
		// 用 EXISTS 而不是 JOIN：一单多行没出完时 JOIN 会让同一张单在结果里出现多次，
		// 分页总数跟着虚高。
		scopes = append(scopes, func(db *gorm.DB) *gorm.DB {
			return db.Where(`delivery_date IS NOT NULL AND delivery_date < ?
				AND status IN ?
				AND EXISTS (
					SELECT 1 FROM biz_sales_order_item i
					WHERE i.sales_order_id = biz_sales_order.id
					  AND i.deleted_at = 0
					  AND i.qty > COALESCE((
						SELECT SUM(a.qty) FROM biz_shipment_allocation a
						JOIN biz_shipment s ON s.id = a.shipment_id
						WHERE a.sales_order_item_id = i.id
						  AND a.deleted_at = 0
						  AND s.deleted_at = 0
						  AND s.status <> ?
					  ), 0) + 0.000001
				)`, asOf, []model.DocumentStatus{
				model.DocumentStatusConfirmed,
				model.DocumentStatusPreparing,
				model.DocumentStatusShipped,
			}, model.DocumentStatusCancelled)
		})
	}
	if f.OwnerID != nil {
		scopes = append(scopes, OwnerScope("partner_id", *f.OwnerID))
	}
	return scopes
}

// Fulfillment 是一张订单的履约量：订购总量与已出总量（已取消的出货不计）。
type Fulfillment struct {
	OrderQty   money.Decimal
	ShippedQty money.Decimal
}

type SalesOrderRepository interface {
	CreateWithItems(ctx context.Context, o *model.SalesOrder, items []model.SalesOrderItem) error
	UpdateWithItems(ctx context.Context, id uint, fields map[string]any, items []model.SalesOrderItem) error
	Update(ctx context.Context, id uint, fields map[string]any) error
	Delete(ctx context.Context, id uint) error
	FindByID(ctx context.Context, id uint) (*model.SalesOrder, error)
	List(ctx context.Context, f SalesOrderFilter, p Pagination) ([]model.SalesOrder, int64, error)
	// FindItemByID 取销售订单明细行并带出所属订单，供采购单校验「以销定采」的挂接。
	// 采购单要确认的不只是行存在，还有"这张销售单还能不能被备货"——已取消的单不能再采。
	FindItemByID(ctx context.Context, itemID uint) (*model.SalesOrderItem, error)
	// FulfillmentByOrders 汇总整页订单的「订购量 / 已出量」，key 是订单 ID。
	//
	// 列表要用这个数，但逐张单调 ShippedQtyByOrder 会让一页二十张单跑二十次聚合；
	// 一次按 IN 取回整页即可。口径与 ShippedQtyByOrder 完全一致（已取消的出货不算），
	// 两处写法不同、语义必须相同，否则列表与详情会各说一个数。
	FulfillmentByOrders(ctx context.Context, orderIDs []uint) (map[uint]Fulfillment, error)
}

type salesOrderRepo struct {
	*Base[model.SalesOrder]
}

func NewSalesOrderRepository(db *gorm.DB) SalesOrderRepository {
	return &salesOrderRepo{Base: NewBase[model.SalesOrder](db)}
}

func (r *salesOrderRepo) CreateWithItems(
	ctx context.Context, o *model.SalesOrder, items []model.SalesOrderItem,
) error {
	err := r.conn(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(o).Error; err != nil {
			return err
		}
		if len(items) == 0 {
			return nil
		}
		for i := range items {
			items[i].SalesOrderID = o.ID
		}
		return tx.Create(&items).Error
	})
	if err != nil {
		return fmt.Errorf("创建销售订单: %w", err)
	}
	o.Items = items
	return nil
}

func (r *salesOrderRepo) UpdateWithItems(
	ctx context.Context, id uint, fields map[string]any, items []model.SalesOrderItem,
) error {
	err := r.conn(ctx).Transaction(func(tx *gorm.DB) error {
		q := tx.Model(&model.SalesOrder{}).Where("id = ?", id)
		// 与 Base.UpdateFields 同一套乐观锁：明细整体替换之后无从恢复。
		expected, hasLock := takeOptimisticLock(fields)
		if hasLock {
			q = q.Where("updated_at = ?", expected)
		}
		res := q.Updates(fields)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			if hasLock {
				return errs.ErrConflict.WithMsg("这条记录已被他人修改，请刷新后重试")
			}
			return errs.ErrNotFound
		}
		// 明细整体替换：行没有稳定身份，按 id 做增量更新会张冠李戴。
		if err := tx.Where("sales_order_id = ?", id).Delete(&model.SalesOrderItem{}).Error; err != nil {
			return err
		}
		if len(items) == 0 {
			return nil
		}
		for i := range items {
			items[i].SalesOrderID = id
		}
		return tx.Create(&items).Error
	})
	if err != nil {
		if errors.Is(err, errs.ErrNotFound) {
			return errs.ErrNotFound
		}
		return fmt.Errorf("更新销售订单 id=%d: %w", id, err)
	}
	return nil
}

func (r *salesOrderRepo) Update(ctx context.Context, id uint, fields map[string]any) error {
	return r.Base.UpdateFields(ctx, id, fields)
}

func (r *salesOrderRepo) Delete(ctx context.Context, id uint) error {
	return r.Base.DeleteByID(ctx, id)
}

func (r *salesOrderRepo) FindByID(ctx context.Context, id uint) (*model.SalesOrder, error) {
	var m model.SalesOrder
	err := r.conn(ctx).
		Preload("Partner").
		Preload("Items", func(db *gorm.DB) *gorm.DB { return db.Order("id ASC") }).
		Preload("Items.Product").
		First(&m, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errs.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("查询销售订单 id=%d: %w", id, err)
	}
	return &m, nil
}

func (r *salesOrderRepo) FindItemByID(
	ctx context.Context, itemID uint,
) (*model.SalesOrderItem, error) {
	var m model.SalesOrderItem
	err := r.conn(ctx).
		Preload("SalesOrder").
		First(&m, itemID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errs.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("查询销售订单明细 id=%d: %w", itemID, err)
	}
	return &m, nil
}

// List 按下单日倒序，同日按 id 倒序：业务上关心的是"最近下了什么单"。
func (r *salesOrderRepo) List(
	ctx context.Context, f SalesOrderFilter, p Pagination,
) ([]model.SalesOrder, int64, error) {
	scopes := append(f.scopes(), func(db *gorm.DB) *gorm.DB {
		return db.Preload("Partner")
	})
	return r.PaginateOrdered(ctx, p, "order_date DESC, id DESC", scopes...)
}

// FulfillmentByOrders 用两条聚合查询取回整页：一条按订单汇总订购量，
// 一条按订单汇总已出量。合写成一条 SQL 会变成三段 JOIN 加 GROUP BY，
// 而订单行数量与分配行数量并不一一对应，SUM 出来的乘积要再除一次——
// 那种 SQL 改一次错一次，两条清楚的查询反而更快也更不容易错。
func (r *salesOrderRepo) FulfillmentByOrders(
	ctx context.Context, orderIDs []uint,
) (map[uint]Fulfillment, error) {
	out := make(map[uint]Fulfillment, len(orderIDs))
	if len(orderIDs) == 0 {
		return out, nil
	}

	var ordered []struct {
		SalesOrderID uint
		Qty          money.Decimal
	}
	err := r.conn(ctx).
		Model(&model.SalesOrderItem{}).
		Where("sales_order_id IN ?", orderIDs).
		Group("sales_order_id").
		Select("sales_order_id AS sales_order_id, COALESCE(SUM(qty), 0) AS qty").
		Scan(&ordered).Error
	if err != nil {
		return nil, fmt.Errorf("统计销售订单订购量: %w", err)
	}
	for _, row := range ordered {
		f := out[row.SalesOrderID]
		f.OrderQty = row.Qty
		out[row.SalesOrderID] = f
	}

	var shipped []struct {
		SalesOrderID uint
		Qty          money.Decimal
	}
	err = r.conn(ctx).
		Model(&model.ShipmentAllocation{}).
		Joins("JOIN biz_shipment ON biz_shipment.id = biz_shipment_allocation.shipment_id").
		Joins(`JOIN biz_sales_order_item ON biz_sales_order_item.id = biz_shipment_allocation.sales_order_item_id`).
		Where("biz_sales_order_item.sales_order_id IN ?", orderIDs).
		Where("biz_shipment.status <> ?", model.DocumentStatusCancelled).
		Where("biz_shipment.deleted_at = 0").
		Group("biz_sales_order_item.sales_order_id").
		Select(`biz_sales_order_item.sales_order_id AS sales_order_id,
			COALESCE(SUM(biz_shipment_allocation.qty), 0) AS qty`).
		Scan(&shipped).Error
	if err != nil {
		return nil, fmt.Errorf("统计销售订单已出量: %w", err)
	}
	for _, row := range shipped {
		f := out[row.SalesOrderID]
		f.ShippedQty = row.Qty
		out[row.SalesOrderID] = f
	}
	return out, nil
}
