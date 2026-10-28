// Package model 定义持久化模型。
//
// 这一层只描述数据形状，不包含任何业务规则，也不依赖上层的 service / handler。
package model

import (
	"time"

	"gorm.io/gorm"
)

// 通用启用状态。
const (
	StatusDisabled int8 = 0
	StatusEnabled  int8 = 1
)

// Base 是所有业务表的公共字段。
// 软删除用 gorm.DeletedAt：删除只置位，历史单据与追溯链不会断。
type Base struct {
	ID        uint           `gorm:"primaryKey" json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}
