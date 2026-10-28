// Package database 负责建立数据库连接与执行迁移。
//
// 三种驱动（postgres / mysql / sqlite）共用同一套上层代码：
// 差异只在这里消化——连接串、连接池默认值、日志级别、方言细节，
// 业务层拿到的始终是一个 *gorm.DB。
package database

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
	"gorm.io/gorm/schema"

	"github.com/edwinhuish/gowork/internal/config"
)

// Open 按配置打开数据库连接并设置连接池。
func Open(cfg config.Database, log *slog.Logger) (*gorm.DB, error) {
	dsn, err := prepareDSN(cfg)
	if err != nil {
		return nil, err
	}

	gormCfg := &gorm.Config{
		Logger: gormlogger.Default.LogMode(logLevel(cfg.LogLevel)),
		// ERP 业务里表名用单数更自然：user / product / supplier。
		NamingStrategy: schema.NamingStrategy{SingularTable: true},
		// 关闭 gorm 的默认事务包装：单条写入本来就在隐式事务里，
		// 多一层显式事务会让 sqlite 的写锁持有时间变长（三库里只有它对此敏感）。
		SkipDefaultTransaction: cfg.Driver == config.DriverSQLite,
	}

	dialector, err := newDialector(cfg.Driver, dsn)
	if err != nil {
		return nil, err
	}

	db, err := gorm.Open(dialector, gormCfg)
	if err != nil {
		return nil, fmt.Errorf("连接 %s 失败: %w", cfg.Driver, err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("获取底层连接池失败: %w", err)
	}

	maxOpen, maxIdle := cfg.MaxOpen, cfg.MaxIdle
	if cfg.Driver == config.DriverSQLite {
		// sqlite 是单写者模型：连接数开太大只会在并发写上抢锁。
		// 读并发靠 WAL，不靠连接数，所以这里收敛到个位数。
		maxOpen = min(maxOpen, 8)
		maxIdle = min(maxIdle, 4)
	}
	sqlDB.SetMaxOpenConns(maxOpen)
	sqlDB.SetMaxIdleConns(maxIdle)
	sqlDB.SetConnMaxLifetime(time.Duration(cfg.MaxLifetime) * time.Second)

	if err := sqlDB.Ping(); err != nil {
		return nil, fmt.Errorf("数据库 ping 失败: %w", err)
	}

	log.Info("数据库已连接", "driver", cfg.Driver, "max_open", maxOpen)
	return db, nil
}

func newDialector(driver, dsn string) (gorm.Dialector, error) {
	switch driver {
	case config.DriverPostgres:
		return postgres.Open(dsn), nil
	case config.DriverMySQL:
		return mysql.Open(dsn), nil
	case config.DriverSQLite:
		return sqlite.Open(dsn), nil
	default:
		return nil, fmt.Errorf("不支持的数据库驱动: %s", driver)
	}
}

// prepareDSN 做驱动特有的事前准备，目前只有 sqlite 需要建目录。
func prepareDSN(cfg config.Database) (string, error) {
	dsn := cfg.ResolvedDSN()
	if cfg.Driver != config.DriverSQLite {
		return dsn, nil
	}

	path := sqliteFilePath(dsn)
	if path == "" || path == ":memory:" {
		return dsn, nil
	}
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return "", fmt.Errorf("创建 sqlite 目录 %s: %w", dir, err)
		}
	}
	return dsn, nil
}

// sqliteFilePath 从 DSN 里取出真实文件路径，兼容 file:xxx?pragma 与裸路径两种写法。
func sqliteFilePath(dsn string) string {
	s := strings.TrimPrefix(dsn, "file:")
	if i := strings.IndexByte(s, '?'); i >= 0 {
		s = s[:i]
	}
	return s
}

// AutoMigrate 同步表结构。生产环境建议改为显式迁移脚本，此处仅为骨架期便利。
func AutoMigrate(db *gorm.DB, models ...any) error {
	if err := db.AutoMigrate(models...); err != nil {
		return fmt.Errorf("自动迁移失败: %w", err)
	}
	return nil
}

// Close 关闭连接池。
func Close(db *gorm.DB) error {
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

// Dialect 返回连接实际使用的方言名，取值是 pkg/dialect 里的常量之一。
// 写方言相关 SQL 时以它为准，不要在业务代码里判断配置。
func Dialect(db *gorm.DB) string { return db.Dialector.Name() }

func logLevel(level string) gormlogger.LogLevel {
	switch strings.ToLower(level) {
	case "silent":
		return gormlogger.Silent
	case "error":
		return gormlogger.Error
	case "info":
		return gormlogger.Info
	default:
		return gormlogger.Warn
	}
}
