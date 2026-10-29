package model

import (
	"time"

	"github.com/edwinhuish/gowork/internal/pkg/money"
)

// 币种小数位的默认值与边界。
//
// 边界与 pkg/money 保持一致：那边的 Round 会把超出范围的值夹到合法区间，
// 这里存进去的值若超出同一区间，就会出现"存的是 8 位、算的是 6 位"的静默偏差。
const (
	DefaultDecimalPlaces = 2
)

// Currency 币种主数据。
//
// 单据上不存币种 id 而存三位代码（见 docs/glossary.md 命名规则），
// 所以这里 code 是业务身份：**永不可改**。改了它，所有引用它的历史汇率
// 与单据都会指向一个不存在的币种，而数据库层面看不出任何异常。
type Currency struct {
	Base
	// 遮蔽 Base 里的同名字段，把它排进「未删才唯一」的复合索引，见 Product 的说明。
	DeletedAt DeletedAt `gorm:"softDelete:milli;not null;default:0;uniqueIndex:uni_currency_code,priority:2" json:"deleted_at,omitempty"`
	Code      string    `gorm:"size:8;uniqueIndex:uni_currency_code,priority:1;not null" json:"code"`
	Name string `gorm:"size:64;not null" json:"name"`
	// Symbol 仅用于界面展示（$ / € / ¥），不参与计算。
	Symbol string `gorm:"size:8" json:"symbol"`
	// DecimalPlaces 是该币种金额的展示与舍入精度。人民币 2 位，日元 0 位。
	DecimalPlaces int `gorm:"not null;default:2" json:"decimal_places"`
	// IsBase 标记本币。全库有且只有一条为 true，由 service 在事务里保证。
	//
	// 不用"配置项"表达本币：本币出现在每张单据的 amount_base 列里，
	// 换了本币之后老单据的本币金额仍是旧本币口径，必须能查到它当时是哪个币种。
	IsBase bool `gorm:"not null;default:false;index" json:"is_base"`
	// Status 同 md_partner：刻意不写 default tag，避免"显式停用"被静默存成启用。
	Status EnableStatus `gorm:"not null;index" json:"status"`
	Remark string       `gorm:"size:255" json:"remark"`
}

func (Currency) TableName() string { return "md_currency" }

// ExchangeRate 汇率：某币种在某生效日相对本币的折算率。
//
// 语义固定为「1 单位 CurrencyCode = Rate 单位 BaseCode」，不提供反向存储。
// 两个方向都存会让"以哪一行为准"变成需要约定的问题，而对账时没人记得住约定。
//
// 只追加、不覆盖历史：同一天同一币种只能有一条，改价要新增一天或先删掉错的那条。
// 覆盖历史会让三个月前那张报价单的成本重算成另一个数。
type ExchangeRate struct {
	Base
	// 遮蔽 Base 里的同名字段：汇率也是软删，"未删才唯一"要把它排进复合索引。
	DeletedAt DeletedAt `gorm:"softDelete:milli;not null;default:0;uniqueIndex:uni_exchange_rate_pair,priority:4" json:"deleted_at,omitempty"`
	// CurrencyCode 外币代码（USD / EUR）。
	CurrencyCode string `gorm:"size:8;not null;index;uniqueIndex:uni_exchange_rate_pair,priority:1" json:"currency_code"`
	// BaseCode 记录当时的本币代码。冗余存一份而不是 join 出来：
	// 本币将来若变更，老汇率的换算口径仍然是老本币。
	BaseCode string `gorm:"size:8;not null;uniqueIndex:uni_exchange_rate_pair,priority:2" json:"base_code"`
	// Rate 是折算率本身，定点表示。列宽 8 位而定点只到 6 位（全仓统一的最小单位），
	// 落库后末两位恒为 0：再多的位数在业务上没意义，每多一位 int64 的上限就少十倍。
	Rate          money.Decimal `gorm:"type:decimal(18,8);not null" json:"rate"`
	EffectiveDate time.Time     `gorm:"type:date;not null;uniqueIndex:uni_exchange_rate_pair,priority:3" json:"effective_date"`
	// Source 记来源（手工 / 银行 / 中国银行牌价），便于事后判断这条价可不可信。
	Source string `gorm:"size:32" json:"source"`
	Remark string `gorm:"size:255" json:"remark"`
}

func (ExchangeRate) TableName() string { return "md_exchange_rate" }
