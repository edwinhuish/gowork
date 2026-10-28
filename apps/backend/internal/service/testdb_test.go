package service

import (
	"log/slog"
	"path/filepath"
	"testing"

	"gorm.io/gorm"

	"github.com/edwinhuish/gowork/internal/config"
	"github.com/edwinhuish/gowork/internal/infra/database"
)

// openTestDB 起一个临时 sqlite 并建好给定的表。
//
// service 层的测试尽量跑在真实数据库上，而不是手写假仓储：
// 唯一索引、事务、写成一条 SQL 的自增，恰恰是假仓储会顺手抹掉的东西，
// 而并发重号、号段不跳、重复汇率这些问题全都长在那里。
func openTestDB(t *testing.T, models ...any) *gorm.DB {
	t.Helper()

	db, err := database.Open(config.Database{
		Driver:      config.DriverSQLite,
		DSN:         filepath.Join(t.TempDir(), "test.db"),
		MaxOpen:     5,
		MaxIdle:     2,
		MaxLifetime: 60,
		LogLevel:    "error",
	}, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatalf("连接 sqlite 失败: %v", err)
	}
	t.Cleanup(func() { _ = database.Close(db) })

	if err := database.AutoMigrate(db, models...); err != nil {
		t.Fatalf("建表失败: %v", err)
	}
	return db
}
