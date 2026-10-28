// Package database 负责建立数据库连接与执行迁移。
package database

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
	"gorm.io/gorm/schema"

	"github.com/edwinhuish/gowork/internal/config"
)

// Open 按配置打开数据库连接并设置连接池。
func Open(cfg config.Database, log *slog.Logger) (*gorm.DB, error) {
	gormCfg := &gorm.Config{
		// 只记录慢查询与错误，避免业务日志被 SQL 淹没。
		Logger: gormlogger.Default.LogMode(gormlogger.Warn),
		// ERP 业务里表名用单数更自然：user / product / supplier。
		NamingStrategy: schema.NamingStrategy{SingularTable: true},
	}

	var dialector gorm.Dialector
	switch cfg.Driver {
	case "mysql":
		dialector = mysql.Open(cfg.DSN)
	case "sqlite":
		dialector = sqlite.Open(cfg.DSN)
	default:
		return nil, fmt.Errorf("不支持的数据库驱动: %s", cfg.Driver)
	}

	db, err := gorm.Open(dialector, gormCfg)
	if err != nil {
		return nil, fmt.Errorf("连接 %s 失败: %w", cfg.Driver, err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("获取底层连接池失败: %w", err)
	}
	sqlDB.SetMaxOpenConns(cfg.MaxOpen)
	sqlDB.SetMaxIdleConns(cfg.MaxIdle)
	sqlDB.SetConnMaxLifetime(time.Duration(cfg.MaxLifetime) * time.Second)

	if err := sqlDB.Ping(); err != nil {
		return nil, fmt.Errorf("数据库 ping 失败: %w", err)
	}

	log.Info("数据库已连接", "driver", cfg.Driver)
	return db, nil
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
