package repository

import (
	"context"
	"fmt"
	"log/slog"

	"gorm.io/gorm"

	"github.com/edwinhuish/gowork/internal/model"
)

// stockBackfillBatch 是存量回填每批的行数。
//
// 存在的理由不是性能而是锁：一条 UPDATE 全表会在流水表上拿一把长写锁，
// 这函数随服务启动跑，那段时间里所有入库出库都被挡住。5000 行的单批在常见机器上
// 是毫秒级，够短；再小就要多跑几十轮，收益不抵往返开销。
//
// 是变量而不是常量，只为了测试能把它调小（见 stock_migrate_test.go）：
// 分批逻辑的分支要用小批量才验得动，插几千行去凑一批既慢又验不出别的东西。
var stockBackfillBatch = 5000

// MigrateStockWarehouse 把库存切到仓库维度（T-56 第二步）。
//
// 两件事，顺序不能换：
//  1. 存量回填：老数据的 warehouse_id 全是 0，而新写入落在默认仓 id 上。
//     不回填的话同一产品会有"0 号仓一行、默认仓一行"两条快照，库存看起来凭空翻倍。
//  2. 切唯一索引：(product_id, deleted_at) → (warehouse_id, product_id, deleted_at)。
//     **不能用 AutoMigrate**：它只建不存在的索引，不改已存在索引的列；
//     而写入侧的 ON CONFLICT 必须匹配到一个真实存在的唯一约束。
//
// 幂等：回填只动 warehouse_id = 0 的行，按 id 游标推进，中途中断下次从断点继续；
// 索引先看列，已经带 warehouse_id 就跳过，重复启动既不会失败，也不会反复重建索引。
func MigrateStockWarehouse(ctx context.Context, db *gorm.DB, log *slog.Logger) error {
	ids := make([]uint, 0, 1)
	if err := db.WithContext(ctx).Model(&model.Warehouse{}).
		Where("is_default = ? AND status = ?", true, model.EnableStatusOn).
		Order("id ASC").Limit(1).Pluck("id", &ids).Error; err != nil {
		return fmt.Errorf("查询默认仓库: %w", err)
	}
	if len(ids) == 0 {
		// 没有仓库可归：保持 warehouse_id = 0 不动。启动时的 EnsureDefault
		// 正常情况下已经建了一条，走到这里说明那条也失败了。
		return nil
	}
	defaultID := ids[0]

	for _, table := range []string{"inv_stock_movement", "inv_stock_snapshot"} {
		if err := backfillWarehouse(ctx, db, table, defaultID, log); err != nil {
			return err
		}
	}

	return ensureIndexColumns(db.Migrator(), &model.StockSnapshot{},
		"uni_stock_snapshot_product", "warehouse_id", "product_id", "deleted_at")
}

// backfillWarehouse 把某张表里 warehouse_id = 0 的存量行按 id 游标分批改成默认仓。
//
// 不写 `UPDATE ... WHERE warehouse_id = 0` 一条到底：存量流水可能几十万行，
// 一条语句的写锁时长与行数成正比，而这条迁移是在服务启动路径上跑的。
// 分批之后每批各是一笔短事务；被打断也只是停在某个 id 上，条件始终是
// warehouse_id = 0，重启从那里接着来。游标（id > lastID）而不是 LIMIT 偏移：
// 后者的每一批都要重新扫过前面已经改完的行。
func backfillWarehouse(
	ctx context.Context, db *gorm.DB, table string, defaultID uint, log *slog.Logger,
) error {
	var (
		ids    []uint
		lastID uint
		total  int64
	)
	for {
		ids = ids[:0]
		if err := db.WithContext(ctx).Table(table).
			Where("warehouse_id = 0 AND id > ?", lastID).
			Order("id ASC").Limit(stockBackfillBatch).
			Pluck("id", &ids).Error; err != nil {
			return fmt.Errorf("查 %s 待回填行: %w", table, err)
		}
		if len(ids) == 0 {
			break
		}
		if err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			// UpdateColumn 而不是 Update：回填不是业务变更，不该把 updated_at 刷新成
			// 今天——那样"这行有多旧"就全被抹掉了，出问题时看不出数据是哪年的。
			return tx.Table(table).Where("id IN ?", ids).
				UpdateColumn("warehouse_id", defaultID).Error
		}); err != nil {
			return fmt.Errorf("回填 %s 的仓库: %w", table, err)
		}
		lastID = ids[len(ids)-1]
		total += int64(len(ids))
		log.Info("库存仓库回填中", "table", table, "已回填", total, "到 id", lastID)
	}
	if total > 0 {
		log.Info("库存仓库回填完成", "table", table, "回填行", total, "仓库", defaultID)
	}
	return nil
}
