// Package model 定义持久化模型。
//
// 这一层只描述数据形状，不包含任何业务规则，也不依赖上层的 service / handler。
package model

import (
	"time"

	"gorm.io/gorm"
)

// EnableStatus 是主数据与账号的启用状态。
//
// 它只回答一个问题：**这条记录还能不能被业务引用**。
// 不表达业务流转——订单、出货这类单据的状态是另一套语义
// （见 docs/glossary.md「单据状态」），将来定义 DocumentStatus 时
// 不要复用本类型，也不要让一张表同时承载两种 status。
type EnableStatus int8

const (
	// EnableStatusOff 停用：记录保留，但不出现在可选列表里，不能新建引用。
	EnableStatusOff EnableStatus = 0
	// EnableStatusOn 启用。
	EnableStatusOn EnableStatus = 1
)

// Base 是所有业务表的公共字段。
// 软删除用 gorm.DeletedAt：删除只置位，历史单据与追溯链不会断。
type Base struct {
	ID        uint           `gorm:"primaryKey" json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}
