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

// TestPurchaseReceiptOverReceive 是 T-51a 的核心验收点：累计收货不能超过订购量。
//
// 超收不是"多收一点没关系"：它会让在途量变成负数，也让采购金额与入库数量
// 从此对不上；而判据必须把**草稿**一起算——只算已确认的话，同一个人可以开两张
// 草稿各收一半，再一起确认，最终收成两倍。
func TestPurchaseReceiptOverReceive(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t,
		&model.PurchaseOrder{}, &model.PurchaseOrderItem{},
		&model.PurchaseReceipt{}, &model.PurchaseReceiptItem{}, &model.DocSeq{})

	order := &model.PurchaseOrder{
		DocNo:        "PO-2610-0001",
		Status:       model.DocumentStatusConfirmed,
		OrderDate:    model.DateOnly(time.Now()),
		CurrencyCode: "CNY",
	}
	if err := db.Create(order).Error; err != nil {
		t.Fatalf("建采购单失败: %v", err)
	}
	item := &model.PurchaseOrderItem{
		PurchaseOrderID: order.ID, ProductID: 1,
		SKUCode: "BH01-0001", NameCN: "不锈钢合页", Unit: "PCS", Qty: money.FromInt(100),
	}
	if err := db.Create(item).Error; err != nil {
		t.Fatalf("建采购明细失败: %v", err)
	}

	svc := NewPurchaseReceiptService(
		repository.NewPurchaseReceiptRepository(db),
		repository.NewPurchaseOrderRepository(db),
		NewDocNoService(repository.NewDocSeqRepository(db)),
		repository.NewTxManager(db),
		nil,
	)

	if _, err := svc.Create(ctx, order.ID, dto.PurchaseReceiptInput{
		Items: []dto.PurchaseReceiptItemInput{{PurchaseOrderItemID: item.ID, Qty: money.FromInt(80)}},
	}); err != nil {
		t.Fatalf("收 80 应成功: %v", err)
	}

	// 再收 30：草稿 80 已占量，80+30 > 100。
	_, err := svc.Create(ctx, order.ID, dto.PurchaseReceiptInput{
		Items: []dto.PurchaseReceiptItemInput{{PurchaseOrderItemID: item.ID, Qty: money.FromInt(30)}},
	})
	if !hasErrCode(err, errs.CodeConflict) {
		t.Fatalf("累计超收应返回 409，得到 %v", err)
	}

	// 收 20 正好补齐：草稿也算量，所以 80+20 等于订购量，允许。
	if _, err := svc.Create(ctx, order.ID, dto.PurchaseReceiptInput{
		Items: []dto.PurchaseReceiptItemInput{{PurchaseOrderItemID: item.ID, Qty: money.FromInt(20)}},
	}); err != nil {
		t.Fatalf("收 20 补齐应成功: %v", err)
	}

	// 收 0 件直接拒绝：空行会让在途量算错，也让"这批到了没有"变成一个谜。
	if _, err := svc.Create(ctx, order.ID, dto.PurchaseReceiptInput{
		Items: []dto.PurchaseReceiptItemInput{{PurchaseOrderItemID: item.ID, Qty: money.Zero()}},
	}); !hasErrCode(err, errs.CodeInvalidParam) {
		t.Fatalf("收 0 件应返回 400，得到 %v", err)
	}
}
