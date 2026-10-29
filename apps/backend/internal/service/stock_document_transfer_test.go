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

// TestStockDocTransfer 是调拨接口层的验收点（T-56）。
//
// 与仓储层的那个测试分开：那边验「键与快照增量」，这边验「一次调拨有没有
// 生成一出一入两条流水、分布对不对、两个仓能不能是同一条」。
// 混在一起，失败时说不清是键写错还是编排写错。
func TestStockDocTransfer(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 10, 30, 10, 0, 0, 0, time.Local)
	clock := func() time.Time { return now }

	db := openTestDB(t,
		&model.Product{}, &model.Partner{}, &model.DocSeq{},
		&model.StockMovement{}, &model.StockSnapshot{},
		&model.StockDocument{}, &model.StockDocumentItem{},
		&model.Warehouse{})
	productRepo := repository.NewProductRepository(db)
	docNoSvc := NewDocNoService(repository.NewDocSeqRepository(db), WithDocNoClock(clock))
	docs := NewStockDocumentService(
		repository.NewStockDocumentRepository(db), productRepo,
		repository.NewPartnerRepository(db), docNoSvc,
		WithStockDocumentClock(clock),
		WithStockDocumentLogger(slog.New(slog.DiscardHandler)),
	)

	p := &model.Product{SKUCode: "BH01-0001", NameCN: "不锈钢合页", Unit: "PCS",
		Status: model.EnableStatusOn}
	if err := productRepo.Create(ctx, p); err != nil {
		t.Fatalf("建产品: %v", err)
	}

	a := &model.Warehouse{Code: "A", Name: "主仓", IsDefault: true, Status: model.EnableStatusOn}
	if err := db.Create(a).Error; err != nil {
		t.Fatalf("建仓 A: %v", err)
	}
	b := &model.Warehouse{Code: "B", Name: "分仓", Status: model.EnableStatusOn}
	if err := db.Create(b).Error; err != nil {
		t.Fatalf("建仓 B: %v", err)
	}

	// 先在 A 入 100，作为调拨的来源库存。
	in := dto.StockDocumentInput{
		Type:        int8(model.StockDocPurchaseIn),
		WarehouseID: &a.ID,
		Items:       []dto.StockDocumentItemInput{{ProductID: p.ID, Qty: qtyPtr(100)}},
	}
	doc, err := docs.Create(ctx, in)
	if err != nil {
		t.Fatalf("建入库单: %v", err)
	}
	if _, err := docs.Confirm(ctx, doc.ID); err != nil {
		t.Fatalf("确认入库单: %v", err)
	}

	snapshotQty := func(warehouseID uint) money.Decimal {
		t.Helper()
		var rows []struct{ Qty money.Decimal }
		if err := db.Raw(
			"SELECT qty FROM inv_stock_snapshot WHERE product_id = ? AND warehouse_id = ? AND deleted_at = 0",
			p.ID, warehouseID).Scan(&rows).Error; err != nil {
			t.Fatalf("查快照: %v", err)
		}
		if len(rows) == 0 {
			return money.Zero()
		}
		return rows[0].Qty
	}

	// 源仓与目标仓相同：拒绝。
	same := dto.StockDocumentInput{
		Type:              int8(model.StockDocTransfer),
		WarehouseID:       &a.ID,
		TargetWarehouseID: &a.ID,
		Items:             []dto.StockDocumentItemInput{{ProductID: p.ID, Qty: qtyPtr(10)}},
	}
	if _, err := docs.Create(ctx, same); err == nil {
		t.Fatal("源仓与目标仓相同应该被拒")
	} else if !hasErrCode(err, errs.CodeInvalidParam) {
		t.Fatalf("应为 400，实际 %v", err)
	}

	// 正常调拨：A 出 40、B 入 40。
	tr := dto.StockDocumentInput{
		Type:              int8(model.StockDocTransfer),
		WarehouseID:       &a.ID,
		TargetWarehouseID: &b.ID,
		Items:             []dto.StockDocumentItemInput{{ProductID: p.ID, Qty: qtyPtr(40)}},
	}
	tdoc, err := docs.Create(ctx, tr)
	if err != nil {
		t.Fatalf("建调拨单: %v", err)
	}
	if _, err := docs.Confirm(ctx, tdoc.ID); err != nil {
		t.Fatalf("确认调拨单: %v", err)
	}

	// 两条流水：一个 out（A）、一个 in（B）。
	var movements []model.StockMovement
	if err := db.Where("source_type = ? AND source_id = ?",
		model.StockSourceStockDocument, tdoc.ID).Find(&movements).Error; err != nil {
		t.Fatalf("查调拨流水: %v", err)
	}
	if len(movements) != 2 {
		t.Fatalf("一次调拨应生成两条流水，实际 %d 条", len(movements))
	}
	outSeen, inSeen := false, false
	for _, m := range movements {
		if m.Direction == model.StockDirectionOut && m.WarehouseID == a.ID {
			outSeen = true
		}
		if m.Direction == model.StockDirectionIn && m.WarehouseID == b.ID {
			inSeen = true
		}
	}
	if !outSeen || !inSeen {
		t.Fatalf("应为「A 出、B 入」，实际 %+v", movements)
	}

	qa, qb := snapshotQty(a.ID), snapshotQty(b.ID)
	if qa.Cmp(money.MustParse("60")) != 0 {
		t.Errorf("调拨后仓 A 应为 60，得到 %s", qa)
	}
	if qb.Cmp(money.MustParse("40")) != 0 {
		t.Errorf("调拨后仓 B 应为 40，得到 %s", qb)
	}
	if total := qa.Add(qb); total.Cmp(money.MustParse("100")) != 0 {
		t.Errorf("调拨不改总量：应为 100，得到 %s", total)
	}
}
