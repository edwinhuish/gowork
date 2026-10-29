package repository

import (
	"context"
	"log/slog"
	"path/filepath"
	"testing"

	"gorm.io/gorm"

	"github.com/edwinhuish/gowork/internal/config"
	"github.com/edwinhuish/gowork/internal/infra/database"
	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/errs"
)

func newDupTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := database.Open(config.Database{
		Driver:      config.DriverSQLite,
		DSN:         filepath.Join(t.TempDir(), "dup.db"),
		MaxOpen:     2,
		MaxIdle:     1,
		MaxLifetime: 60,
		LogLevel:    "error",
	}, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatalf("打开测试数据库: %v", err)
	}
	t.Cleanup(func() { _ = database.Close(db) })

	if err := database.AutoMigrate(db, &model.Product{}); err != nil {
		t.Fatalf("建表: %v", err)
	}
	return db
}

// 唯一索引被撞上时必须是 409，而不是 500。
//
// 前置的 Exists 校验挡不住并发：两个请求同时通过校验、同时写入，第二个必然撞约束。
// 不翻译的话用户看到的是一次"服务器内部错误"，而它其实是"这个编码已经有人用了"。
func TestDuplicateKeyReturnsConflict(t *testing.T) {
	db := newDupTestDB(t)
	repo := NewProductRepository(db)
	ctx := context.Background()

	first := &model.Product{
		SKUCode: "DUP-0001", NameCN: "先建的", Unit: "PCS", Status: model.EnableStatusOn,
	}
	if err := repo.Create(ctx, first); err != nil {
		t.Fatalf("建第一条产品失败: %v", err)
	}

	err := repo.Create(ctx, &model.Product{
		SKUCode: "DUP-0001", NameCN: "撞号的", Unit: "PCS", Status: model.EnableStatusOn,
	})
	if err == nil {
		t.Fatal("同 SKU 的第二条应被唯一索引拒绝")
	}
	e, ok := errs.From(err)
	if !ok || e.Code != errs.CodeConflict {
		t.Fatalf("期望 40901，实际 %v", err)
	}
}

// 事务里的 tx.Create 走的是同一套回调：这条路径最容易漏，
// 因为它不经过 Base 的通用方法，而是各仓储自己开的 Transaction。
func TestDuplicateKeyInsideTransactionReturnsConflict(t *testing.T) {
	db := newDupTestDB(t)
	repo := NewProductRepository(db)
	txs := NewTxManager(db)
	ctx := context.Background()

	if err := repo.Create(ctx, &model.Product{
		SKUCode: "DUP-0002", NameCN: "先建的", Unit: "PCS", Status: model.EnableStatusOn,
	}); err != nil {
		t.Fatalf("建第一条产品失败: %v", err)
	}

	err := txs.Transaction(ctx, func(ctx context.Context) error {
		return repo.Create(ctx, &model.Product{
			SKUCode: "DUP-0002", NameCN: "事务里撞号的", Unit: "PCS", Status: model.EnableStatusOn,
		})
	})
	if err == nil {
		t.Fatal("事务内的同 SKU 写入应被拒绝")
	}
	e, ok := errs.From(err)
	if !ok || e.Code != errs.CodeConflict {
		t.Fatalf("期望 40901，实际 %v", err)
	}

	// 事务回滚了：第一条仍在，撞号的那条没落库。
	var n int64
	if err := db.Model(&model.Product{}).Count(&n).Error; err != nil {
		t.Fatalf("统计产品: %v", err)
	}
	if n != 1 {
		t.Errorf("产品数 = %d，期望 1（撞号的那条应随事务回滚）", n)
	}
}
