package migrate

import (
	"fmt"
	"log/slog"
	"time"

	"gorm.io/gorm"

	"github.com/edwinhuish/gowork/internal/model"
)

const backfillOpportunityRateLockName = "backfill_opportunity_rate_locked_on"

// BackfillOpportunityRateLockedOn 给存量商机补上折算日（T-61）。
//
// 列由 AutoMigrate 加出来是 NULL，而详情页要把这一天显示给用户看——
// NULL 会渲染成 0001-01-01，比不显示更糟：它看起来像一个真实的日期。
//
// 补 created_at 的日期，不补"今天"：那批预测确实是录入那天按当时的汇率算出来的，
// 记成今天等于凭空说"它昨天才重估过"。
//
// 只处理未删除的商机：软删的不显示，恢复时会随下一次保存按录入日补齐。
// 幂等靠 sys_migration 与"只选 NULL 的行"双保险。
func BackfillOpportunityRateLockedOn(db *gorm.DB, log *slog.Logger) error {
	if err := ensureRecordTable(db); err != nil {
		return err
	}
	// 列还没建出来时既不执行也不记账：记了账，等结构补齐就再也不会跑。
	if !db.Migrator().HasColumn(&model.Opportunity{}, "rate_locked_on") {
		log.Warn("crm_opportunity 缺少 rate_locked_on 列，跳过商机折算日回填")
		return nil
	}

	done, err := applied(db, backfillOpportunityRateLockName)
	if err != nil {
		return err
	}
	if done {
		return nil
	}

	var rows []struct {
		ID        uint
		CreatedAt time.Time
	}
	if err := db.Model(&model.Opportunity{}).
		Select("id", "created_at").
		Where("rate_locked_on IS NULL").
		Find(&rows).Error; err != nil {
		return fmt.Errorf("查询待回填折算日的商机: %w", err)
	}

	if len(rows) > 0 {
		err = db.Transaction(func(tx *gorm.DB) error {
			for _, row := range rows {
				if err := tx.Model(&model.Opportunity{}).
					Where("id = ?", row.ID).
					Update("rate_locked_on", model.DateOnly(row.CreatedAt)).Error; err != nil {
					return fmt.Errorf("回填商机 %d 的折算日: %w", row.ID, err)
				}
			}
			return nil
		})
		if err != nil {
			return err
		}
		log.Info("已回填商机折算日", "count", len(rows))
	}
	return markApplied(db, backfillOpportunityRateLockName)
}
