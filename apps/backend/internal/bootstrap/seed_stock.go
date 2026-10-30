package bootstrap

import (
	"context"
	"fmt"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/model"
)

// seedStockAndFinance 写入期初库存。
//
// 库存只能由流水改变，所以演示数据不给快照表塞数字，而是造一张**已确认的采购入库单**：
// 确认那一刻才生成流水并推动快照。这样库存页、流水页、出入库单页三处口径天然一致，
// 也不会出现"快照有数、却没有一张单解释它从哪来"的演示数据。
//
// 数量按已出货的量给足：出货确认会生成销售出库单，期初不够的话库存会变负。
func (a *App) seedStockAndFinance(ctx context.Context, d seedDeps, m *demoMaster) error {
	supplier := m.suppliers["SUP001"]
	if supplier == nil {
		return fmt.Errorf("演示数据缺少供应商档案")
	}

	type stockLine struct {
		sku string
		qty string
	}
	lines := []stockLine{
		{"BH01-0001-SS", "9000"},
		{"LK01-0001-BK", "4000"},
		{"BH03-0001-SS", "5000"},
		{"FS02-0001", "5000"},
		{"FS03-0001", "4000"},
		{"LK03-0001", "5000"},
		{"MS01-0001", "4000"},
	}

	items := make([]dto.StockDocumentItemInput, 0, len(lines))
	for _, line := range lines {
		product, ok := m.products[line.sku]
		if !ok {
			continue
		}
		qty := dec(line.qty)
		items = append(items, dto.StockDocumentItemInput{
			ProductID: product.ID,
			Qty:       &qty,
			Remark:    "期初建账",
		})
	}

	doc, err := d.StockDocs.Create(ctx, dto.StockDocumentInput{
		Type:      int8(model.StockDocPurchaseIn),
		DocDate:   day(-100),
		PartnerID: &supplier.ID,
		Remark:    "期初库存建账（演示数据）",
		Items:     items,
	})
	if err != nil {
		return fmt.Errorf("写入期初入库单: %w", err)
	}
	if _, err := d.StockDocs.Confirm(ctx, doc.ID); err != nil {
		return fmt.Errorf("确认期初入库单: %w", err)
	}

	a.Log.Info("已写入期初库存", "lines", len(items))
	return nil
}
