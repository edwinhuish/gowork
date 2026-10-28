package model

import (
	"fmt"
	"time"
)

// DocumentStatus 是业务单据的流转阶段。
//
// 与 EnableStatus 是两套语义，不要互相顶替：主数据的 status 回答"还能不能被引用"，
// 单据的 status 回答"这笔生意走到哪一步了"。两者都是小整数，赋错了编译器不报，
// 只会在筛选条件上出错（见 docs/glossary.md「状态」一节）。
//
// 取值故意留空档（10 / 20 / 30 …）。将来要在"已确认"和"备货中"之间插一个
// "已收订金"，只需给一个新常量，不用把已有记录的 30 改成 40 —— 连续编号看着整齐，
// 但它的整齐是拿历史数据换的。
type DocumentStatus int8

const (
	// DocumentStatusDraft 草稿：可改可删，不占用库存也不计应收。
	DocumentStatusDraft DocumentStatus = 10
	// DocumentStatusConfirmed 已确认：对客户/供应商的承诺成立，可以开始备货。
	DocumentStatusConfirmed DocumentStatus = 20
	// DocumentStatusPreparing 备货中：已下采购、在验货或在等船期。
	DocumentStatusPreparing DocumentStatus = 30
	// DocumentStatusShipped 已出货：货权已转移，之后只能走完成或退货，不能取消。
	DocumentStatusShipped DocumentStatus = 40
	// DocumentStatusCompleted 已完成：款货两清，单据归档。
	DocumentStatusCompleted DocumentStatus = 50
	// DocumentStatusCancelled 已取消：终态。保留记录供追溯，不再参与统计。
	DocumentStatusCancelled DocumentStatus = 90
)

// IsValid 拒绝未定义的取值。
func (s DocumentStatus) IsValid() bool {
	switch s {
	case DocumentStatusDraft, DocumentStatusConfirmed, DocumentStatusPreparing,
		DocumentStatusShipped, DocumentStatusCompleted, DocumentStatusCancelled:
		return true
	}
	return false
}

// IsFinal 报告是否为终态：已完成与已取消都不再流转。
func (s DocumentStatus) IsFinal() bool {
	return s == DocumentStatusCompleted || s == DocumentStatusCancelled
}

// CanTransitTo 是状态机的唯一实现。
//
// 允许任意跳转等于没有状态：把"已出货"的订单取消和把草稿取消是两件事，
// 前者意味着货已经在海上，必须走退货或红字单据，而不是把状态一改了事。
//
// 规则只允许前进与取消，不允许回退：回退会让"这批货到底发没发"变成要靠翻日志才能回答。
func (s DocumentStatus) CanTransitTo(next DocumentStatus) bool {
	if !next.IsValid() {
		return false
	}
	// 取消只从"未出货"的阶段发生。已出货的单据要冲出，走退货单或红字，不改状态。
	if next == DocumentStatusCancelled {
		return s == DocumentStatusDraft || s == DocumentStatusConfirmed || s == DocumentStatusPreparing
	}

	switch s {
	case DocumentStatusDraft:
		return next == DocumentStatusConfirmed
	case DocumentStatusConfirmed:
		return next == DocumentStatusPreparing
	case DocumentStatusPreparing:
		return next == DocumentStatusShipped
	case DocumentStatusShipped:
		return next == DocumentStatusCompleted
	default:
		// 终态，以及"自己到自己"这种空操作，一律拒绝。
		return false
	}
}

// String 让日志与错误文案里看到阶段名而不是裸数字。
func (s DocumentStatus) String() string {
	switch s {
	case DocumentStatusDraft:
		return "draft"
	case DocumentStatusConfirmed:
		return "confirmed"
	case DocumentStatusPreparing:
		return "preparing"
	case DocumentStatusShipped:
		return "shipped"
	case DocumentStatusCompleted:
		return "completed"
	case DocumentStatusCancelled:
		return "cancelled"
	default:
		return fmt.Sprintf("status(%d)", int8(s))
	}
}

// Label 是给人看的中文名，用于错误提示与日志。
//
// 前端 constants/catalog.ts 里有同一组中文选项，两处必须同时改：
// 后端改枚举而前端没跟，界面会把新阶段显示成"未知状态"。
func (s DocumentStatus) Label() string {
	switch s {
	case DocumentStatusDraft:
		return "草稿"
	case DocumentStatusConfirmed:
		return "已确认"
	case DocumentStatusPreparing:
		return "备货中"
	case DocumentStatusShipped:
		return "已出货"
	case DocumentStatusCompleted:
		return "已完成"
	case DocumentStatusCancelled:
		return "已取消"
	default:
		return fmt.Sprintf("未知状态(%d)", int8(s))
	}
}

// DocType 是单据类型，同时决定单号前缀。
//
// 用两位字母而不是数字：单号会出现在邮件、报关资料和聊天记录里，
// "SO-202610-0001" 一眼能看出是销售订单，"01-202610-0001" 只能去查表。
type DocType string

const (
	DocTypeQuotation     DocType = "QT" // 报价单
	DocTypeSalesOrder    DocType = "SO" // 销售订单
	DocTypePurchaseOrder DocType = "PO" // 采购订单
	DocTypeShipment      DocType = "SH" // 出货批次
	DocTypeReceivable    DocType = "RC" // 应收单
	DocTypePayable       DocType = "PY" // 应付单
	DocTypePayment       DocType = "PM" // 收付款单
	DocTypeCustoms       DocType = "CD" // 报关单
)

// IsValid 挡住手误拼出来的类型。
//
// 不校验的话，一个拼错的类型会安静地在号段表里开一条新序列，
// 表现是"单号从 0001 重新开始"，而没有任何报错。
func (t DocType) IsValid() bool {
	switch t {
	case DocTypeQuotation, DocTypeSalesOrder, DocTypePurchaseOrder, DocTypeShipment,
		DocTypeReceivable, DocTypePayable, DocTypePayment, DocTypeCustoms:
		return true
	}
	return false
}

// DocNoPeriod 返回单号里的年月段（YYYYMM）。
func DocNoPeriod(t time.Time) string {
	return t.Format("200601")
}

// FormatDocNo 拼出业务单号：类型(2)-年月(6)-流水(4)，如 SO-202610-0001。
func FormatDocNo(t DocType, period string, seq uint) string {
	return fmt.Sprintf("%s-%s-%04d", t, period, seq)
}
