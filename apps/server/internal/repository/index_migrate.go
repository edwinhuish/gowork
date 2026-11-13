package repository

import (
	"fmt"

	"gorm.io/gorm"
)

// ensureIndexColumns 保证索引 idxName 存在，且它的列包含 required 里的每一列。
//
// 专门给「改索引列」这类迁移用：AutoMigrate 只建不存在的索引，不改已存在索引的列，
// 所以列变了必须自己 DROP + CREATE。但**不能无条件重建**——这些函数随服务启动跑，
// 每启一次就在存量表上全表扫描建一次索引并加一把锁，库越大重启越慢。
// 列已经对得上就直接返回。
func ensureIndexColumns(m gorm.Migrator, dst any, idxName string, required ...string) error {
	if m.HasIndex(dst, idxName) {
		indexes, err := m.GetIndexes(dst)
		if err != nil {
			return fmt.Errorf("读取索引 %s: %w", idxName, err)
		}
		found := false
		for _, ix := range indexes {
			if ix.Name() != idxName {
				continue
			}
			found = true
			if hasAllColumns(ix.Columns(), required) {
				return nil
			}
		}
		if found {
			if err := m.DropIndex(dst, idxName); err != nil {
				return fmt.Errorf("删除旧索引 %s: %w", idxName, err)
			}
		}
	}
	if err := m.CreateIndex(dst, idxName); err != nil {
		return fmt.Errorf("建立索引 %s: %w", idxName, err)
	}
	return nil
}

func hasAllColumns(cols []string, required []string) bool {
	for _, want := range required {
		hit := false
		for _, col := range cols {
			if col == want {
				hit = true
				break
			}
		}
		if !hit {
			return false
		}
	}
	return true
}
