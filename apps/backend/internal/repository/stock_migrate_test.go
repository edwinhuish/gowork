package repository

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/edwinhuish/gowork/internal/config"
	"github.com/edwinhuish/gowork/internal/infra/database"
	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/money"
)

// TestBackfillWarehouseBatches 验存量回填真的分批，而不是一条 UPDATE 到底。
//
// 把批次调到 3、造 7 行：第一批装不下，游标必须往前挪一次。
// 断言三件事——一行不漏、日志里出现三轮「回填中」、再跑一次是空转。
// 第二条是关键：只比对最终结果的话，"一条 UPDATE 全表"同样能过，
// 而那正是要在生产上被淘汰的写法（写锁时长与行数成正比）。
func TestBackfillWarehouseBatches(t *testing.T) {
	ctx := context.Background()
	log, buf := captureLog()

	prev := stockBackfillBatch
	stockBackfillBatch = 3
	t.Cleanup(func() { stockBackfillBatch = prev })

	db := openMigrateTestDB(t, log)

	const rows = 7
	moves := make([]model.StockMovement, 0, rows)
	for i := 0; i < rows; i++ {
		moves = append(moves, model.StockMovement{
			DocNo:      fmt.Sprintf("MV-%03d", i),
			ProductID:  uint(i + 1),
			SKUCode:    fmt.Sprintf("SKU-%d", i+1),
			Type:       model.StockMovementPurchase,
			Direction:  model.StockDirectionIn,
			Qty:        money.FromInt(1),
			OccurredAt: model.DateOnly(time.Now()),
		})
	}
	if err := db.Create(&moves).Error; err != nil {
		t.Fatalf("建存量流水: %v", err)
	}

	if err := backfillWarehouse(ctx, db, "inv_stock_movement", 9, log); err != nil {
		t.Fatalf("回填失败: %v", err)
	}

	var left int64
	if err := db.Model(&model.StockMovement{}).Where("warehouse_id <> ?", 9).Count(&left).Error; err != nil {
		t.Fatalf("统计未回填流水: %v", err)
	}
	if left != 0 {
		t.Errorf("还有 %d 条流水没回填", left)
	}

	if rounds := strings.Count(buf.String(), "库存仓库回填中"); rounds != 3 {
		t.Errorf("7 行按每批 3 行应分 3 批，日志里只有 %d 轮：\n%s", rounds, buf.String())
	}

	// 断点续跑：再执行一次什么都不该做，也不该报错。
	buf.Reset()
	if err := backfillWarehouse(ctx, db, "inv_stock_movement", 9, log); err != nil {
		t.Fatalf("二次回填失败（应空转）: %v", err)
	}
	if strings.Contains(buf.String(), "库存仓库回填中") {
		t.Errorf("没有待回填的行时不该再起批次：\n%s", buf.String())
	}
}

// TestMigrateStockWarehouseSkipsIndexRebuild 验索引列已经对了就不再重建。
//
// 这条迁移随服务启动跑，无条件 DROP + CREATE 等于每次重启都在存量表上
// 重建一次唯一索引。判据用 sqlite 的 schema_version：任何 DDL 都会让它加一，
// 而"索引已对得上"这条路径一次 DDL 都不该发。比比对 index 的 rootpage 可靠
// ——重建有可能复用刚释放的页号，那样就看不出来了。
func TestMigrateStockWarehouseSkipsIndexRebuild(t *testing.T) {
	ctx := context.Background()
	log, _ := captureLog()
	db := openMigrateTestDB(t, log)

	wh := &model.Warehouse{Code: "MAIN", Name: "主仓", IsDefault: true}
	if err := db.Create(wh).Error; err != nil {
		t.Fatalf("建默认仓: %v", err)
	}

	if err := MigrateStockWarehouse(ctx, db, log); err != nil {
		t.Fatalf("首次迁移失败: %v", err)
	}
	before := schemaVersion(t, db)

	if err := MigrateStockWarehouse(ctx, db, log); err != nil {
		t.Fatalf("二次迁移失败: %v", err)
	}
	if after := schemaVersion(t, db); after != before {
		t.Errorf("二次迁移又动了表结构（schema_version %d → %d），启动时会重建索引", before, after)
	}

	// 判据自检：手工重建一次索引，schema_version 必须跟着变。
	// 少了这一段，上面那条断言在"schema_version 压根不随 DDL 动"的库上会永远通过。
	if err := db.Exec("DROP INDEX uni_stock_snapshot_product").Error; err != nil {
		t.Fatalf("自检:删索引: %v", err)
	}
	if err := db.Exec(
		"CREATE UNIQUE INDEX uni_stock_snapshot_product ON inv_stock_snapshot (warehouse_id, product_id, deleted_at)",
	).Error; err != nil {
		t.Fatalf("自检:建索引: %v", err)
	}
	if v := schemaVersion(t, db); v == before {
		t.Errorf("重建了索引但 schema_version 没变（%d），这条测试的判据无效", v)
	}
}

func captureLog() (*slog.Logger, *bytes.Buffer) {
	buf := &bytes.Buffer{}
	return slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelInfo})), buf
}

func openMigrateTestDB(t *testing.T, log *slog.Logger) *gorm.DB {
	t.Helper()

	db, err := database.Open(config.Database{
		Driver:      config.DriverSQLite,
		DSN:         filepath.Join(t.TempDir(), "stockmigrate.db"),
		MaxOpen:     2,
		MaxIdle:     1,
		MaxLifetime: 60,
		LogLevel:    "error",
	}, log)
	if err != nil {
		t.Fatalf("打开测试数据库: %v", err)
	}
	t.Cleanup(func() { _ = database.Close(db) })

	if err := database.AutoMigrate(db, &model.Warehouse{}, &model.StockMovement{}, &model.StockSnapshot{}); err != nil {
		t.Fatalf("建表: %v", err)
	}
	return db
}

// schemaVersion 读 sqlite 的 schema_version：任何 DDL（建表、建/删索引）都会让它加一。
func schemaVersion(t *testing.T, db *gorm.DB) int {
	t.Helper()

	var v int
	if err := db.Raw("PRAGMA schema_version").Scan(&v).Error; err != nil {
		t.Fatalf("读 schema_version: %v", err)
	}
	return v
}
