package service

import (
	"context"
	"testing"
	"time"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/errs"
	"github.com/edwinhuish/gowork/internal/pkg/money"
	"github.com/edwinhuish/gowork/internal/repository"
)

// TestPurchaseReturnOverReturn 是 T-52b3 的验收点：累计退货不得超过收货量。
//
// 退超过收到的，库存会变成凭空多出来的负数；而对账时最难解释的正是
// 「账上少了 20 件，仓库里一件没多」。
//
// 这里只验校验：超退与 0 件都在调用出入库服务之前就被拦下，所以 stockIn 传 nil
// 也不会被用到。真正的出库（含库存减少）由收货面板的端到端流程覆盖。
func TestPurchaseReturnOverReturn(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t,
		&model.PurchaseOrder{}, &model.PurchaseOrderItem{},
		&model.PurchaseReceipt{}, &model.PurchaseReceiptItem{}, &model.DocSeq{})

	order := &model.PurchaseOrder{
		DocNo:        "PO-2610-0002",
		Status:       model.DocumentStatusConfirmed,
		OrderDate:    model.DateOnly(time.Now()),
		CurrencyCode: "CNY",
	}
	if err := db.Create(order).Error; err != nil {
		t.Fatalf("建采购单失败: %v", err)
	}

	receipt := &model.PurchaseReceipt{
		DocNo:           "RCV-202610-0001",
		PurchaseOrderID: order.ID,
		Status:          model.DocumentStatusConfirmed,
		ReceivedDate:    model.DateOnly(time.Now()),
	}
	if err := db.Create(receipt).Error; err != nil {
		t.Fatalf("建收货单失败: %v", err)
	}
	item := &model.PurchaseReceiptItem{
		ReceiptID: receipt.ID, PurchaseOrderItemID: 1, ProductID: 1,
		SKUCode: "BH01-0001", NameCN: "不锈钢合页", Unit: "PCS",
		Qty: money.FromInt(100), ReturnedQty: money.FromInt(90),
	}
	if err := db.Create(item).Error; err != nil {
		t.Fatalf("建收货明细失败: %v", err)
	}

	svc := NewPurchaseReceiptService(
		repository.NewPurchaseReceiptRepository(db),
		repository.NewPurchaseOrderRepository(db),
		NewDocNoService(repository.NewDocSeqRepository(db)),
		repository.NewTxManager(db),
		nil,
	)

	// 已退 90，再退 20 → 110 > 100。
	_, err := svc.Return(ctx, receipt.ID, dto.PurchaseReturnInput{
		Items: []dto.PurchaseReturnItemInput{
			{PurchaseReceiptItemID: item.ID, Qty: money.FromInt(20)},
		},
	})
	if !hasErrCode(err, errs.CodeConflict) {
		t.Fatalf("累计超退应返回 409，得到 %v", err)
	}

	// 退 0 件直接拒绝：空行会让累计校验变成一次无意义的写库。
	if _, err := svc.Return(ctx, receipt.ID, dto.PurchaseReturnInput{
		Items: []dto.PurchaseReturnItemInput{
			{PurchaseReceiptItemID: item.ID, Qty: money.Zero()},
		},
	}); !hasErrCode(err, errs.CodeInvalidParam) {
		t.Fatalf("退 0 件应返回 400，得到 %v", err)
	}
}
