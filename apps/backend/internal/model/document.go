package model

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// DocumentStatus 是业务单据的流转阶段。
//
// 与 EnableStatus 是两套语义，不要互相顶替：EnableStatus 回答"这个账号或这门币种
// 还能不能被引用"（主数据不再有它，那边的"还能不能用"由软删除回答），
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
	// DocTypeShipment 送货单（一票货）。它同时是这一票的**套号**：
	// 商业发票与装箱单共用这个号，只在文件名与打印件上加 -CI / -PL 区分。
	// 旧前缀 `SH-` 已由 migrate 重编为 `DN-`，并留下新旧对照表。
	DocTypeShipment   DocType = "DN"
	DocTypeReceivable DocType = "RC" // 应收单
	DocTypePayable    DocType = "PY" // 应付单
	DocTypePayment    DocType = "PM" // 收付款单
	DocTypeCustoms    DocType = "CD" // 报关单
	// 库存流水号。流水不是单据（没有状态），但手工录入的期初与盘盈亏
	// 没有来源单据可引用，需要一个能在电话里报出来的标识。
	DocTypeStockMovement DocType = "MV"
	// 出入库单号。按单据类型分前缀：入库单与出库单分开，
	// 单号本身就是"这批货往哪个方向走"，不必再去看类型列。
	DocTypeStockIn    DocType = "IN"
	DocTypeStockOut   DocType = "OUT"
	DocTypeStockCount DocType = "CT"
	// DocTypePurchaseReceipt 是采购收货单（T-51a）。单独一段号：
	// 收货单不是采购单的一部分，它有自己的发生日与自己的对账口径。
	DocTypePurchaseReceipt DocType = "RCV"
	// DocTypeTransfer 是调拨单（T-56）：仓间转移，总量不变，要看得见两个仓。
	DocTypeTransfer DocType = "TR"
	// DocTypeCreditNote 是贷项通知单（T-52b）：冲减某张应收的一部分。
	DocTypeCreditNote DocType = "CN"
	// DocTypeDebitNote 是借项通知单（T-52b3）：冲减某张应付的一部分，与贷项对称。
	DocTypeDebitNote DocType = "DBN"
	// 形式发票与销售合同共用一张表、各自一条序列：单号一眼能看出是哪种，
	// 而"同一份东西两个名字"的问题只存在于类型命名，不存在于号段。
	DocTypeProformaInvoice DocType = "PI"
	DocTypeSalesContract   DocType = "SC"
)

// IsValid 挡住手误拼出来的类型。
//
// 不校验的话，一个拼错的类型会安静地在号段表里开一条新序列，
// 表现是"单号从 0001 重新开始"，而没有任何报错。
func (t DocType) IsValid() bool {
	switch t {
	case DocTypeQuotation, DocTypeSalesOrder, DocTypePurchaseOrder, DocTypeShipment,
		DocTypeReceivable, DocTypePayable, DocTypePayment, DocTypeCustoms,
		DocTypeStockMovement, DocTypeStockIn, DocTypeStockOut, DocTypeStockCount,
		DocTypePurchaseReceipt,
		DocTypeTransfer,
		DocTypeCreditNote,
		DocTypeDebitNote,
		DocTypeProformaInvoice, DocTypeSalesContract:
		return true
	}
	return false
}

// DocTypeDeal 是案号号段的内部类型，不会出现在任何单号里。
//
// 复用 sys_doc_seq 而不是另建一张表：号段的语义就是"某个键下已发到几号"，
// 案号与月号段的区别只是键从年月换成了 `{客户}:{年}`。
const DocTypeDeal DocType = "DEAL"

// DocNoPeriod 返回单号里的年月段（YYYYMM）。
func DocNoPeriod(t time.Time) string {
	return t.Format("200601")
}

// docNoStyle 返回该类型单号的序号位数与年段宽度。
//
// 采购与送货单是 `PO-2610-001` 这种三段式：月内 999 张够用，单号也短，
// 写在箱唛与手写单上不容易抄错。其余类型仍是 4 位序号、6 位年段。
func docNoStyle(t DocType) (seqWidth, yearWidth int) {
	switch t {
	case DocTypePurchaseOrder, DocTypeShipment:
		return 3, 2
	}
	return 4, 6
}

// MaxSeq 返回该类型单号序号的上限：三位 999、四位 9999。
//
// 用尽要报错而不能进位：多一位会让所有按列宽核对单号的表格错位，
// 也让「类型-年月-序号」的分段变得不可切分。
func MaxSeq(t DocType) uint {
	seqWidth, _ := docNoStyle(t)
	switch seqWidth {
	case 3:
		return 999
	default:
		return 9999
	}
}

// FormatDocNo 拼出业务单号：`PO-2610-001` 或 `IN-202610-0001`（按类型）。
//
// period 一律传 4 位年的 YYYYMM：跨世纪才不会把 2100 与 2026 挤成同一行，
// 只在展示时截掉前两位。报价单与销售订单不走这里（见 FormatDealDocNo）。
func FormatDocNo(t DocType, period string, seq uint) string {
	seqWidth, yearWidth := docNoStyle(t)
	p := period
	if yearWidth == 2 && len(period) == 6 {
		p = period[2:]
	}
	return fmt.Sprintf("%s-%s-%0*d", t, p, seqWidth, seq)
}

// DealBaseNo 拼案号：{客户编码}{YY}{NN}，如 DZ2603，读作「DZ 客户 2026 年第 3 笔生意」。
//
// 客户编码不得以数字结尾（术语表硬性约束）：案号靠字面量分段，
// 编码尾部是数字就分不清哪几位是编码、哪几位是年份。
func DealBaseNo(partnerCode string, year int, seq uint) string {
	return fmt.Sprintf("%s%02d%02d", partnerCode, year%100, seq)
}

// FormatDealDocNo 拼案号内的单据号：{案号}-{类型}{NN}，如 DZ2603-QT01 / DZ2603-SO01。
//
// 序号与案号里的 NN 相同：同一笔生意的报价与订单在单号上就并排放着，
// 客户报出 DZ2603 时，不用解释是哪张单也知道在说哪一票。
func FormatDealDocNo(baseNo string, t DocType, seq uint) string {
	return fmt.Sprintf("%s-%s%02d", baseNo, t, seq)
}

// ParseDealDocNo 把 DZ2603-QT01 拆回 (DZ2603, 1)。
//
// 从右边找最后一次 "-{类型}"：客户编码允许含 "-"，从左边切会把 DZ-CO 切成 DZ。
func ParseDealDocNo(docNo string, t DocType) (baseNo string, seq uint, ok bool) {
	suffix := "-" + string(t)
	idx := strings.LastIndex(docNo, suffix)
	if idx <= 0 {
		return "", 0, false
	}
	digits := docNo[idx+len(suffix):]
	if len(digits) != 2 {
		return "", 0, false
	}
	n, err := strconv.Atoi(digits)
	if err != nil || n <= 0 {
		return "", 0, false
	}
	return docNo[:idx], uint(n), true
}
