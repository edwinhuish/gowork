package service

import (
	"context"
	"testing"
	"time"

	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/money"
	"github.com/edwinhuish/gowork/internal/repository"
)

// TestMigrateStockWarehouse 是 T-56 迁移的演练。
//
// 迁移上线前只在空库跑过，从未用"存量数据"验过；而它要处理的是两类真实风险：
//   - 老数据的 warehouse_id 全是 0，不回填就会与新写入的默认仓分成两组快照，
//     同一产品的库存看起来凭空翻倍；
//   - 唯一索引从三列变四列，AutoMigrate 不改已存在索引，必须显式 DROP + CREATE。
//
// 所以这里先**手工装回旧索引**、造 0 号仓的存量数据，再跑迁移——
// 否则 openTestDB 的 AutoMigrate 会直接建出新索引，测出来的东西与线上无关。
func TestMigrateStockWarehouse(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t, &model.Warehouse{}, &model.StockMovement{}, &model.StockSnapshot{}).
		WithContext(ctx)

	// 把索引还原成迁移前的样子：(product_id, deleted_at)。
	// 这一步是整条测试的前提，失败就没必要往下跑。
	if err := db.Exec("DROP INDEX IF EXISTS uni_stock_snapshot_product").Error; err != nil {
		t.Fatalf("删除迁移后的索引: %v", err)
	}
	if err := db.Exec(
		"CREATE UNIQUE INDEX uni_stock_snapshot_product ON inv_stock_snapshot (product_id, deleted_at)",
	).Error; err != nil {
		t.Fatalf("装回旧的三列索引: %v", err)
	}

	// 默认仓必须先存在：迁移要拿它的 id 去回填。
	wh := &model.Warehouse{Code: "MAIN", Name: "主仓", IsDefault: true, Status: model.EnableStatusOn}
	if err := db.Create(wh).Error; err != nil {
		t.Fatalf("建默认仓: %v", err)
	}
	other := &model.Warehouse{Code: "B", Name: "分仓", Status: model.EnableStatusOn}
	if err := db.Create(other).Error; err != nil {
		t.Fatalf("建第二个仓: %v", err)
	}

	// 造「迁移前」的数据：warehouse_id 全为 0。
	move := &model.StockMovement{
		DocNo:      "MV-OLD-001",
		ProductID:  7,
		SKUCode:    "SKU-1",
		Type:       model.StockMovementPurchase,
		Direction:  model.StockDirectionIn,
		Qty:        money.FromInt(50),
		OccurredAt: model.DateOnly(time.Now()),
	}
	if err := db.Create(move).Error; err != nil {
		t.Fatalf("建存量流水: %v", err)
	}
	snap := &model.StockSnapshot{ProductID: 7, Qty: money.FromInt(50)}
	if err := db.Create(snap).Error; err != nil {
		t.Fatalf("建存量快照: %v", err)
	}
	if snap.WarehouseID != 0 {
		t.Fatalf("存量快照的仓库应为 0，实际 %d", snap.WarehouseID)
	}

	// 旧索引下，同一产品在第二个仓插入快照必然失败。
	// 这条断言同时证明上面确实装回了旧索引——否则整条测试就是自说自话。
	legacy := &model.StockSnapshot{ProductID: 7, WarehouseID: other.ID, Qty: money.FromInt(20)}
	if err := db.Create(legacy).Error; err == nil {
		t.Fatalf("旧索引下第二个仓的快照竟然插进去了，说明旧索引没装回去")
	}

	if err := repository.MigrateStockWarehouse(db); err != nil {
		t.Fatalf("迁移失败: %v", err)
	}

	var gotMove model.StockMovement
	if err := db.First(&gotMove, move.ID).Error; err != nil {
		t.Fatalf("读回流水: %v", err)
	}
	if gotMove.WarehouseID != wh.ID {
		t.Errorf("流水应回填到默认仓 %d，实际 %d", wh.ID, gotMove.WarehouseID)
	}

	var gotSnap model.StockSnapshot
	if err := db.First(&gotSnap, snap.ID).Error; err != nil {
		t.Fatalf("读回快照: %v", err)
	}
	if gotSnap.WarehouseID != wh.ID {
		t.Errorf("快照应回填到默认仓 %d，实际 %d", wh.ID, gotSnap.WarehouseID)
	}

	// 索引必须真的变成四列，而不只是"函数没报错"。
	cols := make([]struct{ Name string }, 0, 4)
	if err := db.Raw("PRAGMA index_info('uni_stock_snapshot_product')").Scan(&cols).Error; err != nil {
		t.Fatalf("读索引列: %v", err)
	}
	hasWarehouse := false
	for _, c := range cols {
		if c.Name == "warehouse_id" {
			hasWarehouse = true
		}
	}
	if !hasWarehouse {
		t.Errorf("迁移后的索引应包含 warehouse_id，实际列 %v", cols)
	}

	// 迁移之后，同一产品在另一个仓可以再有一行快照——库存不再凭空翻倍。
	if err := db.Create(legacy).Error; err != nil {
		t.Fatalf("第二个仓的快照插入失败（唯一索引还是旧的三列？）: %v", err)
	}

	// 幂等：再跑一次不该失败，也不该把数据改坏。
	if err := repository.MigrateStockWarehouse(db); err != nil {
		t.Fatalf("第二次迁移失败（应幂等）: %v", err)
	}
	if err := db.First(&gotSnap, snap.ID).Error; err != nil {
		t.Fatalf("二次迁移后读回快照: %v", err)
	}
	if gotSnap.WarehouseID != wh.ID {
		t.Errorf("二次迁移不该改仓库，实际 %d", gotSnap.WarehouseID)
	}
	var count int64
	if err := db.Model(&model.StockSnapshot{}).Count(&count).Error; err != nil {
		t.Fatalf("统计快照: %v", err)
	}
	if count != 2 {
		t.Errorf("二次迁移不该增删快照，实际 %d 行", count)
	}
}
