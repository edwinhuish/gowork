package service

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/money"
	"github.com/edwinhuish/gowork/internal/repository"
)

// TestStockWarehouseIsolation 是 T-56 的验收点。
//
// 三件事一起验：
//   - 两个仓的库存各自增减、互不影响（快照唯一键是 warehouse + product）；
//   - 调拨只改分布不改总量；
//   - 同一产品在两个仓各有一行快照，而不是一行被两边争用。
//
// 这几条此前只有"编译通过 + 迁移函数幂等"两点保证，而它们动的是库存底表：
// 唯一键改错时症状是 ON CONFLICT 报错或库存凭空翻倍，都不会在启动时暴露。
func TestStockWarehouseIsolation(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t, &model.Product{}, &model.StockMovement{}, &model.StockSnapshot{},
		&model.Warehouse{}, &model.StockDocument{}, &model.StockDocumentItem{})
	repo := repository.NewStockRepository(db)

	// 两个仓：A 是默认仓（defaultWarehouseID 会取它），B 不是。
	a := &model.Warehouse{Code: "A", Name: "主仓", IsDefault: true, Status: model.EnableStatusOn}
	if err := db.Create(a).Error; err != nil {
		t.Fatalf("建仓 A: %v", err)
	}
	b := &model.Warehouse{Code: "B", Name: "分仓", Status: model.EnableStatusOn}
	if err := db.Create(b).Error; err != nil {
		t.Fatalf("建仓 B: %v", err)
	}

	const productID uint = 42
	seq := 0
	move := func(warehouseID uint, dir model.StockDirection, qty string) {
		t.Helper()
		seq++
		m := &model.StockMovement{
			DocNo:       fmt.Sprintf("MV-TEST-%03d", seq),
			ProductID:   productID,
			SKUCode:     "SKU-1",
			WarehouseID: warehouseID,
			Type:        model.StockMovementPurchase,
			Direction:   dir,
			Qty:         money.MustParse(qty),
			OccurredAt:  model.DateOnly(time.Now()),
		}
		if err := repo.CreateMovement(ctx, m); err != nil {
			t.Fatalf("写入流水（仓 %d，%s）: %v", warehouseID, qty, err)
		}
	}

	snapshotQty := func(warehouseID uint) money.Decimal {
		t.Helper()
		var rows []struct{ Qty money.Decimal }
		if err := db.Raw(
			"SELECT qty FROM inv_stock_snapshot WHERE product_id = ? AND warehouse_id = ? AND deleted_at = 0",
			productID, warehouseID).Scan(&rows).Error; err != nil {
			t.Fatalf("查快照（仓 %d）: %v", warehouseID, err)
		}
		if len(rows) == 0 {
			return money.Zero()
		}
		return rows[0].Qty
	}

	// 入库：A 收 100，B 收 30。
	move(a.ID, model.StockDirectionIn, "100")
	move(b.ID, model.StockDirectionIn, "30")

	if got := snapshotQty(a.ID); got.Cmp(money.MustParse("100")) != 0 {
		t.Errorf("仓 A 应为 100，得到 %s", got)
	}
	if got := snapshotQty(b.ID); got.Cmp(money.MustParse("30")) != 0 {
		t.Errorf("仓 B 应为 30，得到 %s", got)
	}

	// 同一产品在两个仓各一行：唯一键是 (warehouse_id, product_id, deleted_at)，
	// 不带 warehouse 的话第二行会撞唯一约束。
	var rowCount int64
	if err := db.Raw(
		"SELECT COUNT(*) FROM inv_stock_snapshot WHERE product_id = ? AND deleted_at = 0",
		productID).Scan(&rowCount).Error; err != nil {
		t.Fatalf("数快照行: %v", err)
	}
	if rowCount != 2 {
		t.Fatalf("同一产品在两个仓应有两行快照，实际 %d 行", rowCount)
	}

	// 调拨：A 出 40、B 入 40。总量不变，分布变了。
	move(a.ID, model.StockDirectionOut, "40")
	move(b.ID, model.StockDirectionIn, "40")

	qa, qb := snapshotQty(a.ID), snapshotQty(b.ID)
	if qa.Cmp(money.MustParse("60")) != 0 {
		t.Errorf("调拨后仓 A 应为 60，得到 %s", qa)
	}
	if qb.Cmp(money.MustParse("70")) != 0 {
		t.Errorf("调拨后仓 B 应为 70，得到 %s", qb)
	}
	total := qa.Add(qb)
	if total.Cmp(money.MustParse("130")) != 0 {
		t.Errorf("调拨不该改变总量：应为 130，得到 %s", total)
	}
}
