package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/errs"
)

// T-40：出货确认即出库。三条验收各有用例：
// 确认后库存减少且流水能追到出库单号、未确认不产生流水、重复确认 409。
func TestShipmentConfirmIssuesSalesOut(t *testing.T) {
	now := time.Date(2026, 10, 28, 10, 0, 0, 0, time.Local)
	fx := newShipmentFixture(t, &now)
	ctx := context.Background()
	itemA := fx.orderA.Items[0]

	ship, err := fx.shipments.Create(ctx, dto.ShipmentInput{
		Allocations: []dto.ShipmentAllocationInput{{SalesOrderItemID: itemA.ID, Qty: qtyOf(60)}},
	})
	if err != nil {
		t.Fatalf("建出货批次失败: %v", err)
	}

	countDocs := func() int64 {
		var n int64
		if err := fx.db.Model(&model.StockDocument{}).
			Where("type = ?", model.StockDocSalesOut).Count(&n).Error; err != nil {
			t.Fatalf("统计出库单失败: %v", err)
		}
		return n
	}

	// 草稿 / 已确认 / 备货中：都还没出货，库存不该动。
	for _, status := range []model.DocumentStatus{
		model.DocumentStatusConfirmed, model.DocumentStatusPreparing,
	} {
		if _, err := fx.shipments.UpdateStatus(ctx, ship.ID,
			dto.ShipmentStatusInput{Status: int8(status)}); err != nil {
			t.Fatalf("流转到 %s 失败: %v", status.Label(), err)
		}
		if n := countDocs(); n != 0 {
			t.Fatalf("到「%s」为止不该生成出库单，得到 %d 张", status.Label(), n)
		}
	}

	// 确认出货（备货中 → 已出货）。
	if _, err := fx.shipments.UpdateStatus(ctx, ship.ID,
		dto.ShipmentStatusInput{Status: int8(model.DocumentStatusShipped)}); err != nil {
		t.Fatalf("确认出货失败: %v", err)
	}

	var doc model.StockDocument
	if err := fx.db.Where("type = ? AND source_doc_no = ?",
		model.StockDocSalesOut, ship.DocNo).First(&doc).Error; err != nil {
		t.Fatalf("应生成一张销售出库单: %v", err)
	}
	if doc.Status != model.DocumentStatusConfirmed {
		t.Errorf("出库单应已确认（出货是既成事实），得到 %s", doc.Status.Label())
	}
	var items []model.StockDocumentItem
	if err := fx.db.Where("document_id = ?", doc.ID).Find(&items).Error; err != nil {
		t.Fatalf("查出库明细失败: %v", err)
	}
	if len(items) != 1 || !approxQty(items[0].Qty, 60) {
		t.Fatalf("出库明细应是一行 60 件，得到 %+v", items)
	}

	// 流水：能追到这张出库单，数量为出。
	var movements []model.StockMovement
	if err := fx.db.Where("source_id = ? AND source_type = ?",
		doc.ID, model.StockSourceStockDocument).Find(&movements).Error; err != nil {
		t.Fatalf("查流水失败: %v", err)
	}
	if len(movements) != 1 {
		t.Fatalf("应有一条出库流水，得到 %d", len(movements))
	}
	if movements[0].SourceDocNo != doc.DocNo {
		t.Errorf("流水要能追到出库单号，得到 %q，期望 %q", movements[0].SourceDocNo, doc.DocNo)
	}
	if !strings.HasPrefix(doc.DocNo, "OUT-") {
		t.Errorf("出库单号前缀应为 OUT-，得到 %q", doc.DocNo)
	}
	if !approxQty(movements[0].SignedQty(), -60) {
		t.Errorf("出库流水的有符号数量应为 -60，得到 %v", movements[0].SignedQty())
	}

	// 快照：没有入库时库存应为 -60（单池模型下的负数就是负库存）。
	var snaps []model.StockSnapshot
	if err := fx.db.Where("product_id = ?", itemA.ProductID).Find(&snaps).Error; err != nil {
		t.Fatalf("查库存快照失败: %v", err)
	}
	if len(snaps) != 1 || !approxQty(snaps[0].Qty, -60) {
		t.Fatalf("库存快照应为 -60，得到 %+v", snaps)
	}

	t.Run("重复确认 409", func(t *testing.T) {
		_, err := fx.shipments.UpdateStatus(ctx, ship.ID,
			dto.ShipmentStatusInput{Status: int8(model.DocumentStatusShipped)})
		wantCode(t, err, errs.CodeConflict, "已出货再确认")
		if n := countDocs(); n != 1 {
			t.Errorf("重复确认不该出第二张出库单，得到 %d 张", n)
		}
	})

	t.Run("未出货就取消不产生流水", func(t *testing.T) {
		other, err := fx.shipments.Create(ctx, dto.ShipmentInput{
			Allocations: []dto.ShipmentAllocationInput{{SalesOrderItemID: itemA.ID, Qty: qtyOf(10)}},
		})
		if err != nil {
			t.Fatalf("建第二票失败: %v", err)
		}
		if _, err := fx.shipments.UpdateStatus(ctx, other.ID,
			dto.ShipmentStatusInput{Status: int8(model.DocumentStatusCancelled)}); err != nil {
			t.Fatalf("取消失败: %v", err)
		}
		if n := countDocs(); n != 1 {
			t.Errorf("取消未出货的批次不该产生出库单，得到 %d 张", n)
		}
	})
}

// 出库失败时状态不能改：宁可这次确认失败，也不要一张"已出货但库存没减"的单。
func TestShipmentConfirmRollsBackStatusOnStockFailure(t *testing.T) {
	now := time.Date(2026, 10, 28, 10, 0, 0, 0, time.Local)
	fx := newShipmentFixture(t, &now)
	ctx := context.Background()
	itemA := fx.orderA.Items[0]

	ship, err := fx.shipments.Create(ctx, dto.ShipmentInput{
		Allocations: []dto.ShipmentAllocationInput{{SalesOrderItemID: itemA.ID, Qty: qtyOf(5)}},
	})
	if err != nil {
		t.Fatalf("建出货批次失败: %v", err)
	}
	for _, status := range []model.DocumentStatus{
		model.DocumentStatusConfirmed, model.DocumentStatusPreparing,
	} {
		if _, err := fx.shipments.UpdateStatus(ctx, ship.ID,
			dto.ShipmentStatusInput{Status: int8(status)}); err != nil {
			t.Fatalf("流转失败: %v", err)
		}
	}

	// 人为让出库失败：先把这票的"来源单号"占掉，确认时就会撞上判重。
	// （比替换仓储更省事，也顺带验证了判重这道闸。）
	if err := fx.db.Create(&model.StockDocument{
		DocNo: "OUT-TOOK", Type: model.StockDocSalesOut,
		Status: model.DocumentStatusConfirmed, DocDate: model.DateOnly(now),
		SourceDocNo: ship.DocNo,
	}).Error; err != nil {
		t.Fatalf("造占位出库单失败: %v", err)
	}

	_, err = fx.shipments.UpdateStatus(ctx, ship.ID,
		dto.ShipmentStatusInput{Status: int8(model.DocumentStatusShipped)})
	wantCode(t, err, errs.CodeConflict, "重复出库")

	after, err := fx.shipments.Get(ctx, ship.ID)
	if err != nil {
		t.Fatalf("重读出货单失败: %v", err)
	}
	if after.Status == model.DocumentStatusShipped {
		t.Error("出库失败时不该把状态改成已出货")
	}
}
