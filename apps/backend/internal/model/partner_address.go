package model

// PartnerAddressType 是地址的用途。
//
// 分成四种而不是"多地址一锅端"：外贸一张单证上同时出现三种地址——
// 发票抬头（收款方）、收货人（consignee）、通知方（notify party），
// 三者常常分属不同主体，而发货地址又可能与它们都不同。
// 蓝图业务假设第 6 条说的就是这件事，所以类型不是标签，是取址的依据。
type PartnerAddressType string

const (
	// PartnerAddressInvoice 是发票抬头地址：印在 PI 与商业发票上。
	PartnerAddressInvoice PartnerAddressType = "invoice"
	// PartnerAddressDelivery 是收货地址：货送到哪。
	PartnerAddressDelivery PartnerAddressType = "delivery"
	// PartnerAddressConsignee 是提单收货人地址：提单上"Consignee"那一栏。
	PartnerAddressConsignee PartnerAddressType = "consignee"
	// PartnerAddressNotify 是通知方地址：船到港后通知谁，通常是客户的报关行。
	PartnerAddressNotify PartnerAddressType = "notify"
)

// AllPartnerAddressTypes 是全部合法用途，顺序即界面上的顺序。
var AllPartnerAddressTypes = []PartnerAddressType{
	PartnerAddressInvoice,
	PartnerAddressDelivery,
	PartnerAddressConsignee,
	PartnerAddressNotify,
}

// IsValid 拒绝未定义的取值。
func (t PartnerAddressType) IsValid() bool {
	for _, v := range AllPartnerAddressTypes {
		if t == v {
			return true
		}
	}
	return false
}

// Label 供错误提示与日志使用，避免把 invoice 这种内部值直接抛给用户。
func (t PartnerAddressType) Label() string {
	switch t {
	case PartnerAddressInvoice:
		return "发票地址"
	case PartnerAddressDelivery:
		return "收货地址"
	case PartnerAddressConsignee:
		return "提单收货人地址"
	case PartnerAddressNotify:
		return "通知方地址"
	default:
		return string(t)
	}
}

// PartnerAddress 是主体的一个地址。
//
// 独立成表而不是往 md_partner 上加 address2 / address3：地址是 1:N，
// 而且每个类型都可能不止一条（同一客户在中国的两个收货仓），
// 往主表加列迟早要加第四个、第五个，而"哪一列是提单地址"只能靠字段名记忆。
//
// 归属：共享主数据（md_ 前缀）。ERP 的单证取它，CRM 的客户详情展示它。
type PartnerAddress struct {
	Base
	// PartnerID 是所属主体。删主体时地址不级联删：主体是软删，
	// 恢复时地址要跟着回来，级联删掉就找不回来了。
	PartnerID uint `gorm:"not null;index:idx_partner_address,priority:1" json:"partner_id"`
	// Type 是用途，与 PartnerID 一起构成"同一主体同一用途"的分组。
	Type PartnerAddressType `gorm:"size:16;not null;index:idx_partner_address,priority:2" json:"type"`
	// IsDefault 是该用途下的默认地址，单证未指定时取它。
	//
	// 每个（主体，用途）最多一条为真，由仓储层在同一事务里保证：
	// 与主联系人同一套做法，不用部分唯一索引——SQLite / MySQL / PostgreSQL
	// 对"条件唯一"的支持各不相同，应用层事务是唯一三库一致的写法。
	IsDefault  bool   `gorm:"not null;default:false" json:"is_default"`
	Contact    string `gorm:"size:64" json:"contact"`
	Phone      string `gorm:"size:32" json:"phone"`
	Country    string `gorm:"size:64" json:"country"`
	Province   string `gorm:"size:64" json:"province"`
	City       string `gorm:"size:64" json:"city"`
	PostalCode string `gorm:"size:16" json:"postal_code"`
	// Address 是门牌与街道，**不含国家省市**——那三项单独存，
	// 因为单证排版按行取，拼在一起的字符串没法拆。
	Address string `gorm:"size:255;not null" json:"address"`
	Remark  string `gorm:"size:255" json:"remark"`
}

func (PartnerAddress) TableName() string { return "md_partner_address" }
