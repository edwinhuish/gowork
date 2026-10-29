package model

import (
	"time"

	"github.com/edwinhuish/gowork/internal/pkg/money"
)

// PurchaseOrder 是向供应商下的采购单：销售订单的镜像。
//
// 它与销售订单共用同一套单据骨架（`DocNo`、`DocumentStatus`、服务端汇总合计），
// 差别在两处：对方是供应商（不是客户），明细可以挂到销售订单行上——这就是「以销定采」。
//
// 为什么"挂到销售订单行"必须是一列而不是备注里写一句：
// 采购代理的日常是"这 800 只合页是为 PER001 那张单备的"。有了这一列，
// 才能回答"哪些货还没采""这单的毛利里采购成本是多少"；没有它，只能靠人记。
type PurchaseOrder struct {
	Base
	// 遮蔽 Base 里的同名字段，把它排进「未删才唯一」的复合索引，见 Product 的说明。
	DeletedAt DeletedAt `gorm:"softDelete:milli;not null;default:0;uniqueIndex:uni_purchase_order_doc_no,priority:2" json:"deleted_at,omitempty"`
	DocNo     string    `gorm:"size:32;uniqueIndex:uni_purchase_order_doc_no,priority:1;not null" json:"doc_no"`
	// SupplierID 指向承担供应商角色的主体，由 service 校验角色位。
	SupplierID uint           `gorm:"index;not null" json:"supplier_id"`
	TradeTerm  TradeTerm      `gorm:"size:8;not null" json:"trade_term"`
	Status     DocumentStatus `gorm:"not null;index" json:"status"`

	OrderDate time.Time `gorm:"type:date;not null;index" json:"order_date"`
	// DeliveryDate 是供应商承诺的到货日，可空——下单时还没谈定是常态。
	DeliveryDate *time.Time `gorm:"type:date" json:"delivery_date"`

	// 采购币种通常与销售币种不同（收美元、付人民币），所以汇率必须单独锁一份。
	CurrencyCode string        `gorm:"size:8;not null" json:"currency_code"`
	ExchangeRate money.Decimal `gorm:"type:decimal(18,8);not null;default:1" json:"exchange_rate"`

	// 合计由明细汇总，写入时算一次；入参里没有合计字段，前端传了也读不到。
	AmountTotal     money.Decimal `gorm:"type:decimal(16,4);not null;default:0" json:"amount_total"`
	AmountBaseTotal money.Decimal `gorm:"type:decimal(16,4);not null;default:0" json:"amount_base_total"`

	Remark string `gorm:"size:255" json:"remark"`

	Supplier *Partner            `gorm:"foreignKey:SupplierID" json:"supplier,omitempty"`
	Items    []PurchaseOrderItem `gorm:"foreignKey:PurchaseOrderID" json:"items,omitempty"`

	// OverdueNode 是列表投影出来的「最靠前的逾期未完成节点」文案（T-38），
	// 如「验货」；没有逾期时为空。只读、不建列：它由 service 按截止日批量算出来。
	OverdueNode string `gorm:"->;-:migration" json:"overdue_node,omitempty"`
	// Milestones 只有详情带（跟单节点）。
	Milestones []PurchaseMilestone `gorm:"foreignKey:PurchaseOrderID" json:"milestones,omitempty"`
}

func (PurchaseOrder) TableName() string { return "biz_purchase_order" }

// IsEditable 报告采购单还能不能改。只有草稿能改：已确认的采购单是给工厂的承诺，
// 改价改量要留痕（T-24 的变更单）。
func (o PurchaseOrder) IsEditable() bool { return o.Status == DocumentStatusDraft }

// PurchaseOrderItem 是采购明细行。
type PurchaseOrderItem struct {
	Base
	PurchaseOrderID uint `gorm:"index;not null" json:"purchase_order_id"`
	ProductID       uint `gorm:"index;not null" json:"product_id"`

	SKUCode string `gorm:"size:32;not null" json:"sku_code"`
	NameCN  string `gorm:"size:128" json:"name_cn"`
	Spec    string `gorm:"size:128" json:"spec"`
	Unit    string `gorm:"size:16" json:"unit"`
	// Qty 是定点数：与采购价相乘决定行金额，中途转 float64 会把末位带歪。
	Qty money.Decimal `gorm:"type:decimal(14,3);not null" json:"qty"`

	UnitPrice  money.Decimal `gorm:"type:decimal(14,4);not null;default:0" json:"unit_price"`
	Amount     money.Decimal `gorm:"type:decimal(16,4);not null;default:0" json:"amount"`
	AmountBase money.Decimal `gorm:"type:decimal(16,4);not null;default:0" json:"amount_base"`

	// SalesOrderItemID 非空表示这一行是为某张销售订单的某一行采购的（以销定采）。
	// 只存 id 不存单号快照：销售订单行不会消失，而单号可以从关联里查出来。
	SalesOrderItemID *uint `gorm:"index" json:"sales_order_item_id"`
	// SourceID 记录这行采购对应哪条货源，事后能回答"当初是按哪家的报价下的单"。
	SourceID *uint `gorm:"index" json:"source_id"`

	Remark string `gorm:"size:255" json:"remark"`

	Product *Product `gorm:"foreignKey:ProductID" json:"product,omitempty"`
}

func (PurchaseOrderItem) TableName() string { return "biz_purchase_order_item" }
