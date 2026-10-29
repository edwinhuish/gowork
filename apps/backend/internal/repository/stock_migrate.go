package repository

import (
	"fmt"

	"gorm.io/gorm"

	"github.com/edwinhuish/gowork/internal/model"
)

// MigrateStockWarehouse 把库存切到仓库维度（T-56 第二步）。
//
// 两件事，顺序不能换：
//  1. 存量回填：老数据的 warehouse_id 全是 0，而新写入落在默认仓 id 上。
//     不回填的话同一产品会有"0 号仓一行、默认仓一行"两条快照，库存看起来凭空翻倍。
//  2. 切唯一索引：(product_id, deleted_at) → (warehouse_id, product_id, deleted_at)。
//     **不能用 AutoMigrate**：它只建不存在的索引，不改已存在索引的列；
//     而写入侧的 ON CONFLICT 必须匹配到一个真实存在的唯一约束。
//
// 幂等：回填只动 warehouse_id = 0 的行；索引先判存在再删，重复启动不会失败。
func MigrateStockWarehouse(db *gorm.DB) error {
	ids := make([]uint, 0, 1)
	if err := db.Model(&model.Warehouse{}).
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
		if err := db.Exec(
			"UPDATE "+table+" SET warehouse_id = ? WHERE warehouse_id = 0", defaultID,
		).Error; err != nil {
			return fmt.Errorf("回填 %s 的仓库: %w", table, err)
		}
	}

	m := db.Migrator()
	if m.HasIndex(&model.StockSnapshot{}, "uni_stock_snapshot_product") {
		if err := m.DropIndex(&model.StockSnapshot{}, "uni_stock_snapshot_product"); err != nil {
			return fmt.Errorf("删除旧快照唯一索引: %w", err)
		}
	}
	if err := m.CreateIndex(&model.StockSnapshot{}, "uni_stock_snapshot_product"); err != nil {
		return fmt.Errorf("建立新快照唯一索引: %w", err)
	}
	return nil
}
