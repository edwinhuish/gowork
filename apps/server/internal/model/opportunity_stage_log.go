package model

import "time"

// OpportunityStageLog 是一次商机阶段变更的留痕（T-61）。
//
// 不嵌 Base，与 AuditLog 同一个理由：这条记录的价值在于**改不了**。
// 阶段历史被改过一次，后面每一行都不值得信——而它要回答的恰恰是
// "这笔单子在三月的时候卡在哪一步"。所以仓储只给 Create 与 List。
//
// 为什么不用 sys_audit_log 代替：审计表记的是"谁提交了什么请求"，
// 请求体里只有新值；"从哪个阶段来的"要自己回放历史才能算出来，
// 而回放要求每次 PUT 都完整、不丢字段——那是另一条脆弱的路。
type OpportunityStageLog struct {
	ID            uint `gorm:"primaryKey" json:"id"`
	OpportunityID uint `gorm:"index;not null" json:"opportunity_id"`

	// FromStage 为 0 表示这条商机刚建出来，没有"从哪个阶段来"。
	FromStage OpportunityStage `gorm:"not null;default:0" json:"from_stage"`
	ToStage   OpportunityStage `gorm:"not null" json:"to_stage"`
	// Probability 是变更之后落库的赢单率：阶段与赢率一起看才知道
	// "当时是按几成把握算的这笔预测"。
	Probability int `gorm:"not null;default:0" json:"probability"`

	// 操作人记 id 与姓名快照：人可能改昵称、可能停用，历史里要留当时的说法。
	ActorID   uint   `gorm:"not null;default:0" json:"actor_id"`
	ActorName string `gorm:"size:64" json:"actor_name"`

	// Remark 是变更备注，可留空（改阶段通常不解释，解释也没人填）。
	Remark string `gorm:"size:255" json:"remark"`

	CreatedAt time.Time `gorm:"index;not null" json:"created_at"`
}

func (OpportunityStageLog) TableName() string { return "crm_opportunity_stage_log" }
