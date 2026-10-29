package repository

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"github.com/edwinhuish/gowork/internal/model"
)

// OpportunityStageLogRepository 只追加、不修改。
//
// 接口上就没有 Update / Delete 两个方法：阶段历史一旦能改，"这笔单三月卡在哪"
// 就没有可信的答案了。要更正只能再写一条（谁改的、为什么），不是把旧的抹掉。
type OpportunityStageLogRepository interface {
	// Create 追加一条留痕。参与外层事务（走 dbFor），与商机本身的写入同生共死。
	Create(ctx context.Context, m *model.OpportunityStageLog) error
	// ListByOpportunity 按时间正序返回，前端照着顺序排时间线，不必再排一次。
	ListByOpportunity(ctx context.Context, opportunityID uint) ([]model.OpportunityStageLog, error)
}

type opportunityStageLogRepo struct {
	db *gorm.DB
}

func NewOpportunityStageLogRepository(db *gorm.DB) OpportunityStageLogRepository {
	return &opportunityStageLogRepo{db: db}
}

func (r *opportunityStageLogRepo) Create(
	ctx context.Context, m *model.OpportunityStageLog,
) error {
	if err := dbFor(ctx, r.db).Create(m).Error; err != nil {
		return fmt.Errorf("写入商机阶段留痕: %w", err)
	}
	return nil
}

func (r *opportunityStageLogRepo) ListByOpportunity(
	ctx context.Context, opportunityID uint,
) ([]model.OpportunityStageLog, error) {
	logs := make([]model.OpportunityStageLog, 0, 8)
	err := dbFor(ctx, r.db).
		Where("opportunity_id = ?", opportunityID).
		Order("created_at ASC, id ASC").
		Find(&logs).Error
	if err != nil {
		return nil, fmt.Errorf("查询商机阶段留痕 opportunity_id=%d: %w", opportunityID, err)
	}
	return logs, nil
}
