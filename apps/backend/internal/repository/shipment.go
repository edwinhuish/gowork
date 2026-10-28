package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/errs"
)

// ShipmentFilter 是出货列表的查询条件。
type ShipmentFilter struct {
	Keyword string
	Status  *model.DocumentStatus
	// SalesOrderID 查"这张订单出了几次货"，走 EXISTS 子查询而不是 JOIN：
	// 一票装多张订单时 JOIN 会让同一票在结果里出现多次，分页总数也跟着虚高。
	SalesOrderID *uint
}

func (f ShipmentFilter) scopes() []Scope {
	scopes := make([]Scope, 0, 3)
	if f.Keyword != "" {
		scopes = append(scopes, likeAny(f.Keyword, "doc_no", "remark"))
	}
	if f.Status != nil {
		st := *f.Status
		scopes = append(scopes, func(db *gorm.DB) *gorm.DB {
			return db.Where("status = ?", st)
		})
	}
	if f.SalesOrderID != nil {
		soID := *f.SalesOrderID
		scopes = append(scopes, func(db *gorm.DB) *gorm.DB {
			return db.Where(`EXISTS (
				SELECT 1 FROM biz_shipment_allocation a
				JOIN biz_sales_order_item i ON i.id = a.sales_order_item_id
				WHERE a.shipment_id = biz_shipment.id
				  AND a.deleted_at IS NULL AND i.sales_order_id = ?
			)`, soID)
		})
	}
	return scopes
}

type ShipmentRepository interface {
	CreateWithAllocations(
		ctx context.Context, s *model.Shipment, rows []model.ShipmentAllocation,
	) error
	UpdateWithAllocations(
		ctx context.Context, id uint, fields map[string]any, rows []model.ShipmentAllocation,
	) error
	Update(ctx context.Context, id uint, fields map[string]any) error
	Delete(ctx context.Context, id uint) error
	// BumpInvoiceRevision 把商票版本加一，返回新版本号（改证重开）。
	//
	// 用 SQL 自增而不是"读出来 +1 再写回"：两个窗口同时点重开，
	// 后写的那次会覆盖前一次，两版并成一版，而客户手上已经拿到了其中一版。
	BumpInvoiceRevision(ctx context.Context, id uint) (int, error)
	FindByID(ctx context.Context, id uint) (*model.Shipment, error)
	List(ctx context.Context, f ShipmentFilter, p Pagination) ([]model.Shipment, int64, error)
	// AllocatedQty 返回某条销售订单行**已被有效出货占用**的数量。
	//
	// 已取消的出货不计入：取消的意义就是把数量还回去，否则那一行会被永久占死，
	// 客户取消一票再补一票时就会报"超出未出数量"——而实际上什么都没出。
	// excludeShipmentID 供编辑时排除自己。
	AllocatedQty(ctx context.Context, salesOrderItemID uint, excludeShipmentID uint) (float64, error)
	// ShippedQtyByOrder 返回一张销售订单下每行已出数量，key 是明细行 ID。
	//
	// 一次聚合取整张单，而不是每行查一次：订单详情页一屏就有十几行，
	// 逐行查会把"打开一张订单"变成十几次往返。
	ShippedQtyByOrder(ctx context.Context, salesOrderID uint) (map[uint]float64, error)
	// ListShippableItems 列出"还能出货的销售订单行"，供出货单选行。
	//
	// 过滤条件与 service 的校验是同一套语义：订单已确认或备货中、且未出数量大于 0。
	// 放在 SQL 里而不是取回来再筛：可出货行会随历史单积累到几千条，
	// 全量取回再丢掉"已出完"的部分，等于每次打开出货单都做一次全表扫描。
	// excludeShipmentID 供编辑时用：把这一票自己已经占的数量还给它，
	// 否则"已出完的行"在编辑页的下拉里会消失，用户改数量时看不到自己那一行。
	ListShippableItems(
		ctx context.Context, keyword string, excludeShipmentID uint, limit int,
	) ([]ShippableItem, error)
}

// ShippableItem 是"还能出货的销售订单行"的查询结果。
type ShippableItem struct {
	SalesOrderID     uint
	SalesOrderDocNo  string
	PartnerName      string
	SalesOrderItemID uint
	SKUCode          string
	NameCN           string
	Spec             string
	Unit             string
	Qty              float64
	ShippedQty       float64
	DeliveryDate     *time.Time
}

type shipmentRepo struct {
	*Base[model.Shipment]
}

func NewShipmentRepository(db *gorm.DB) ShipmentRepository {
	return &shipmentRepo{Base: NewBase[model.Shipment](db)}
}

func (r *shipmentRepo) CreateWithAllocations(
	ctx context.Context, s *model.Shipment, rows []model.ShipmentAllocation,
) error {
	err := r.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(s).Error; err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}
		for i := range rows {
			rows[i].ShipmentID = s.ID
		}
		return tx.Create(&rows).Error
	})
	if err != nil {
		return fmt.Errorf("创建出货批次: %w", err)
	}
	s.Allocations = rows
	return nil
}

func (r *shipmentRepo) UpdateWithAllocations(
	ctx context.Context, id uint, fields map[string]any, rows []model.ShipmentAllocation,
) error {
	err := r.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&model.Shipment{}).Where("id = ?", id).Updates(fields)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return errs.ErrNotFound
		}
		if err := tx.Where("shipment_id = ?", id).
			Delete(&model.ShipmentAllocation{}).Error; err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}
		for i := range rows {
			rows[i].ShipmentID = id
		}
		return tx.Create(&rows).Error
	})
	if err != nil {
		if errors.Is(err, errs.ErrNotFound) {
			return errs.ErrNotFound
		}
		return fmt.Errorf("更新出货批次 id=%d: %w", id, err)
	}
	return nil
}

func (r *shipmentRepo) Update(ctx context.Context, id uint, fields map[string]any) error {
	return r.Base.UpdateFields(ctx, id, fields)
}

func (r *shipmentRepo) Delete(ctx context.Context, id uint) error {
	return r.Base.DeleteByID(ctx, id)
}

func (r *shipmentRepo) BumpInvoiceRevision(ctx context.Context, id uint) (int, error) {
	db := r.DB().WithContext(ctx)
	res := db.Model(&model.Shipment{}).Where("id = ?", id).
		Update("invoice_revision", gorm.Expr("invoice_revision + 1"))
	if res.Error != nil {
		return 0, fmt.Errorf("升商票版本 id=%d: %w", id, res.Error)
	}
	if res.RowsAffected == 0 {
		return 0, errs.ErrNotFound
	}

	// 回读而不是自己算 +1：并发时以库里落下的值为准，算出来的那个数与实际版本可能差一版。
	var rev int
	if err := db.Model(&model.Shipment{}).Where("id = ?", id).
		Pluck("invoice_revision", &rev).Error; err != nil {
		return 0, fmt.Errorf("读取商票版本 id=%d: %w", id, err)
	}
	return rev, nil
}

func (r *shipmentRepo) FindByID(ctx context.Context, id uint) (*model.Shipment, error) {
	var m model.Shipment
	err := r.DB().WithContext(ctx).
		Preload("Allocations", func(db *gorm.DB) *gorm.DB { return db.Order("id ASC") }).
		Preload("Allocations.SalesOrderItem").
		Preload("Allocations.SalesOrderItem.SalesOrder").
		First(&m, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errs.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("查询出货批次 id=%d: %w", id, err)
	}
	return &m, nil
}

// List 按出货日倒序：最近发走的那票在最上面。
// 列表带分配行（一票通常只有几行），因为列表要显示"共几件、来自哪几张单"。
func (r *shipmentRepo) List(
	ctx context.Context, f ShipmentFilter, p Pagination,
) ([]model.Shipment, int64, error) {
	scopes := append(f.scopes(), func(db *gorm.DB) *gorm.DB {
		return db.Preload("Allocations", func(db *gorm.DB) *gorm.DB { return db.Order("id ASC") }).
			Preload("Allocations.SalesOrderItem").
			Preload("Allocations.SalesOrderItem.SalesOrder")
	})
	return r.PaginateOrdered(ctx, p, "ship_date DESC, id DESC", scopes...)
}

func (r *shipmentRepo) AllocatedQty(
	ctx context.Context, salesOrderItemID uint, excludeShipmentID uint,
) (float64, error) {
	q := r.DB().WithContext(ctx).
		Model(&model.ShipmentAllocation{}).
		Joins("JOIN biz_shipment ON biz_shipment.id = biz_shipment_allocation.shipment_id").
		Where("biz_shipment_allocation.sales_order_item_id = ?", salesOrderItemID).
		Where("biz_shipment.status <> ?", model.DocumentStatusCancelled).
		Where("biz_shipment.deleted_at IS NULL")
	if excludeShipmentID > 0 {
		q = q.Where("biz_shipment_allocation.shipment_id <> ?", excludeShipmentID)
	}

	var sum float64
	if err := q.Select("COALESCE(SUM(biz_shipment_allocation.qty), 0)").Scan(&sum).Error; err != nil {
		return 0, fmt.Errorf("统计销售订单行已出数量 item=%d: %w", salesOrderItemID, err)
	}
	return sum, nil
}

// ShippedQtyByOrder 与 AllocatedQty 同一口径（已取消的出货不算），
// 区别只是按键汇总：一处口径写两遍，早晚会在某个报表里对不上。
func (r *shipmentRepo) ShippedQtyByOrder(
	ctx context.Context, salesOrderID uint,
) (map[uint]float64, error) {
	var rows []struct {
		SalesOrderItemID uint
		Qty              float64
	}
	err := r.DB().WithContext(ctx).
		Model(&model.ShipmentAllocation{}).
		Joins("JOIN biz_shipment ON biz_shipment.id = biz_shipment_allocation.shipment_id").
		Joins(`JOIN biz_sales_order_item ON biz_sales_order_item.id = biz_shipment_allocation.sales_order_item_id`).
		Where("biz_sales_order_item.sales_order_id = ?", salesOrderID).
		Where("biz_shipment.status <> ?", model.DocumentStatusCancelled).
		Where("biz_shipment.deleted_at IS NULL").
		Group("biz_shipment_allocation.sales_order_item_id").
		Select(`biz_shipment_allocation.sales_order_item_id AS sales_order_item_id,
			COALESCE(SUM(biz_shipment_allocation.qty), 0) AS qty`).
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("统计销售订单已出数量 order=%d: %w", salesOrderID, err)
	}
	out := make(map[uint]float64, len(rows))
	for _, row := range rows {
		out[row.SalesOrderItemID] = row.Qty
	}
	return out, nil
}

// ListShippableItems 用 LEFT JOIN 聚合子查询算已出数量，而不是逐行调用 AllocatedQty。
//
// 子查询里已取消的出货不计入，口径与 AllocatedQty 一致——两处口径分叉，
// 表现是"下拉里说还能出 40，提交时却报只能出 30"。
func (r *shipmentRepo) ListShippableItems(
	ctx context.Context, keyword string, excludeShipmentID uint, limit int,
) ([]ShippableItem, error) {
	shippedQ := r.DB().Table("biz_shipment_allocation AS a").
		Joins("JOIN biz_shipment AS s ON s.id = a.shipment_id").
		Where("a.deleted_at IS NULL").
		Where("s.deleted_at IS NULL").
		Where("s.status <> ?", model.DocumentStatusCancelled)
	if excludeShipmentID > 0 {
		shippedQ = shippedQ.Where("a.shipment_id <> ?", excludeShipmentID)
	}
	shipped := shippedQ.
		Group("a.sales_order_item_id").
		Select("a.sales_order_item_id AS sales_order_item_id, COALESCE(SUM(a.qty), 0) AS shipped_qty")

	q := r.DB().WithContext(ctx).
		Table("biz_sales_order_item AS i").
		Joins("JOIN biz_sales_order AS o ON o.id = i.sales_order_id").
		Joins("LEFT JOIN md_partner AS p ON p.id = o.partner_id").
		Joins("LEFT JOIN (?) AS t ON t.sales_order_item_id = i.id", shipped).
		Where("i.deleted_at IS NULL").
		Where("o.deleted_at IS NULL").
		Where("o.status IN ?", []model.DocumentStatus{
			model.DocumentStatusConfirmed, model.DocumentStatusPreparing,
		}).
		Where("i.qty > COALESCE(t.shipped_qty, 0) + 0.000001")

	if kw := strings.TrimSpace(keyword); kw != "" {
		like := "%" + kw + "%"
		q = q.Where("(o.doc_no LIKE ? OR i.sku_code LIKE ? OR i.name_cn LIKE ? OR p.name LIKE ?)",
			like, like, like, like)
	}

	var rows []ShippableItem
	err := q.Select(`i.sales_order_id AS sales_order_id,
			o.doc_no AS sales_order_doc_no,
			COALESCE(p.name, '') AS partner_name,
			i.id AS sales_order_item_id,
			i.sku_code, i.name_cn, i.spec, i.unit, i.qty,
			COALESCE(t.shipped_qty, 0) AS shipped_qty,
			o.delivery_date`).
		Order("o.delivery_date ASC, i.id ASC").
		Limit(limit).
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("查询可出货订单行: %w", err)
	}
	return rows, nil
}
