package database

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/edwinhuish/gowork/internal/config"
	"github.com/edwinhuish/gowork/internal/model"
)

// legacyProbe 模拟改造前的表：deleted_at 是可空的时间戳。
type legacyProbe struct {
	ID        uint       `gorm:"primaryKey"`
	Code      string     `gorm:"size:32;not null"`
	DeletedAt *time.Time `gorm:"column:deleted_at"`
}

func (legacyProbe) TableName() string { return "probe_soft_delete_legacy" }

// currentProbe 是改造后的形状：整数标记，并与业务列组成唯一索引。
type currentProbe struct {
	ID        uint            `gorm:"primaryKey"`
	Code      string          `gorm:"size:32;not null;uniqueIndex:uni_probe_code,priority:1"`
	DeletedAt model.DeletedAt `gorm:"softDelete:milli;not null;default:0;uniqueIndex:uni_probe_code,priority:2"`
}

func (currentProbe) TableName() string { return "probe_soft_delete_legacy" }

// 升级路径必须在三种库上都成立：老库里软删标记是时间戳，改完要变成 Unix 毫秒，
// 且"哪些行被删了"不能变——未删除的是 0，已删除的保留原时间点。
func TestPrepareSoftDeleteUpgradesLegacyColumn(t *testing.T) {
	cases := []struct {
		name   string
		cfg    config.Database
		skipIf func() string
	}{
		{
			name: "sqlite",
			cfg: config.Database{
				Driver:      config.DriverSQLite,
				DSN:         filepath.Join(t.TempDir(), "probe.db"),
				MaxOpen:     2,
				MaxIdle:     1,
				MaxLifetime: 60,
				LogLevel:    "error",
			},
		},
		{
			name: "postgres",
			cfg: config.Database{
				Driver: config.DriverPostgres, DSN: os.Getenv("GOWORK_TEST_PG_DSN"),
				MaxOpen: 2, MaxIdle: 1, MaxLifetime: 60, LogLevel: "error",
			},
			skipIf: func() string {
				if os.Getenv("GOWORK_TEST_PG_DSN") == "" {
					return "未设置 GOWORK_TEST_PG_DSN"
				}
				return ""
			},
		},
		{
			name: "mysql",
			cfg: config.Database{
				Driver: config.DriverMySQL, DSN: os.Getenv("GOWORK_TEST_MYSQL_DSN"),
				MaxOpen: 2, MaxIdle: 1, MaxLifetime: 60, LogLevel: "error",
			},
			skipIf: func() string {
				if os.Getenv("GOWORK_TEST_MYSQL_DSN") == "" {
					return "未设置 GOWORK_TEST_MYSQL_DSN"
				}
				return ""
			},
		},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			if c.skipIf != nil {
				if reason := c.skipIf(); reason != "" {
					t.Skip(reason)
				}
			}

			// 造数据用一条连接，升级用另一条：真实场景里升级发生在进程启动、
			// 此前没有任何语句，而 pgx 会按 SQL 文本缓存参数类型——
			// 同一条连接上"先用时间戳插入、再把列改成整数"会让缓存与实际脱节。
			seed, err := Open(c.cfg, slog.New(slog.DiscardHandler))
			if err != nil {
				t.Fatalf("连接: %v", err)
			}
			if err := seed.Migrator().DropTable(&legacyProbe{}); err != nil {
				t.Fatalf("清理旧表: %v", err)
			}
			if err := seed.AutoMigrate(&legacyProbe{}); err != nil {
				t.Fatalf("建旧表: %v", err)
			}

			deletedAt := time.Now().Add(-24 * time.Hour).UTC().Truncate(time.Millisecond)
			if err := seed.Create(&legacyProbe{Code: "KEEP", DeletedAt: nil}).Error; err != nil {
				t.Fatalf("写未删行: %v", err)
			}
			if err := seed.Create(&legacyProbe{Code: "GONE", DeletedAt: &deletedAt}).Error; err != nil {
				t.Fatalf("写已删行: %v", err)
			}
			_ = Close(seed)

			db, err := Open(c.cfg, slog.New(slog.DiscardHandler))
			if err != nil {
				t.Fatalf("重新连接: %v", err)
			}
			t.Cleanup(func() { _ = Close(db) })
			t.Cleanup(func() { _ = db.Migrator().DropTable(&currentProbe{}) })

			// AutoMigrate 内部先修列的表示，再按新模型同步结构。
			if err := AutoMigrate(db, &currentProbe{}); err != nil {
				t.Fatalf("升级: %v", err)
			}

			var rows []struct {
				Code      string
				DeletedAt int64
			}
			if err := db.Table("probe_soft_delete_legacy").
				Select("code", "deleted_at").Order("code").Scan(&rows).Error; err != nil {
				t.Fatalf("读回数据: %v", err)
			}
			if len(rows) != 2 {
				t.Fatalf("应有 2 行，得到 %d", len(rows))
			}
			// 按 code 排序：GONE 在前，KEEP 在后。
			if rows[0].Code != "GONE" || rows[1].Code != "KEEP" {
				t.Fatalf("行序异常: %+v", rows)
			}
			// 容差 2ms：sqlite 用 julianday 换算，浮点末位会差一点点。
			if want := deletedAt.UnixMilli(); rows[0].DeletedAt < want-2 || rows[0].DeletedAt > want+2 {
				t.Errorf("已删除行的标记 = %d，期望约 %d（原时间点的毫秒）", rows[0].DeletedAt, want)
			}
			if rows[1].DeletedAt != 0 {
				t.Errorf("未删除行的标记 = %d，期望 0", rows[1].DeletedAt)
			}

			// 升级后同编码可重建：这既是列改造的结果，也是索引改造的结果。
			if err := db.Create(&currentProbe{Code: "GONE"}).Error; err != nil {
				t.Errorf("升级后同编码无法重建: %v", err)
			}
		})
	}
}

// 改造过的库上再来一次，不该再拆索引。
//
// 汇率（uni_exchange_rate_pair）与产品编号（uni_product_code）这两个索引改造前后同名，
// 只判"索引存在"的话，它们每次启动都会被删一次、再由 AutoMigrate 重建一次——
// 存量表上那就是一次全表扫描加一把锁，而这段跑在服务启动路径上。
// 判据用 sqlite 的 schema_version：这个函数除了 DDL 什么都不做，它一动就说明真的动了表。
func TestPrepareSoftDeleteLeavesMigratedIndexesAlone(t *testing.T) {
	db, err := Open(config.Database{
		Driver:      config.DriverSQLite,
		DSN:         filepath.Join(t.TempDir(), "migrated.db"),
		MaxOpen:     2,
		MaxIdle:     1,
		MaxLifetime: 60,
		LogLevel:    "error",
	}, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatalf("连接: %v", err)
	}
	t.Cleanup(func() { _ = Close(db) })

	if err := AutoMigrate(db, &model.ExchangeRate{}); err != nil {
		t.Fatalf("建表: %v", err)
	}
	if !db.Migrator().HasIndex(&model.ExchangeRate{}, "uni_exchange_rate_pair") {
		t.Fatal("前提不成立：迁移后应有汇率唯一索引")
	}
	before := sqliteSchemaVersion(t, db)

	if err := prepareSoftDelete(db, &model.ExchangeRate{}); err != nil {
		t.Fatalf("二次准备失败（应空转）: %v", err)
	}
	if after := sqliteSchemaVersion(t, db); after != before {
		t.Errorf("改造过的库上又动了表结构（schema_version %d → %d）：每次启动都会重建索引", before, after)
	}
	if !db.Migrator().HasIndex(&model.ExchangeRate{}, "uni_exchange_rate_pair") {
		t.Error("索引被删掉了，AutoMigrate 之外没人会把它建回来")
	}

	// 判据自检：手工删一次索引，schema_version 必须跟着变，否则上面那条断言没有判别力。
	if err := db.Migrator().DropIndex(&model.ExchangeRate{}, "uni_exchange_rate_pair"); err != nil {
		t.Fatalf("自检:删索引: %v", err)
	}
	if v := sqliteSchemaVersion(t, db); v == before {
		t.Errorf("删了索引但 schema_version 没变（%d），这条测试的判据无效", v)
	}
}

// sqliteSchemaVersion 读 schema_version：任何 DDL 都会让它加一。
func sqliteSchemaVersion(t *testing.T, db *gorm.DB) int {
	t.Helper()

	var v int
	if err := db.Raw("PRAGMA schema_version").Scan(&v).Error; err != nil {
		t.Fatalf("读 schema_version: %v", err)
	}
	return v
}

// 空库（列还不存在）时应当是纯空转，不报错、不做任何结构改动。
func TestPrepareSoftDeleteSkipsFreshDatabase(t *testing.T) {
	db, err := Open(config.Database{
		Driver:      config.DriverSQLite,
		DSN:         filepath.Join(t.TempDir(), "fresh.db"),
		MaxOpen:     2,
		MaxIdle:     1,
		MaxLifetime: 60,
		LogLevel:    "error",
	}, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatalf("连接: %v", err)
	}
	t.Cleanup(func() { _ = Close(db) })

	if err := prepareSoftDelete(db, &currentProbe{}, &model.Product{}); err != nil {
		t.Fatalf("空库应直接返回: %v", err)
	}
	if db.Migrator().HasTable(&currentProbe{}) {
		t.Error("空转不应建表")
	}
}
