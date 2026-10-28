package model

import "time"

// DocNoRename 是单号重编的新旧对照：客户手上的旧号必须还能查回来。
//
// 重编只在结构升级时发生一次（如 `SH-` → `DN-`），但客户、货代与报关行手里的
// 纸质单会用很多年。"查一下 SH-202610-0001 现在是什么号"如果只能靠翻提交历史，
// 那张单在业务上就等于丢了。
//
// 归属 sys_：它是发号机制的账，不是业务数据；也不嵌 Base——对照记录不该被软删除，
// 删掉就意味着一段历史无法解释。
type DocNoRename struct {
	ID      uint    `gorm:"primaryKey" json:"id"`
	DocType DocType `gorm:"size:8;not null;index" json:"doc_type"`
	// DocID 指向具体那张单，旧号与新号都能落到同一条记录上。
	DocID uint `gorm:"not null;index" json:"doc_id"`
	// OldNo 唯一：同一个旧号不可能被重编两次。
	OldNo     string    `gorm:"size:32;not null;uniqueIndex" json:"old_no"`
	NewNo     string    `gorm:"size:32;not null;index" json:"new_no"`
	RenamedAt time.Time `gorm:"not null" json:"renamed_at"`
}

func (DocNoRename) TableName() string { return "sys_doc_no_rename" }
