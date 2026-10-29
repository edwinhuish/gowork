package repository

import (
	"gorm.io/gorm"
)

// OwnerScope 按主体归属限制单据，partnerColumn 是单据表上的客户外键列名。
//
// 归属只存在主体上（`md_partner.owner_id`），单据通过 `partner_id` 继承：
// 客户换了负责人，历史与未来的单据一起跟着走，不会出现"订单归张三、客户归李四"
// 这种必须靠人解释的状态。
//
// `owner_id = 0`（未分配）对所有人可见：刚上线的存量数据与"先录进来还没分人"
// 的单据都在这一堆里，藏起来只会让人以为数据丢了。
//
// 用 IN 子查询而不是 JOIN 主体表：六类单据的筛选条件大量使用 `status` / `doc_no`
// 这类裸列名，而 md_partner 也有同名列，一 JOIN 就变成 ambiguous，
// 三个方言还各有各的报法。子查询走主键索引，代价可以忽略。
func OwnerScope(partnerColumn string, ownerID uint) Scope {
	return func(db *gorm.DB) *gorm.DB {
		return db.Where(
			partnerColumn+" IN (SELECT id FROM md_partner WHERE owner_id = ? OR owner_id = 0)",
			ownerID,
		)
	}
}
