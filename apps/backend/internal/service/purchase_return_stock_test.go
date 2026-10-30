package service

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/errs"
	"github.com/edwinhuish/gowork/internal/pkg/money"
	"github.com/edwinhuish/gowork/internal/repository"
)

// TestPurchaseReturnReducesStock 验采购退货的副作用（T-52b3）。
//
// 已有测试只覆盖「超退被拦」与「退 0 件被拒」，两条都在调出入库服务之前就返回了——
// 也就是说"退货真的把库存减掉了"这件事从没验过。而它与刚写的收货测试是同一条
// 链的两侧：一个加库存，一个减库存，方向错了库存会越退越多。
func TestPurchaseReturnReducesStock(t *testing.T) {
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
	receiptSvc := NewPurchaseReceiptService(
		repository.NewPurchaseReceiptRepository(db),
		repository.NewPurchaseOrderRepository(db),
		docNoSvc, txs, stockDocSvc,
		WithPurchaseReceiptLogger(slog.New(slog.DiscardHandler)))

	// 先收 100 并确认：库存 100。
	receipt, err := receiptSvc.Create(ctx, order.ID, dto.PurchaseReceiptInput{
		Items: []dto.PurchaseReceiptItemInput{
			{PurchaseOrderItemID: item.ID, Qty: money.FromInt(100)},
		},
	})
	if err != nil {
		t.Fatalf("建收货单: %v", err)
	}
	if _, err := receiptSvc.Confirm(ctx, receipt.ID); err != nil {
		t.Fatalf("确认收货单: %v", err)
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
	if got := stockQty(); got.Cmp(money.FromInt(100)) != 0 {
		t.Fatalf("收货后库存应为 100，实际 %s", got)
	}

	// 退 30。
	detail, err := receiptSvc.Get(ctx, receipt.ID)
	if err != nil {
		t.Fatalf("取收货单详情: %v", err)
	}
	if len(detail.Items) == 0 {
		t.Fatal("收货单应有明细")
	}
	returnedItemID := detail.Items[0].ID

	if _, err := receiptSvc.Return(ctx, receipt.ID, dto.PurchaseReturnInput{
		Reason: "焊点开裂",
		Items: []dto.PurchaseReturnItemInput{
			{PurchaseReceiptItemID: returnedItemID, Qty: money.FromInt(30)},
		},
	}); err != nil {
		t.Fatalf("退货: %v", err)
	}

	if got := stockQty(); got.Cmp(money.FromInt(70)) != 0 {
		t.Errorf("退 30 之后库存应为 70，实际 %s", got)
	}

	// 已退量要落库：否则下一次退货的累计校验从 0 开始，能退成两倍。
	detail, err = receiptSvc.Get(ctx, receipt.ID)
	if err != nil {
		t.Fatalf("退货后取收货单: %v", err)
	}
	if got := detail.Items[0].ReturnedQty; got.Cmp(money.FromInt(30)) != 0 {
		t.Errorf("已退数量应为 30，实际 %s", got)
	}

	// 再退 80：累计 110 > 100，必须拦下且库存不变。
	_, err = receiptSvc.Return(ctx, receipt.ID, dto.PurchaseReturnInput{
		Items: []dto.PurchaseReturnItemInput{
			{PurchaseReceiptItemID: returnedItemID, Qty: money.FromInt(80)},
		},
	})
	if err == nil {
		t.Fatal("累计退货超过收货量应被拒绝")
	}
	if !hasErrCode(err, errs.CodeConflict) {
		t.Fatalf("超退应为 409，实际 %v", err)
	}
	if got := stockQty(); got.Cmp(money.FromInt(70)) != 0 {
		t.Errorf("被拒的退货不该动库存，实际 %s", got)
	}

	// 退货出库单要能追到收货单号。
	var docs []model.StockDocument
	if err := db.Where("type = ?", model.StockDocPurchaseReturn).Find(&docs).Error; err != nil {
		t.Fatalf("查退货出库单: %v", err)
	}
	if len(docs) != 1 {
		t.Fatalf("应有一张采购退货出库单，实际 %d 张", len(docs))
	}
}
