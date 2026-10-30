package repository

import (
	"gorm.io/gorm"

	"github.com/edwinhuish/gowork/internal/model"
)

// MigrateExchangeRateType 把汇率唯一索引换成含 rate_type 的四列（T-60）。
//
// gorm 的 AutoMigrate 只建"不存在的索引"，不会改已存在索引的列；
// 而旧索引 (currency_code, base_code, effective_date, deleted_at) 仍然在，
// 第二天想补一条买入价就会被它挡住——双价功能等于没做。
//
// 幂等：索引列已经带上 rate_type 就什么都不做，重复启动不会失败、也不会重建索引。
// 存量行由 AutoMigrate 补上 rate_type 的默认值 'middle'（列有 default），
// 所以回填这一步不需要单独写。
func MigrateExchangeRateType(db *gorm.DB) error {
	return ensureIndexColumns(db.Migrator(), &model.ExchangeRate{},
		"uni_exchange_rate_pair", "rate_type", "currency_code", "base_code", "effective_date", "deleted_at")
}
