package model

import (
	"strings"
	"time"

	"github.com/edwinhuish/gowork/internal/pkg/money"
)

// Shipment 是一次出货（一票）。
//
// 为什么单独建表而不是在订单上加"出货日期"：一张订单可以分三次走（先空运两箱样品、
// 再拼柜、最后尾货），三次各有自己的日期与单据；反过来，一票也可以装三张订单的货
// ——外贸拼柜是常态。所以出货与订单是多对多，"一次出货"必须能独立存在。
type Shipment struct {
	Base
	// 遮蔽 Base 里的同名字段，把它排进「未删才唯一」的复合索引，见 Product 的说明。
	DeletedAt DeletedAt      `gorm:"softDelete:milli;not null;default:0;uniqueIndex:uni_shipment_doc_no,priority:2" json:"deleted_at,omitempty"`
	DocNo     string         `gorm:"size:32;uniqueIndex:uni_shipment_doc_no,priority:1;not null" json:"doc_no"`
	Status    DocumentStatus `gorm:"not null;index" json:"status"`
	// ShipDate 是实际（或计划）出货日，决定它落在哪个统计月份。
	ShipDate time.Time `gorm:"type:date;not null;index" json:"ship_date"`
	Remark   string    `gorm:"size:255" json:"remark"`

	// ChargeAllocBasis 是运杂费摊到行的依据（T-59）：amount 按金额（默认，
	// 与改造前一致）、qty 按数量、volume 按体积、weight 按毛重、cartons 按箱数。
	//
	// 放在票头而不是每行：一票货的分摊方式是一个决定，不是每行各选一次；
	// 而按行选会让「这票运费怎么摊的」要逐行看。
	ChargeAllocBasis string `gorm:"size:8;not null;default:'amount'" json:"charge_alloc_basis"`

	// ForwarderID 是这票货的货代（T-55）。可空：直发业务有时自己订舱。
	// 指向 md_partner 里带 forwarder 角色位的主体，角色由 service 写入时校验。
	// 之前货代只作为费用行的收款方存在，票面上没有落点，四份外销单证就印不出它。
	ForwarderID *uint    `gorm:"index" json:"forwarder_id"`
	Forwarder   *Partner `gorm:"foreignKey:ForwarderID" json:"forwarder,omitempty"`

	// InvoiceRevision 是这一票商票的改证次数：0 是首版，改一次加一。
	//
	// 改证**不换套号**：报关、提单与客户账上都认这个号，换号等于换了一票货。
	// 版本只体现在文件名与打印件上（`-CI-R1`），这样"客户手上是哪一版"能说清楚。
	InvoiceRevision int `gorm:"not null;default:0" json:"invoice_revision"`

	// 物流节点。这四个值是**订舱之后**才知道的，而出货单在草稿阶段就把数量定死了，
	// 所以它们不跟 ShipmentInput 走：走 `PUT /shipments/:id/logistics`，
	// 任何状态都能改。这与"数量只能在草稿改"是两条相反的规则，理由不同：
	// 数量一变，仓库与货代手里的单就白做了；船期一变，只是把实际情况记下来。
	//
	// BLNo 提单号、Vessel 船名航次由承运人给，是**外部号**：只记录，不参与发号与唯一性。
	BLNo   string     `gorm:"size:64" json:"bl_no"`
	Vessel string     `gorm:"size:128" json:"vessel"`
	ETD    *time.Time `gorm:"type:date" json:"etd"`
	ETA    *time.Time `gorm:"type:date" json:"eta"`

	Allocations  []ShipmentAllocation `gorm:"foreignKey:ShipmentID" json:"allocations,omitempty"`
	Declarations []CustomsDeclaration `gorm:"foreignKey:ShipmentID" json:"declarations,omitempty"`
}

func (Shipment) TableName() string { return "biz_shipment" }

// PackingTotals 汇总整票的箱数与毛净重。
//
// 汇总只由分配行相加得出，不在票头上另存一份：两处数字一旦分家，
// 装箱单的合计与明细就对不上，而客户是按明细逐行点数、按合计签收的。
func (s Shipment) PackingTotals() (cartons int, gross, net money.Decimal) {
	for _, a := range s.Allocations {
		cartons += a.Cartons
		gross = gross.Add(a.GrossWeight)
		net = net.Add(a.NetWeight)
	}
	return cartons, gross, net
}

// IsEditable 报告这票出货还能不能改。只有草稿能改：
// 已确认的一票已经被仓库与货代按它备货、订舱，改数量等于让他们白干。
func (s Shipment) IsEditable() bool { return s.Status == DocumentStatusDraft }

// ShipmentAllocation 是"这票货里有哪些销售订单行、各多少"。
//
// 它是订单与出货之间的桥：一条销售订单行可以分到多票，一票也可以装多张订单的行。
// 数量校验（不得超过未出数量）只能落在这一层——订单行上的 qty 是"答应客户多少"，
// 而"还能出多少"要看它已经被多少票分走了。
type ShipmentAllocation struct {
	Base
	ShipmentID uint `gorm:"index;not null" json:"shipment_id"`
	// SalesOrderItemID 指向具体那一行销售订单，不是订单头：
	// 一张订单里两行是同一产品的不同包装，按订单头分配就分不清出的是哪一种。
	SalesOrderItemID uint `gorm:"index;not null" json:"sales_order_item_id"`

	// 快照：出货单要发给仓库与货代，产品改名后单上仍是当时那个说法。
	SKUCode string `gorm:"size:32;not null" json:"sku_code"`
	NameCN  string `gorm:"size:128" json:"name_cn"`
	Spec    string `gorm:"size:128" json:"spec"`
	Unit    string `gorm:"size:16" json:"unit"`
	// BatchNo 是这批货的批次号（T-58）：与收货时填的批次对应，
	// 出货确认后写进销售出库流水，才能从"客户投诉的那一批"追到收货单。
	BatchNo string `gorm:"size:64;index" json:"batch_no"`

	// Qty 是定点数：可出数量的校验全靠逐位比较，"差不多的 0.0001"就是超发。
	Qty    money.Decimal `gorm:"type:decimal(14,3);not null" json:"qty"`
	Remark string        `gorm:"size:255" json:"remark"`

	// 装箱数据：装箱单靠这几项才印得出来。
	//
	// 挂在分配行而不是票头：一票装多种货时，票头上写着"12 箱 / 240kg"
	// 拆不到任何一行，装箱单每行只能留空，客户也就无法按行核对。
	//
	// 重量是**这一行的合计**，不是单箱重量：合计直接相加就对得上，
	// 若按单箱记，改一次箱数就要把所有合计重算一遍，迟早对不上。
	Cartons     int           `gorm:"not null;default:0" json:"cartons"`
	GrossWeight money.Decimal `gorm:"type:decimal(12,3);not null;default:0" json:"gross_weight"`
	NetWeight   money.Decimal `gorm:"type:decimal(12,3);not null;default:0" json:"net_weight"`
	// Measurement 是外箱尺寸，人工填成 "60×40×30CM"。
	//
	// 存文本而不是长宽高三个数字：装箱单只把它原样印出来，系统不算体积、
	// 也不校验格式——分箱方案要到实际装箱时才定（蓝图 §五.4.4）。
	Measurement string `gorm:"size:64" json:"measurement"`
	// Volume 是这一行的**合计**体积（m³），由「箱数 × 产品单箱体积」算出。
	//
	// 单独存而不是查询时现算：订舱按 CBM 计费，一票货报出去之后再改箱规，
	// 报给货代的数字不能跟着变。人工填装箱数据时它保持 0——留空比填一个
	// 按错规格算出来的数更有用，装箱单上那一栏也就不印。
	Volume money.Decimal `gorm:"type:decimal(12,6);not null;default:0" json:"volume"`

	// 报关要素快照（T-55）。出货时从产品抄一份，之后不再跟随产品变化。
	//
	// 不快照的后果：产品上改一次 HS 编码，历史出货的报关单与产地证就
	// 复现不出当时的申报内容——而报关资料是要留档备查的，改一个字都算改单。
	HSCode         string `gorm:"size:16" json:"hs_code"`
	DeclaredNameCN string `gorm:"size:128" json:"declared_name_cn"`
	DeclaredNameEN string `gorm:"size:128" json:"declared_name_en"`
	Material       string `gorm:"size:255" json:"material"`
	OriginCountry  string `gorm:"size:64" json:"origin_country"`
	LegalUnit      string `gorm:"size:16" json:"legal_unit"`

	// SalesOrderItem 只用于展示"这行来自哪张订单"（单号在它下面一层），
	// 不参与任何计算。
	SalesOrderItem *SalesOrderItem `gorm:"foreignKey:SalesOrderItemID" json:"sales_order_item,omitempty"`

	// SalesOrderDocNo 是把单号摊平出来给列表与详情用的只读投影。
	// 不让前端去 allocations[].sales_order_item.sales_order.doc_no 里翻：
	// ShipmentAllocation 嵌的 SalesOrderItem 里那个整单是不回吐接口的。
	SalesOrderDocNo string `gorm:"-" json:"sales_order_doc_no"`
}

func (ShipmentAllocation) TableName() string { return "biz_shipment_allocation" }

// BilingualName 返回"中文名 / English name"（T-64）。
//
// 单证上同时印中英文是外贸惯例：客户看英文、报关行与工厂看中文。
// 相比"另出一份英文版"，双写的优势是**数字天然一致**——
// 两份文件各自的金额、箱数靠人同步，迟早对不上，而客户拿两份对不上就会来问。
//
// 英文名取报关快照里的 declared_name_en（T-55 抄进分配行的那一份），
// 没有就只印中文：正式单证上留一个占位符比缺一行更糟。
func (a ShipmentAllocation) BilingualName() string {
	cn := strings.TrimSpace(a.NameCN)
	en := strings.TrimSpace(a.DeclaredNameEN)
	if cn == "" {
		return en
	}
	if en == "" || en == cn {
		return cn
	}
	return cn + " / " + en
}
