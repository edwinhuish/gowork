package dto

import (
	"time"

	"github.com/edwinhuish/gowork/internal/pkg/money"
)

// StockDocumentQuery 是出入库单的列表查询。
type StockDocumentQuery struct {
	PageQuery
	Type      *int8 `form:"type"`
	Status    *int8 `form:"status"`
	PartnerID *uint `form:"partner_id"`
}

// StockDocumentItemInput 是一行明细。
//
// Qty 用指针：盘点单填 0 是合法结果（盘到没有货），
// 而 `binding:"required"` 会把数值 0 当成"没填"。
// 入库 / 出库的 qty > 0 由 service 校验。
type StockDocumentItemInput struct {
	ProductID uint           `json:"product_id" binding:"required"`
	Qty       *money.Decimal `json:"qty" binding:"required"`
	Remark    string         `json:"remark" binding:"max=255"`
}

// StockDocumentInput 是出入库单的全量入参（新建与编辑共用）。
//
// 没有 status：状态只能由"确认"这个动作推进，不接受前端直接指定。
type StockDocumentInput struct {
	// Type 只接受已实现的类型（采购入库 / 销售出库 / 盘点），
	// 调拨要等仓库维度（T-16b）。
	Type        int8                     `json:"type" binding:"required"`
	DocDate     string                   `json:"doc_date"`
	PartnerID   *uint                    `json:"partner_id"`
	SourceDocNo string                   `json:"source_doc_no" binding:"max=32"`
	Remark      string                   `json:"remark" binding:"max=255"`
	Items       []StockDocumentItemInput `json:"items" binding:"required,min=1,dive"`
	// UpdatedAt 是编辑页打开时读到的版本；带上它做条件更新，期间被别人改过则 409。
	UpdatedAt *time.Time `json:"updated_at"`
}
