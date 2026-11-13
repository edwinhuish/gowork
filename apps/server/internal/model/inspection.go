package model

import "time"

// InspectionResult 是验货结论（T-51b）。
//
// 只有「验货」节点上的一个完成日期回答不了"验出了什么"。代理采购卖给客户的
// 核心承诺是「我验过货」——出了争议要能拿出抽检比例、缺陷项与照片。
// 三档结论而不是「合格 / 不合格」两档：外贸里最常见的是"有瑕疵但客户接受，
// 让价或挑拣后出货"，把它硬塞进两档，人就会随手点合格。
type InspectionResult string

const (
	// InspectionPassed 合格：按 AQL 抽检未发现致命或主要缺陷。
	InspectionPassed InspectionResult = "passed"
	// InspectionFailed 不合格：要求返工后复验。
	InspectionFailed InspectionResult = "failed"
	// InspectionPartial 有条件接受：有缺陷，经客户同意后让步接收或挑拣后出货。
	InspectionPartial InspectionResult = "partial"
)

// IsValid 拒绝未定义的取值。
func (r InspectionResult) IsValid() bool {
	switch r {
	case InspectionPassed, InspectionFailed, InspectionPartial:
		return true
	}
	return false
}

// Label 供错误提示与日志使用。
func (r InspectionResult) Label() string {
	switch r {
	case InspectionPassed:
		return "合格"
	case InspectionFailed:
		return "不合格"
	case InspectionPartial:
		return "有条件接受"
	default:
		return string(r)
	}
}

// Inspection 是一次验货记录。
//
// 与跟单节点（PurchaseMilestone 的"验货"节点）分工：节点记「哪天验的」，
// 本表记「验出了什么」。两者不合并的理由是节点表被逾期筛选按行聚合，
// 往里塞结论与长文本会让那张表的查询变重，而验货记录是低频写入、低频读取。
type Inspection struct {
	Base
	PurchaseOrderID uint `gorm:"index;not null" json:"purchase_order_id"`
	// PurchaseOrderItemID 可空：整单验货时留空，按行验货时挂到具体采购行。
	PurchaseOrderItemID *uint `gorm:"index" json:"purchase_order_item_id"`

	Result InspectionResult `gorm:"size:16;not null;index" json:"result"`
	// SampleRate 是抽检比例（百分比，0 表示未记录）。
	// 用整数百分比而不是文本：AQL 抽检是能给客户看的数，写成"抽了 20%"就没法筛。
	SampleRate int `gorm:"not null;default:0" json:"sample_rate"`
	// InspectedAt 是实际验货日，决定它落在哪个统计月份。
	InspectedAt time.Time `gorm:"type:date;not null;index" json:"inspected_at"`
	// Inspector 是验货人（用户名快照，不是外键）：账号改名后历史仍读得出当时是谁。
	Inspector string `gorm:"size:64" json:"inspector"`
	// Defects 是缺陷项，多行文本。不拆成结构化字段：缺陷的描述远比字段多，
	// 而这一段是给人（和客户）看的，不是给统计用的。
	Defects string `gorm:"size:1000" json:"defects"`
	Remark  string `gorm:"size:255" json:"remark"`

	// SKUCode 是采购行的物料快照投影，供列表直接显示（不落库）。
	SKUCode string `gorm:"-" json:"sku_code,omitempty"`
}

func (Inspection) TableName() string { return "biz_inspection" }
