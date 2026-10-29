package repository

import (
	"context"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/edwinhuish/gowork/internal/config"
	"github.com/edwinhuish/gowork/internal/infra/database"
	"github.com/edwinhuish/gowork/internal/model"
)

func newSoftDeleteDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := database.Open(config.Database{
		Driver:      config.DriverSQLite,
		DSN:         filepath.Join(t.TempDir(), "softdelete.db"),
		MaxOpen:     2,
		MaxIdle:     1,
		MaxLifetime: 60,
		LogLevel:    "error",
	}, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatalf("打开测试数据库: %v", err)
	}
	t.Cleanup(func() { _ = database.Close(db) })

	if err := database.AutoMigrate(db, &model.Product{}, &model.Partner{}); err != nil {
		t.Fatalf("建表: %v", err)
	}
	return db
}

func mustCreateProduct(t *testing.T, repo ProductRepository, sku, name string) *model.Product {
	t.Helper()
	p := &model.Product{SKUCode: sku, NameCN: name, Unit: "PCS", Status: model.EnableStatusOn}
	if err := repo.Create(context.Background(), p); err != nil {
		t.Fatalf("建产品 %s: %v", sku, err)
	}
	return p
}

// 删除写的是 Unix 毫秒整数，不是时间戳；查询默认把它过滤掉。
func TestSoftDeleteWritesMarkerAndFilters(t *testing.T) {
	db := newSoftDeleteDB(t)
	repo := NewProductRepository(db)
	ctx := context.Background()

	p := mustCreateProduct(t, repo, "SD-0001", "被删的")
	before := time.Now().UnixMilli()

	if err := repo.Delete(ctx, p.ID); err != nil {
		t.Fatalf("删除产品: %v", err)
	}

	// 普通查询看不到。
	if _, err := repo.FindByID(ctx, p.ID); err == nil {
		t.Fatal("软删的产品仍能被普通查询读到")
	}

	// 带上 Unscoped 才看得见，标记是毫秒时间戳且落在删除动作附近。
	var raw model.Product
	if err := db.Unscoped().First(&raw, p.ID).Error; err != nil {
		t.Fatalf("Unscoped 读取: %v", err)
	}
	if raw.DeletedAt == 0 {
		t.Fatal("删除标记没写进去")
	}
	marked := int64(raw.DeletedAt)
	if marked < before-60_000 || marked > time.Now().UnixMilli()+60_000 {
		t.Errorf("删除标记 = %d，不像 Unix 毫秒时间戳", marked)
	}
}

// 未删除的行 deleted_at 恒为 0，所以复合唯一索引表达的是「未删才唯一」：
// 删掉之后同一个 SKU 可以立刻重建，两行共存。
func TestSoftDeleteAllowsRecreateSameSKU(t *testing.T) {
	db := newSoftDeleteDB(t)
	repo := NewProductRepository(db)
	ctx := context.Background()

	first := mustCreateProduct(t, repo, "SD-0002", "第一版")
	if err := repo.Delete(ctx, first.ID); err != nil {
		t.Fatalf("删除: %v", err)
	}

	second := mustCreateProduct(t, repo, "SD-0002", "重建的")
	if second.ID == first.ID {
		t.Fatal("重建应是一条新记录")
	}

	var visible int64
	if err := db.Model(&model.Product{}).Where("sku_code = ?", "SD-0002").Count(&visible).Error; err != nil {
		t.Fatalf("统计可见行: %v", err)
	}
	if visible != 1 {
		t.Errorf("普通查询可见 %d 行，期望 1 行", visible)
	}

	var all int64
	if err := db.Unscoped().Model(&model.Product{}).
		Where("sku_code = ?", "SD-0002").Count(&all).Error; err != nil {
		t.Fatalf("统计全部行: %v", err)
	}
	if all != 2 {
		t.Errorf("Unscoped 共 %d 行，期望 2 行（一删一未删）", all)
	}
}

// 恢复就是把标记写回 0；但如果此时已经用同一个 SKU 建了新记录，
// 恢复会撞唯一索引——这是"未删才唯一"的必然结果，必须撞得明确。
func TestSoftDeleteRestoreConflictsWithRecreatedSKU(t *testing.T) {
	db := newSoftDeleteDB(t)
	repo := NewProductRepository(db)
	ctx := context.Background()

	first := mustCreateProduct(t, repo, "SD-0003", "第一版")
	if err := repo.Delete(ctx, first.ID); err != nil {
		t.Fatalf("删除: %v", err)
	}
	mustCreateProduct(t, repo, "SD-0003", "重建的")

	err := db.Unscoped().Model(&model.Product{}).
		Where("id = ?", first.ID).Update("deleted_at", 0).Error
	if err == nil {
		t.Fatal("同 SKU 已有未删记录时，恢复旧记录应当被唯一索引拒绝")
	}

	// 删掉占用 SKU 的那条之后，恢复就能成功：标记写回 0 即可。
	var second model.Product
	if err := db.Where("sku_code = ?", "SD-0003").First(&second).Error; err != nil {
		t.Fatalf("读取重建的那条: %v", err)
	}
	if err := repo.Delete(ctx, second.ID); err != nil {
		t.Fatalf("删除重建的那条: %v", err)
	}
	if err := db.Unscoped().Model(&model.Product{}).
		Where("id = ?", first.ID).Update("deleted_at", 0).Error; err != nil {
		t.Fatalf("恢复旧记录: %v", err)
	}
	if _, err := repo.FindByID(ctx, first.ID); err != nil {
		t.Errorf("恢复后应能被普通查询读到: %v", err)
	}
}

// 物理删除是软删之外的唯一出口：Unscoped().Delete 之后连 Unscoped 都查不到，
// 而软删只是标记。两者的边界必须一眼分得清。
func TestSoftDeleteVersusHardDelete(t *testing.T) {
	db := newSoftDeleteDB(t)
	repo := NewProductRepository(db)
	ctx := context.Background()

	soft := mustCreateProduct(t, repo, "SD-0004", "软的")
	if err := repo.Delete(ctx, soft.ID); err != nil {
		t.Fatalf("软删: %v", err)
	}
	hard := mustCreateProduct(t, repo, "SD-0005", "硬的")
	if err := db.Unscoped().Delete(&model.Product{}, hard.ID).Error; err != nil {
		t.Fatalf("物理删除: %v", err)
	}

	var row model.Product
	if err := db.Unscoped().First(&row, soft.ID).Error; err != nil {
		t.Errorf("软删的记录在 Unscoped 下应仍在: %v", err)
	}
	if err := db.Unscoped().First(&model.Product{}, hard.ID).Error; err == nil {
		t.Error("物理删除的记录不该还能读到")
	}
}
