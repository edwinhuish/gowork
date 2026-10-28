package model

import (
	"fmt"
	"time"
)

// CustomsStatus 是报关单在海关那边的状态。
//
// 不复用通用的 DocumentStatus：报关单没有"备货中""已出货"，
// 而它的"退单"（海关退回要求修改）也不等于单据作废——改完还要重新申报。
// 两者混用一个枚举，前端就得为同一种颜色写两套判断。
type CustomsStatus int8

const (
	// CustomsStatusDraft 待申报：要素已备齐，还没提交给海关。
	CustomsStatusDraft CustomsStatus = 10
	// CustomsStatusDeclared 已申报：已向海关提交，等放行。
	CustomsStatusDeclared CustomsStatus = 20
	// CustomsStatusReleased 已放行：海关放行，可以装船。
	CustomsStatusReleased CustomsStatus = 30
	// CustomsStatusReturned 已退单：海关退回要求修改。改完重新申报，不是作废。
	CustomsStatusReturned CustomsStatus = 90
)

func (s CustomsStatus) IsValid() bool {
	switch s {
	case CustomsStatusDraft, CustomsStatusDeclared, CustomsStatusReleased, CustomsStatusReturned:
		return true
	}
	return false
}

func (s CustomsStatus) Label() string {
	switch s {
	case CustomsStatusDraft:
		return "待申报"
	case CustomsStatusDeclared:
		return "已申报"
	case CustomsStatusReleased:
		return "已放行"
	case CustomsStatusReturned:
		return "已退单"
	default:
		return fmt.Sprintf("未知状态(%d)", int8(s))
	}
}

// CustomsDeclaration 是一票货的报关记录。
//
// **一票可以有多张**：同一票货里不同 HS 编码要分票申报，算两张报关单；
// 被退单后改单重报也是新的一条，旧的留着——"海关退回过什么"这件事
// 只有在旧单还在的时候才回答得了。所以它是一对多，不是挂在出货批次上的几个字段。
//
// 报关单号、提单号这类**外部号**只记录、不参与发号与唯一性（蓝图 §五.4）：
// 号码由海关给，系统再造一个号只会在对单时多一层翻译。
type CustomsDeclaration struct {
	Base
	ShipmentID uint          `gorm:"index;not null" json:"shipment_id"`
	Status     CustomsStatus `gorm:"not null;index" json:"status"`
	// DeclNo 报关单号，海关给的 18 位号。允许为空：要素先备齐、号后补是常态，
	// 而"待申报"这个状态存在的意义就是把这段空档记下来。
	DeclNo string `gorm:"size:32;index" json:"decl_no"`
	// DeclDate 申报日期，留空表示还没申报。
	DeclDate *time.Time `gorm:"type:date" json:"decl_date"`
	// SupervisionMode 监管方式代码，如 0110 一般贸易 / 0615 进料对口 / 1039 市场采购。
	//
	// 存代码而不是枚举：海关的监管方式有几十种且会调整，做成枚举意味着
	// 每加一种都要发一次版本，而它只是个四位数字。
	SupervisionMode string `gorm:"size:8" json:"supervision_mode"`
	Remark          string `gorm:"size:255" json:"remark"`
}

func (CustomsDeclaration) TableName() string { return "biz_customs_declaration" }
