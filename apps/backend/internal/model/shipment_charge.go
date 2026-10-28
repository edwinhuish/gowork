package model

import "fmt"

// ShipmentChargeType 是一票货的某笔费用属于哪一类。
//
// 取值故意留空档（10 / 20 / 30 …）：将来要在报关费与拖车费之间插一类"查验费"，
// 只加一个新值，不用把已有的 30 改成 40；连续编号看着整齐，但它的整齐是拿历史数据换的。
//
// 中文标签要与前端 `constants/catalog.ts` 的 SHIPMENT_CHARGE_OPTIONS 一致。
type ShipmentChargeType int8

const (
	// ShipmentChargeFreight 海运费 / 空运费：整票的主运费。
	ShipmentChargeFreight ShipmentChargeType = 10
	// ShipmentChargeCustoms 报关费：报关行的服务费。不含关税与进口增值税——
	// 那两项由进口方在目的国缴纳，我方出口一般不承担。
	ShipmentChargeCustoms ShipmentChargeType = 20
	// ShipmentChargeTrucking 拖车 / 内陆运输：工厂到港、港到仓。
	ShipmentChargeTrucking ShipmentChargeType = 30
	// ShipmentChargeInsurance 保险费。CIF 报价里含的就是这一项与海运费。
	ShipmentChargeInsurance ShipmentChargeType = 40
	// ShipmentChargeOther 其它：单证、仓储、查验、改单等零星费用。
	ShipmentChargeOther ShipmentChargeType = 90
)

func (t ShipmentChargeType) IsValid() bool {
	switch t {
	case ShipmentChargeFreight, ShipmentChargeCustoms, ShipmentChargeTrucking,
		ShipmentChargeInsurance, ShipmentChargeOther:
		return true
	}
	return false
}

func (t ShipmentChargeType) Label() string {
	switch t {
	case ShipmentChargeFreight:
		return "海运费"
	case ShipmentChargeCustoms:
		return "报关费"
	case ShipmentChargeTrucking:
		return "拖车费"
	case ShipmentChargeInsurance:
		return "保险费"
	case ShipmentChargeOther:
		return "其它"
	default:
		return fmt.Sprintf("未知类型(%d)", int8(t))
	}
}

// ShipmentCharge 是归集到一票货的一笔费用。
//
// 它回答"这票货除了货款还花了多少"，是单票毛利（T-22）的减项。
// 挂在一票货而不是摊到每张订单行上：一票可以装三张订单的货，
// 而海运费是按整票收的，摊到行上要先定分摊规则，规则一改历史数字全变。
// 真要按行看成本时，用"本票费用 ÷ 本票货值"分摊，那是报表的事，不是落库的口径。
//
// **只登记我方承担的费用。** FOB 下客户付的海运费不录：录进来会被算成我方成本，
// 毛利凭空低一截，而钱一分没花。客户承担又由我方垫付的情况，先记在这里，
// T-20 做应收时再冲减——现在没有应收载体，记了也冲不掉。
type ShipmentCharge struct {
	Base
	ShipmentID uint               `gorm:"index;not null" json:"shipment_id"`
	Type       ShipmentChargeType `gorm:"not null;index" json:"type"`
	// PartnerID 是收这笔钱的主体（货代或供应商），可为空：先记账、后补付款对象的
	// 场景很常见（货代账单只发来一个总数，明细还没拆）。
	// 它不表示"谁承担"——这张表里每一行都是我方的成本。
	PartnerID *uint `gorm:"index" json:"partner_id"`

	// 金额按「原币 + 币种 + 本币 + 汇率」四件套存，与产品货源同一套口径。
	// 只存原币的话，把外币费用和人民币费用加在一起要先手工换算，
	// 而汇率每天都在变，昨天的合计今天就对不上。
	Amount       float64 `gorm:"type:decimal(14,2);not null;default:0" json:"amount"`
	CurrencyCode string  `gorm:"size:8;not null" json:"currency_code"`
	AmountBase   float64 `gorm:"type:decimal(14,2);not null;default:0" json:"amount_base"`
	ExchangeRate float64 `gorm:"type:decimal(18,8);not null;default:1" json:"exchange_rate"`

	Remark string `gorm:"size:255" json:"remark"`

	Partner *Partner `gorm:"foreignKey:PartnerID" json:"partner,omitempty"`
}

func (ShipmentCharge) TableName() string { return "biz_shipment_charge" }
