package migrate

import (
	"fmt"
	"log/slog"

	"gorm.io/gorm"

	"github.com/edwinhuish/gowork/internal/model"
	codenorm "github.com/edwinhuish/gowork/internal/pkg/code"
)

const backfillSupplierSKUNormName = "backfill_supplier_sku_norm"

// BackfillSupplierSKUNorm 给存量的货源补上 supplier_sku_norm。
//
// 列由 AutoMigrate 加出来，但加列不会填值：存量行的 norm 是空串，
// 于是"按供应商货号搜产品"对老数据静默失效——搜不到时没人会想到
// 是少了一次回填。新录入的行由 service 计算，这里只补历史。
//
// 只处理未删除的货源：软删的行不参与检索，真被恢复时会随下一次编辑重算。
// 幂等靠 sys_migration 与"只选 norm 为空的行"双保险。
func BackfillSupplierSKUNorm(db *gorm.DB, log *slog.Logger) error {
	if err := ensureRecordTable(db); err != nil {
		return err
	}
	// 列还没建出来（关掉 AutoMigrate 的环境）时既不执行也不记账：
	// 记了账，等结构补齐之后它就再也不会跑。
	if !db.Migrator().HasColumn(&model.ProductSource{}, "supplier_sku_norm") {
		log.Warn("md_product_source 缺少 supplier_sku_norm 列，跳过货号回填")
		return nil
	}

	done, err := applied(db, backfillSupplierSKUNormName)
	if err != nil {
		return err
	}
	if done {
		return nil
	}

	var rows []struct {
		ID          uint
		SupplierSKU string
	}
	// 空货号跳过：归一化后仍是空串，回填没有意义，也会让"没有货号"的行
	// 看上去像是处理过了。
	if err := db.Model(&model.ProductSource{}).
		Select("id", "supplier_sku").
		Where("supplier_sku <> ''").
		Where("supplier_sku_norm IS NULL OR supplier_sku_norm = ''").
		Find(&rows).Error; err != nil {
		return fmt.Errorf("查询待回填的供应商货号: %w", err)
	}

	if len(rows) > 0 {
		err = db.Transaction(func(tx *gorm.DB) error {
			for _, row := range rows {
				if err := tx.Model(&model.ProductSource{}).
					Where("id = ?", row.ID).
					Update("supplier_sku_norm", codenorm.Normalize(row.SupplierSKU)).Error; err != nil {
					return fmt.Errorf("回填货源 %d 的货号归一化值: %w", row.ID, err)
				}
			}
			return nil
		})
		if err != nil {
			return err
		}
		log.Info("已回填供应商货号归一化值", "count", len(rows))
	}
	return markApplied(db, backfillSupplierSKUNormName)
}
