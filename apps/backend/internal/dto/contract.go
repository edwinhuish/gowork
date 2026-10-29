package dto

import (
	"time"

	"github.com/edwinhuish/gowork/internal/pkg/money"
)

// ContractQuery 是合同列表的查询条件。
//
// 不提供"来源单号"筛选：那份单号的持有者是报价单或订单，
// 按它找合同应该从来源单据的详情点过来，而不是在合同列表里再手输一遍。
type ContractQuery struct {
	PageQuery
	Type      *int8 `form:"type"`
	PartnerID *uint `form:"partner_id"`
	Status    *int8 `form:"status"`
}

// ContractTermInput 是一条付款分期：30% 订金 / 70% 见提单副本。
//
// 没有本币金额与序号：本币 = 原币 × 合同汇率，序号 = 提交顺序。
// 让前端同时提交两个币种，等于允许它们互相对不上；
// 让前端决定序号，则"先订金后尾款"要靠界面保证，换个客户端就乱了。
type ContractTermInput struct {
	Name string `json:"name" binding:"required,max=64"`
	// Amount 按字符串收发（money.Decimal）；负数在服务层判。
	Amount money.Decimal `json:"amount"`
	// DueDate 约定付款日，可留空（"见提单副本后 7 天"这种没有具体日期的场景）。
	DueDate string `json:"due_date"`
	// Trigger 付款条件，与 DueDate 可同时存在。
	Trigger string `json:"trigger" binding:"max=128"`
	// ReceivableID 是这一期对应的应收单（T-61），可留空表示还没开票。
	ReceivableID *uint `json:"receivable_id"`
}

// ContractGenerateInput 是从来源单据生成合同的入参。
//
// 没有金额、币种、汇率、客户：全部照抄来源单据（见 model.Contract 的注释）。
// 合同是"把已经报出去的价写成一份承诺"，任一金额项可以手填，
// 这份合同与来源单据就对不上，而出货与开票按哪个算没人说得清。
type ContractGenerateInput struct {
	// Type 10 形式发票 / 20 销售合同。
	Type int8 `json:"type" binding:"required"`
	// SourceType 10 报价单 / 20 销售订单。
	SourceType int8 `json:"source_type" binding:"required"`
	SourceID   uint `json:"source_id" binding:"required"`
	// SignDate 签订（出具）日，留空取今天。单号年月按它取。
	SignDate string `json:"sign_date"`
	// DeliveryDate 约定交期，留空取来源单据的交期。
	DeliveryDate string `json:"delivery_date"`
	Remark       string `json:"remark" binding:"max=255"`
	// Terms 至少一条：没有分期的合同没法回答"客户什么时候付这笔钱"。
	Terms []ContractTermInput `json:"terms" binding:"required,min=1,dive"`
	// ClauseIDs 是从条款库挑的模板 id，按提交顺序排列（顺序就是合同上的条款顺序）。
	// 只传 id 不传正文：正文由服务端从模板抄成**快照**，
	// 前端传正文的话，改模板不会影响已生成的合同这条就没人保证了。
	ClauseIDs []uint `json:"clause_ids"`
}

// ContractUpdateInput 是草稿上可改的字段。
//
// 来源、客户、金额不在其中：合同金额来自来源单据，要改金额就改那张单据、
// 作废这张合同重新生成。允许在这里改金额，等于让合同与订单各说一套。
type ContractUpdateInput struct {
	// SignDate 留空表示保持原签订日——它已经印在单号与已发出去的 PDF 上了，
	// 不因为"这次没填"就变成今天。
	SignDate string `json:"sign_date"`
	// DeliveryDate 留空表示清空交期：交期谈掉是常事，而它本来就可空。
	DeliveryDate string              `json:"delivery_date"`
	Remark       string              `json:"remark" binding:"max=255"`
	Terms        []ContractTermInput `json:"terms" binding:"required,min=1,dive"`
	// ClauseIDs 整体替换合同上的条款（同样从模板抄快照）。留空表示清空条款——
	// 与 terms 一致：编辑保存就是"这一份现在长这样"，不做增量。
	ClauseIDs []uint `json:"clause_ids"`
	// UpdatedAt 是编辑页打开时读到的版本；带上它做条件更新，期间被别人改过则 409。
	UpdatedAt *time.Time `json:"updated_at"`
}

// ContractStatusInput 只接受合同真正会走到的两个状态：已确认（20）与已取消（90）。
// 备货中 / 已出货属于订单与出货批次，合同不该出现这些阶段。
type ContractStatusInput struct {
	Status int8 `json:"status" binding:"required"`
}
