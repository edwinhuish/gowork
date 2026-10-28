package model

import "time"

// DocSeq 是单号流水号段：每个「单据类型 + 期段」一条，Seq 是该号段已发出的最大序号。
//
// 归属 sys_ 而不是 biz_：它不属于任何一笔业务，而是发号机制本身的状态。
// 与业务单据的区别也体现在建模上 —— 它**不嵌 Base**：
// 号段被软删除意味着号码可以重发，而单号是永久标识，重发会让两张不同的单撞号。
type DocSeq struct {
	ID uint `gorm:"primaryKey" json:"id"`
	// DocType + Period 是联合唯一键：并发插入时靠数据库拦重复，
	// 而不是靠应用层"先查再插"——那中间有一个必然被踩中的窗口。
	DocType DocType `gorm:"size:8;not null;uniqueIndex:uni_doc_seq_period,priority:1" json:"doc_type"`
	// Period 有两种形态：按月发号的是 YYYYMM（6 位），案号的是 `deal:{partner_id}:{yy}`。
	// 列宽按长的那个留，且**存 4 位年**：2100 年才不至于和 2026 年落到同一行。
	Period    string    `gorm:"size:32;not null;uniqueIndex:uni_doc_seq_period,priority:2" json:"period"`
	Seq       uint      `gorm:"not null" json:"seq"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (DocSeq) TableName() string { return "sys_doc_seq" }
