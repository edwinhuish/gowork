package model

import (
	"time"

	"github.com/edwinhuish/gowork/internal/pkg/money"
)

// SalesOrderChange 是已确认订单的一次变更。
//
// 为什么不能直接改：已确认的订单是对客户的承诺。客户手里那份 PDF、
// 工厂那边照着做的采购单、已经出掉的货，都认当时那个数与那个交期。
// 允许直接编辑，等于让"订单上写着什么"随最后一个编辑它的人变化。
//
// 所以改走变更单：订单本身更新到新值，同时留一条记录说明谁在什么时候、
// 因为什么、把哪几行从多少改成了多少。订单号不变，变的是版本历史。
//
// 不单独发号：单号由订单号派生（`{订单号}-CH{n}`），因为它脱离订单没有意义，
// 而另起一条序列会让"这张单改过几次"需要跨两张表才数得出来。
type SalesOrderChange struct {
	Base
	// 同一张订单的第几次变更，从 1 开始。
	DeletedAt DeletedAt `gorm:"softDelete:milli;not null;default:0;uniqueIndex:uni_order_change_seq,priority:3" json:"deleted_at,omitempty"`
	SalesOrderID uint   `gorm:"not null;uniqueIndex:uni_order_change_seq,priority:1" json:"sales_order_id"`
	Seq          uint   `gorm:"not null;uniqueIndex:uni_order_change_seq,priority:2" json:"seq"`
	DocNo        string `gorm:"size:40;not null" json:"doc_no"`

	// Reason 必填：没有原因的变更记录等于没有记录，
	// 事后翻账时它只能告诉你"改过"，说不出"为什么"。
	Reason string `gorm:"size:255;not null" json:"reason"`
	// Operator 是操作人用户名的快照，不是外键：账号被删或改名之后，
	// 这条历史仍要能读出"当时是谁改的"。
	Operator string `gorm:"size:64;not null" json:"operator"`

	// 变更前后的订单总额，冗余存一份：查历史时不必再回放当时所有行的值。
	BeforeTotal money.Decimal `gorm:"type:decimal(16,4);not null;default:0" json:"before_total"`
	AfterTotal  money.Decimal `gorm:"type:decimal(16,4);not null;default:0" json:"after_total"`

	// 交期挂在订单头上，因此单独记一次；两个都为空表示这次没动交期。
	BeforeDeliveryDate *time.Time `gorm:"type:date" json:"before_delivery_date"`
	AfterDeliveryDate  *time.Time `gorm:"type:date" json:"after_delivery_date"`

	Lines []SalesOrderChangeLine `gorm:"foreignKey:ChangeID" json:"lines,omitempty"`
}

func (SalesOrderChange) TableName() string { return "biz_sales_order_change" }

// SalesOrderChangeLine 是变更单里的一行，记录某一行订单明细的前后值。
//
// 只记真正变了的行：把没动的行也抄一遍，等于每次变更都复制一遍整张订单，
// 而"哪几行变了"才是这张表要回答的问题。
type SalesOrderChangeLine struct {
	Base
	ChangeID         uint `gorm:"index;not null" json:"change_id"`
	SalesOrderItemID uint `gorm:"index;not null" json:"sales_order_item_id"`

	// SKU 快照：订单行本身有快照，这里再存一份是为了让变更记录能独立读懂，
	// 不必回订单表——订单行将来被删（订单整单作废）时它还在。
	SKUCode string `gorm:"size:32" json:"sku_code"`

	BeforeQty       money.Decimal `gorm:"type:decimal(14,3);not null;default:0" json:"before_qty"`
	AfterQty        money.Decimal `gorm:"type:decimal(14,3);not null;default:0" json:"after_qty"`
	BeforeUnitPrice money.Decimal `gorm:"type:decimal(14,4);not null;default:0" json:"before_unit_price"`
	AfterUnitPrice  money.Decimal `gorm:"type:decimal(14,4);not null;default:0" json:"after_unit_price"`
}

func (SalesOrderChangeLine) TableName() string { return "biz_sales_order_change_line" }
