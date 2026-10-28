// Package migrate 存放一次性的数据迁移。
//
// 与 gorm 的 AutoMigrate 分工明确：AutoMigrate 只负责让表结构追上模型，
// 它不会搬数据、不会改名、更不会在两张表之间建立对应关系。
// 凡是"已有数据的形状要变"的情形，都必须写在这里，并且写成一件事一个函数、
// 可反复执行（幂等）。
package migrate

import (
	"fmt"
	"log/slog"
	"time"

	"gorm.io/gorm"
)

// Record 记录已执行的迁移，保证同一份迁移在多次启动中只跑一次。
//
// 不用"数据是否已存在"来判断是否跑过：那种判断在部分成功、手工补数、
// 换库导入等情形下都会给出错误答案，而一张记录表是明确的。
type Record struct {
	Name      string    `gorm:"primaryKey;size:64"`
	AppliedAt time.Time `gorm:"not null"`
}

func (Record) TableName() string { return "sys_migration" }

// Run 执行所有尚未应用的迁移，按注册顺序串行。
//
// 顺序有依赖时必须显式排列：后面的迁移可能建立在前面的结果上。
func Run(db *gorm.DB, log *slog.Logger) error {
	if err := ensureRecordTable(db); err != nil {
		return err
	}
	if err := MergeLegacyCustomersAndSuppliers(db, log); err != nil {
		return err
	}
	return nil
}

// ensureRecordTable 保证 sys_migration 存在。
//
// 每个迁移函数入口都调一次，而不是只在 Run 里调：迁移函数可能被测试
// 或一次性脚本直接调用，依赖"调用方先建表"这种隐含约定，
// 出错时的表现会是一个看不出所以然的 SQL 错误。
func ensureRecordTable(db *gorm.DB) error {
	if err := db.AutoMigrate(&Record{}); err != nil {
		return fmt.Errorf("创建迁移记录表: %w", err)
	}
	return nil
}

func applied(db *gorm.DB, name string) (bool, error) {
	var n int64
	if err := db.Model(&Record{}).Where("name = ?", name).Count(&n).Error; err != nil {
		return false, fmt.Errorf("查询迁移记录 %s: %w", name, err)
	}
	return n > 0, nil
}

func markApplied(db *gorm.DB, name string) error {
	if err := db.Create(&Record{Name: name, AppliedAt: time.Now()}).Error; err != nil {
		return fmt.Errorf("写入迁移记录 %s: %w", name, err)
	}
	return nil
}
