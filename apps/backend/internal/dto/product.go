package dto

import (
	"time"

	"github.com/edwinhuish/gowork/internal/pkg/money"
)

// ProductQuery 产品列表查询条件。
type ProductQuery struct {
	PageQuery
	Category   string            `form:"category"`
	SupplierID *uint             `form:"supplier_id"`
	Status     EnableStatusInput `form:"status"`
	// IncludeDeleted 把已软删的产品也列出来（列表页的「含已删除」开关）。
	// 这些行只该做一件事：恢复——其余编辑入口对它们一律 404。
	IncludeDeleted bool `form:"include_deleted"`

	// SortField / SortOrder 是列表排序。
	//
	// 只认白名单字段（见 repository.sortableProductFields），传别的值一律退回
	// 默认顺序，不报错：排序是个展示偏好，为它挡住整页数据不值得。
	SortField string `form:"sort_field"`
	SortOrder string `form:"sort_order"`
}

// ProductInput 产品新增与编辑共用的入参。
//
// 编辑走全量覆盖（PUT 语义）：前端表单本来就会提交全部可见字段，
// 用同一套结构可以避免 Create/Update 两份几乎相同的校验规则各自漂移。
type ProductInput struct {
	SKUCode    string `json:"sku_code" binding:"required,max=32"`
	NameCN     string `json:"name_cn" binding:"required,max=128"`
	NameEN     string `json:"name_en" binding:"max=128"`
	Category   string `json:"category" binding:"max=32"`
	Spec       string `json:"spec" binding:"max=128"`
	Unit       string `json:"unit" binding:"max=16"`
	SupplierID *uint  `json:"supplier_id"`
	// 金额按字符串收发（money.Decimal），过一遍 float64 就把定点精度丢了。
	// BatchManaged 开启后，这个产品出库必须指定批次号（T-58）。
	BatchManaged bool          `json:"batch_managed"`
	CostPrice    money.Decimal `json:"cost_price"`
	SalePrice    money.Decimal `json:"sale_price"`

	// 包装规格（T-50）：每箱数量、单箱毛净重（kg）、单箱体积（m³）、外箱尺寸。
	// 全部可选；0 表示未维护，出货分配行仍按人工填的合计走。
	PackQty           int           `json:"pack_qty" binding:"gte=0,lte=100000"`
	CartonGrossWeight money.Decimal `json:"carton_gross_weight"`
	CartonNetWeight   money.Decimal `json:"carton_net_weight"`
	CartonVolume      money.Decimal `json:"carton_volume"`
	CartonSize        string        `json:"carton_size" binding:"max=64"`

	// 报关要素的默认值，全部可选：真实报关以出货批次上填的为准，
	// 这里只是省掉每票重复录入。长度上限与 model.Product 的列宽一致。
	HSCode         string `json:"hs_code" binding:"max=16"`
	DeclaredNameCN string `json:"declared_name_cn" binding:"max=128"`
	DeclaredNameEN string `json:"declared_name_en" binding:"max=128"`
	Material       string `json:"material" binding:"max=255"`
	OriginCountry  string `json:"origin_country" binding:"max=64"`
	LegalUnit      string `json:"legal_unit" binding:"max=16"`

	Status EnableStatusInput `json:"status"`
	Remark string            `json:"remark" binding:"max=255"`
	// UpdatedAt 是编辑页打开时读到的版本；带上它做条件更新，期间被别人改过则 409。
	UpdatedAt *time.Time `json:"updated_at"`
}
