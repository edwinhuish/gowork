package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/errs"
	"github.com/edwinhuish/gowork/internal/pkg/money"
)

// StockMovementFilter 是库存流水的查询条件。
type StockMovementFilter struct {
	Keyword   string
	ProductID *uint
	Type      *model.StockMovementType
	// From / To 按业务发生日过滤（含两端），只用到日期部分。
	From *time.Time
	To   *time.Time
}

// SnapshotRow 是库存列表的一行：快照 + 产品的当前说法。
//
// 快照本身不存 SKU / 品名，靠 join 取产品的当前值；产品改名后就该显示新名。
type SnapshotRow struct {
	ProductID      uint
	SKUCode        string
	NameCN         string
	Spec           string
	Unit           string
	Qty            money.Decimal
	LastMovementID uint
	LastOccurredAt *time.Time
	UpdatedAt      time.Time
}

// StockSum 是某个产品全部流水的聚合结果，用于全量重算。
type StockSum struct {
	Qty      money.Decimal
	Count    int64
	LastID   uint
	LastDate *time.Time
}

// StockRepository 同时管流水与快照。
//
// 两者不拆成两个仓储：快照是流水的物化结果，任何一次写入都必须在同一事务里
// 同时动两边。拆开之后，"先写流水、忘了更新快照"这种错就有了落脚点。
type StockRepository interface {
	ListMovements(ctx context.Context, f StockMovementFilter, p Pagination) ([]model.StockMovement, int64, error)
	FindMovementByID(ctx context.Context, id uint) (*model.StockMovement, error)
	// CreateMovement 插入流水并按带符号数量增量更新快照（同一事务）。
	CreateMovement(ctx context.Context, m *model.StockMovement) error
	// DeleteMovement 软删流水并回滚它对快照的影响（同一事务）。
	DeleteMovement(ctx context.Context, m *model.StockMovement) error

	ListSnapshots(ctx context.Context, keyword string, p Pagination) ([]SnapshotRow, int64, error)
	// FindSnapshot 不存在时返回 (nil, nil)：没有快照与"库存为 0"是两件事，
	// 前者说明这个产品从来没有流水。
	FindSnapshot(ctx context.Context, productID uint) (*model.StockSnapshot, error)
	SumMovements(ctx context.Context, productID uint) (StockSum, error)
	ReplaceSnapshot(ctx context.Context, productID uint, sum StockSum) error
	ProductsWithMovements(ctx context.Context) ([]uint, error)
}

type stockRepo struct {
	*Base[model.StockMovement]
}

func NewStockRepository(db *gorm.DB) StockRepository {
	return &stockRepo{Base: NewBase[model.StockMovement](db)}
}

func (r *stockRepo) ListMovements(
	ctx context.Context, f StockMovementFilter, p Pagination,
) ([]model.StockMovement, int64, error) {
	scopes := make([]Scope, 0, 5)
	if f.Keyword != "" {
		scopes = append(scopes, likeAny(f.Keyword,
			"doc_no", "sku_code", "name_cn", "source_doc_no", "remark"))
	}
	if f.ProductID != nil {
		id := *f.ProductID
		scopes = append(scopes, func(db *gorm.DB) *gorm.DB { return db.Where("product_id = ?", id) })
	}
	if f.Type != nil {
		t := *f.Type
		scopes = append(scopes, func(db *gorm.DB) *gorm.DB { return db.Where("type = ?", t) })
	}
	if f.From != nil {
		from := *f.From
		scopes = append(scopes, func(db *gorm.DB) *gorm.DB {
			return db.Where("occurred_at >= ?", from)
		})
	}
	if f.To != nil {
		to := *f.To
		scopes = append(scopes, func(db *gorm.DB) *gorm.DB { return db.Where("occurred_at <= ?", to) })
	}
	// 按发生日倒序：库存流水是按"哪天动过"看的，录入时间只在对账时才有意义。
	return r.PaginateOrdered(ctx, p, "occurred_at DESC, id DESC", scopes...)
}

// FindMovementByID 带上产品：详情页要显示品名与单位，而流水里存的是快照，
// 产品后来改了单位也不影响这条流水的历史说法。
func (r *stockRepo) FindMovementByID(ctx context.Context, id uint) (*model.StockMovement, error) {
	var m model.StockMovement
	err := r.conn(ctx).Preload("Product").First(&m, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errs.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("查询库存流水 id=%d: %w", id, err)
	}
	return &m, nil
}

func (r *stockRepo) CreateMovement(ctx context.Context, m *model.StockMovement) error {
	err := r.conn(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(m).Error; err != nil {
			return fmt.Errorf("写入库存流水: %w", err)
		}
		return addToSnapshot(tx, m.ProductID, m.SignedQty(), m.ID, m.OccurredAt)
	})
	return err
}

func (r *stockRepo) DeleteMovement(ctx context.Context, m *model.StockMovement) error {
	return r.conn(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Delete(&model.StockMovement{}, m.ID)
		if res.Error != nil {
			return fmt.Errorf("删除库存流水 id=%d: %w", m.ID, res.Error)
		}
		if res.RowsAffected == 0 {
			return errs.ErrNotFound
		}
		// 回滚这条流水对快照的影响：删一条入库，存量要减回去。
		return addToSnapshot(tx, m.ProductID, m.SignedQty().Neg(), m.ID, m.OccurredAt)
	})
}

// addToSnapshot 用 ON CONFLICT 做原子增量。
//
// 不用"先查再 insert/update"：两个人同时录一张入库单时，
// 两边都查到"还没有快照"，就会各插一行，撞唯一索引报错；
// 或者都读到旧值再各自写回，后写的把前一个的增量吃掉。
func addToSnapshot(
	tx *gorm.DB, productID uint, delta money.Decimal, movementID uint, occurredAt time.Time,
) error {
	delta = delta.Round(3)
	now := time.Now()
	row := model.StockSnapshot{
		ProductID:      productID,
		Qty:            delta,
		LastMovementID: movementID,
		LastOccurredAt: &occurredAt,
	}
	row.CreatedAt = now
	row.UpdatedAt = now

	err := tx.Clauses(clause.OnConflict{
		// 快照的唯一键是 (product_id, deleted_at)：未删除的行 deleted_at 恒为 0，
		// 所以带上它仍然等价于"每个产品一行"，同时不会与已删除的历史快照相撞。
		// 只写 product_id 的话 PG/SQLite 会报"ON CONFLICT 不匹配任何唯一约束"。
		Columns: []clause.Column{{Name: "product_id"}, {Name: "deleted_at"}},
		DoUpdates: clause.Assignments(map[string]any{
			// 右侧不带表名：MySQL 的 ON DUPLICATE KEY UPDATE 与
			// PostgreSQL 的 DO UPDATE 对表名前缀的接受程度不同，裸列名三库通用。
			//
			// 增量显式 CAST 成定点再相加：驱动送过去的是十进制字符串，
			// MySQL 在 `数值 + 字符串` 里会把字符串按 double 处理，
			// 那就等于在库里又绕了一次浮点，前面所有改造都白做。
			"qty": gorm.Expr("qty + CAST(? AS DECIMAL(16,3))", delta.StringFixed(3)),
			"last_movement_id": movementID,
			"last_occurred_at": occurredAt,
			"updated_at":       now,
		}),
	}).Create(&row).Error
	if err != nil {
		return fmt.Errorf("更新库存快照 product_id=%d: %w", productID, err)
	}
	return nil
}

func (r *stockRepo) ListSnapshots(
	ctx context.Context, keyword string, p Pagination,
) ([]SnapshotRow, int64, error) {
	p = p.normalize()

	base := func() *gorm.DB {
		q := r.conn(ctx).
			Table("inv_stock_snapshot AS s").
			Joins("LEFT JOIN md_product AS p ON p.id = s.product_id AND p.deleted_at = 0").
			Where("s.deleted_at = 0")
		if keyword != "" {
			q = q.Scopes(likeAny(keyword, "p.sku_code", "p.name_cn", "p.spec"))
		}
		return q
	}

	var total int64
	if err := base().Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("统计库存快照: %w", err)
	}
	if total == 0 {
		return []SnapshotRow{}, 0, nil
	}

	var rows []SnapshotRow
	err := base().
		Select(`s.product_id AS product_id,
			COALESCE(p.sku_code, '') AS sku_code,
			COALESCE(p.name_cn, '') AS name_cn,
			COALESCE(p.spec, '') AS spec,
			COALESCE(p.unit, '') AS unit,
			s.qty AS qty,
			s.last_movement_id AS last_movement_id,
			s.last_occurred_at AS last_occurred_at,
			s.updated_at AS updated_at`).
		Order("p.sku_code ASC").
		Offset(p.Offset()).
		Limit(p.PageSize).
		Scan(&rows).Error
	if err != nil {
		return nil, 0, fmt.Errorf("查询库存快照: %w", err)
	}
	return rows, total, nil
}

func (r *stockRepo) FindSnapshot(
	ctx context.Context, productID uint,
) (*model.StockSnapshot, error) {
	var s model.StockSnapshot
	err := r.conn(ctx).Where("product_id = ?", productID).First(&s).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("查询库存快照 product_id=%d: %w", productID, err)
	}
	return &s, nil
}

// SumMovements 把某个产品的全部流水折成一个数，是"库存的权威答案"。
func (r *stockRepo) SumMovements(ctx context.Context, productID uint) (StockSum, error) {
	db := r.conn(ctx)

	var agg struct {
		Qty   money.Decimal
		Count int64
	}
	err := db.Model(&model.StockMovement{}).
		Where("product_id = ?", productID).
		Select("COALESCE(SUM(direction * qty), 0) AS qty, COUNT(*) AS count").
		Scan(&agg).Error
	if err != nil {
		return StockSum{}, fmt.Errorf("聚合库存流水 product_id=%d: %w", productID, err)
	}

	sum := StockSum{Qty: agg.Qty.Round(3), Count: agg.Count}
	if agg.Count == 0 {
		return sum, nil
	}

	var last model.StockMovement
	err = db.Where("product_id = ?", productID).
		Order("occurred_at DESC, id DESC").
		Limit(1).
		Find(&last).Error
	if err != nil {
		return StockSum{}, fmt.Errorf("查询最后一条库存流水 product_id=%d: %w", productID, err)
	}
	sum.LastID = last.ID
	occurred := last.OccurredAt
	sum.LastDate = &occurred
	return sum, nil
}

// ReplaceSnapshot 把快照写成"重算后的唯一正确答案"。
//
// 没有流水时也要写一行 qty=0，而不是删掉这一行：软删会让唯一索引继续占位，
// 下次入库 upsert 会更新到那条已删的行上，表现是"库存加进去了但列表里看不到"。
func (r *stockRepo) ReplaceSnapshot(ctx context.Context, productID uint, sum StockSum) error {
	now := time.Now()
	qty := sum.Qty.Round(3)
	row := model.StockSnapshot{
		ProductID:      productID,
		Qty:            qty,
		LastMovementID: sum.LastID,
		LastOccurredAt: sum.LastDate,
	}
	row.CreatedAt = now
	row.UpdatedAt = now

	err := r.conn(ctx).Clauses(clause.OnConflict{
		// 同 CalcSnapshot 里的说明：唯一键含 deleted_at。
		Columns: []clause.Column{{Name: "product_id"}, {Name: "deleted_at"}},
		DoUpdates: clause.Assignments(map[string]any{
			"qty":              qty,
			"last_movement_id": sum.LastID,
			"last_occurred_at": sum.LastDate,
			"updated_at":       now,
		}),
	}).Create(&row).Error
	if err != nil {
		return fmt.Errorf("重算库存快照 product_id=%d: %w", productID, err)
	}
	return nil
}

func (r *stockRepo) ProductsWithMovements(ctx context.Context) ([]uint, error) {
	var ids []uint
	err := r.conn(ctx).
		Model(&model.StockMovement{}).
		Distinct("product_id").
		Pluck("product_id", &ids).Error
	if err != nil {
		return nil, fmt.Errorf("查询有流水的产品: %w", err)
	}
	return ids, nil
}
