package model

import (
	"fmt"
	"time"
)

// SettlementSource 是应收 / 应付的来源单据类型。
//
// 应收可以挂销售订单或出货批次（F1 的"由订单或出货生成"），
// 应付只挂采购订单。三者在同一个枚举里，各自的方向由 service 限制——
// 拆成两个枚举会让"某天应收要能从合同生成"多一步翻译。
type SettlementSource int8

const (
	// SettlementSourceSalesOrder 销售订单：按订单总额开票，与出货无关。
	SettlementSourceSalesOrder SettlementSource = 10
	// SettlementSourceShipment 出货批次：按一票出货开票，分票出货时每票一笔。
	SettlementSourceShipment SettlementSource = 20
	// SettlementSourcePurchaseOrder 采购订单：应付的唯一来源。
	SettlementSourcePurchaseOrder SettlementSource = 30
)

func (s SettlementSource) IsValid() bool {
	switch s {
	case SettlementSourceSalesOrder, SettlementSourceShipment, SettlementSourcePurchaseOrder:
		return true
	}
	return false
}

func (s SettlementSource) Label() string {
	switch s {
	case SettlementSourceSalesOrder:
		return "销售订单"
	case SettlementSourceShipment:
		return "出货批次"
	case SettlementSourcePurchaseOrder:
		return "采购订单"
	default:
		return fmt.Sprintf("未知来源(%d)", int8(s))
	}
}

// SettlementStatus 是应收单 / 应付单的核销进度。
//
// 与 DocumentStatus 分开：应收单没有"备货中""已出货"，
// 而它的"作废"也不是取消——钱一旦开过票，作废要留痕，不能像草稿一样删。
type SettlementStatus int8

const (
	// SettlementStatusOpen 未收 / 未付。
	SettlementStatusOpen SettlementStatus = 10
	// SettlementStatusPartial 部分收 / 部分付：已有核销，但没结清。
	SettlementStatusPartial SettlementStatus = 20
	// SettlementStatusSettled 已结清：核销总额等于单据金额。
	SettlementStatusSettled SettlementStatus = 30
	// SettlementStatusVoid 作废：开错了或业务取消。保留行，不物理删除。
	SettlementStatusVoid SettlementStatus = 90
)

func (s SettlementStatus) IsValid() bool {
	switch s {
	case SettlementStatusOpen, SettlementStatusPartial, SettlementStatusSettled, SettlementStatusVoid:
		return true
	}
	return false
}

func (s SettlementStatus) Label() string {
	switch s {
	case SettlementStatusOpen:
		return "未结清"
	case SettlementStatusPartial:
		return "部分结清"
	case SettlementStatusSettled:
		return "已结清"
	case SettlementStatusVoid:
		return "已作废"
	default:
		return fmt.Sprintf("未知状态(%d)", int8(s))
	}
}

// Settlement 是应收与应付共有的字段。
//
// 两张表共用这组列，因为它们的语义完全一致：挂一个对方主体、一笔金额
// （原币 + 本币 + 锁定汇率）、一个到期日、一个核销进度。差别只有
// "对方是客户还是供应商"——那件事各自的表里写清即可，不必复制十列。
//
// **一期一张单。** 蓝图 F1 说"支持一单多期"，这里落成"同一笔生意生成多张单、
// 各自带到期日"，而不是"一张单挂多条分期行"：核销（T-21）按单进行，
// 若分期在单内，核销就得再指定"核销第几期"，而 F2 的措辞从头到尾只有
// "一张应收可被多笔收款核销"——它对分期一无所知。拆成多张单，T-21 一行不用改。
type Settlement struct {
	// DocNo 是本单号（RC- / PY-）。它属于单据自己，不取自来源单据：
	// 一张订单开出两期应收，两期各有各的号，回款时客户报的是我们的号。
	DocNo string `gorm:"size:32;uniqueIndex;not null" json:"doc_no"`

	// PartnerID 是对方主体：应收指向客户，应付指向供应商，由 service 校验角色位。
	PartnerID uint `gorm:"index;not null" json:"partner_id"`

	SourceType SettlementSource `gorm:"not null;index" json:"source_type"`
	SourceID   uint             `gorm:"not null;index" json:"source_id"`
	// SourceDocNo 是来源单号，摊平存一份：列表页要显示"来自哪张单"，
	// 每行都去 join 一次订单表不划算，而单号本身不会变。
	SourceDocNo string `gorm:"size:32" json:"source_doc_no"`

	// 金额四件套。汇率**开单时锁定**（F3 的硬规则）：之后汇率表怎么变都不影响
	// 这张单——反过来做（每次查询取最新汇率）会让同一张单在两天里显示两个本币金额，
	// 对账时无法解释。
	CurrencyCode string  `gorm:"size:8;not null" json:"currency_code"`
	ExchangeRate float64 `gorm:"type:decimal(18,8);not null;default:1" json:"exchange_rate"`
	Amount       float64 `gorm:"type:decimal(16,4);not null;default:0" json:"amount"`
	AmountBase   float64 `gorm:"type:decimal(16,4);not null;default:0" json:"amount_base"`

	// SettledAmount 是已核销金额，**派生值**：唯一维护方是 T-21 的核销表。
	// 与库存快照同一条规矩——派生值可以存，但只能由流水重算，不接受直接编辑。
	// 之所以不每次现算：列表页每行都要 join 一次核销表，而账龄（T-22）
	// 要按这个数分组，扫描全表时更明显。
	SettledAmount     float64 `gorm:"type:decimal(16,4);not null;default:0" json:"settled_amount"`
	SettledAmountBase float64 `gorm:"type:decimal(16,4);not null;default:0" json:"settled_amount_base"`

	// DueDate 是到期日。它是账龄分桶（30/60/90+）的唯一依据，
	// 所以不允许为空——没有到期日的应收，账龄报表里哪一桶都放不进去。
	DueDate time.Time        `gorm:"type:date;not null;index" json:"due_date"`
	Status  SettlementStatus `gorm:"not null;index" json:"status"`

	Remark string `gorm:"size:255" json:"remark"`
}

// Outstanding 是未核销的原币金额。账龄与毛利都按它算，
// 而不是按单据金额：已经收回来的钱不再是敞口。
func (s Settlement) Outstanding() float64 {
	return s.Amount - s.SettledAmount
}

// Receivable 是应收单：应向客户收取的一笔款项。
//
// 与订单、出货的关系是"多对一"：一张订单可以开出多张应收（分期），
// 但每张应收只属于一张来源单据。来源总额与已开票金额的关系由 service 守，
// 不在数据库上加约束——那需要跨表 SUM，做不成 CHECK。
type Receivable struct {
	Base
	Settlement

	Partner *Partner `gorm:"foreignKey:PartnerID" json:"partner,omitempty"`
}

func (Receivable) TableName() string { return "biz_receivable" }

// Payable 是应付单：应向供应商支付的一笔款项。
//
// 与应收镜像，但方向相反、来源只有采购订单。分两张表而不是加一列 direction：
// 应收与应付的行权、账龄口径、报表分组都不一样，混在一张表里每次查询
// 都要先带上 `direction = ?`，漏一次就是把应收算进应付。
type Payable struct {
	Base
	Settlement

	Partner *Partner `gorm:"foreignKey:PartnerID" json:"partner,omitempty"`
}

func (Payable) TableName() string { return "biz_payable" }
