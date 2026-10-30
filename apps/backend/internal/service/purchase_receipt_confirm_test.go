package service

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/money"
	"github.com/edwinhuish/gowork/internal/repository"
)

// TestPurchaseReceiptConfirmIncreasesStock 验收货确认的副作用（T-51a）。
//
// 收货是「采购 → 库存」的唯一入口：它错了，后面所有库存数都不可信。
// 已有测试只覆盖超收校验（stockIn 传 nil），确认之后库存有没有真的增加、
// 会不会重复增加，此前完全没验过。
func TestPurchaseReceiptConfirmIncreasesStock(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 10, 30, 10, 0, 0, 0, time.Local)
	clock := func() time.Time { return now }

	db := openTestDB(t,
		&model.Product{}, &model.Partner{}, &model.Warehouse{}, &model.DocSeq{},
		&model.PurchaseOrder{}, &model.PurchaseOrderItem{},
		&model.PurchaseReceipt{}, &model.PurchaseReceiptItem{},
		&model.StockMovement{}, &model.StockSnapshot{},
		&model.StockDocument{}, &model.StockDocumentItem{})

	productRepo := repository.NewProductRepository(db)
	product := &model.Product{SKUCode: "BH01-0001", NameCN: "不锈钢合页", Unit: "PCS"}
	if err := productRepo.Create(ctx, product); err != nil {
		t.Fatalf("建产品: %v", err)
	}
	// 默认仓：确认入库时留空 warehouse_id 会落到它。
	wh := &model.Warehouse{Code: "A", Name: "主仓", IsDefault: true, Status: model.EnableStatusOn}
	if err := db.Create(wh).Error; err != nil {
		t.Fatalf("建仓: %v", err)
	}

	order := &model.PurchaseOrder{
		DocNo: "PO-2610-0001", Status: model.DocumentStatusConfirmed,
		OrderDate: model.DateOnly(now), CurrencyCode: "CNY",
	}
	if err := db.Create(order).Error; err != nil {
		t.Fatalf("建采购单: %v", err)
	}
	item := &model.PurchaseOrderItem{
		PurchaseOrderID: order.ID, ProductID: product.ID,
		SKUCode: product.SKUCode, NameCN: product.NameCN, Unit: product.Unit,
		Qty: money.FromInt(100),
	}
	if err := db.Create(item).Error; err != nil {
		t.Fatalf("建采购明细: %v", err)
	}

	docNoSvc := NewDocNoService(repository.NewDocSeqRepository(db), WithDocNoClock(clock))
	txs := repository.NewTxManager(db)
	stockDocSvc := NewStockDocumentService(
		repository.NewStockDocumentRepository(db), productRepo,
		repository.NewPartnerRepository(db), docNoSvc,
		WithStockDocumentClock(clock),
		WithStockDocumentLogger(slog.New(slog.DiscardHandler)))
	svc := NewPurchaseReceiptService(
		repository.NewPurchaseReceiptRepository(db),
		repository.NewPurchaseOrderRepository(db),
		docNoSvc, txs, stockDocSvc,
		WithPurchaseReceiptLogger(slog.New(slog.DiscardHandler)))

	receipt, err := svc.Create(ctx, order.ID, dto.PurchaseReceiptInput{
		Items: []dto.PurchaseReceiptItemInput{
			{PurchaseOrderItemID: item.ID, Qty: money.FromInt(40)},
		},
	})
	if err != nil {
		t.Fatalf("建收货单: %v", err)
	}

	stockQty := func() money.Decimal {
		t.Helper()
		var rows []struct{ Qty money.Decimal }
		if err := db.Raw(
			"SELECT qty FROM inv_stock_snapshot WHERE product_id = ? AND deleted_at = 0",
			product.ID).Scan(&rows).Error; err != nil {
			t.Fatalf("查快照: %v", err)
		}
		if len(rows) == 0 {
			return money.Zero()
		}
		return rows[0].Qty
	}

	// 草稿不动库存：这是"收货单 vs 入库单"两张单的根本区别。
	if got := stockQty(); !got.IsZero() {
		t.Fatalf("草稿不该动库存，实际 %s", got)
	}

	if _, err := svc.Confirm(ctx, receipt.ID); err != nil {
		t.Fatalf("确认收货单: %v", err)
	}
	if got := stockQty(); got.Cmp(money.FromInt(40)) != 0 {
		t.Errorf("确认后库存应为 40，得到 %s", got)
	}

	// 再确认一次必须被拦：状态已经不是草稿。
	if _, err := svc.Confirm(ctx, receipt.ID); err == nil {
		t.Fatal("重复确认应被拒绝")
	}

	// 库存只加了一次。
	if got := stockQty(); got.Cmp(money.FromInt(40)) != 0 {
		t.Errorf("重复确认不该再增加库存，实际 %s", got)
	}

	// 入库单必须存在且引用收货单号：否则"这批货是从哪张收货单进来的"答不出来。
	var docs []model.StockDocument
	if err := db.Where("source_doc_no = ?", receipt.DocNo).Find(&docs).Error; err != nil {
		t.Fatalf("查入库单: %v", err)
	}
	if len(docs) != 1 {
		t.Fatalf("应有一张采购入库单，实际 %d 张", len(docs))
	}
	if docs[0].Type != model.StockDocPurchaseIn {
		t.Errorf("入库单类型应为采购入库，实际 %v", docs[0].Type)
	}
}
