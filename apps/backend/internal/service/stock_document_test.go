package service

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/errs"
	"github.com/edwinhuish/gowork/internal/pkg/money"
	"github.com/edwinhuish/gowork/internal/repository"
)

type stockDocFixture struct {
	docs    StockDocumentService
	stock   StockService
	db      *gorm.DB
	product *model.Product
}

func newStockDocFixture(t *testing.T, now *time.Time) stockDocFixture {
	t.Helper()
	ctx := context.Background()

	db := openTestDB(t,
		&model.Product{}, &model.Partner{}, &model.DocSeq{},
		&model.StockMovement{}, &model.StockSnapshot{},
		&model.StockDocument{}, &model.StockDocumentItem{})

	clock := func() time.Time { return *now }
	productRepo := repository.NewProductRepository(db)
	stockRepo := repository.NewStockRepository(db)
	docNoSvc := NewDocNoService(repository.NewDocSeqRepository(db), WithDocNoClock(clock))

	p := &model.Product{SKUCode: "BH01-0001", NameCN: "不锈钢合页", Spec: "100×75×3",
		Unit: "PCS", Status: model.EnableStatusOn}
	if err := productRepo.Create(ctx, p); err != nil {
		t.Fatalf("建产品失败: %v", err)
	}

	return stockDocFixture{
		docs: NewStockDocumentService(
			repository.NewStockDocumentRepository(db), productRepo,
			repository.NewPartnerRepository(db), docNoSvc,
			WithStockDocumentClock(clock),
			WithStockDocumentLogger(slog.New(slog.DiscardHandler)),
		),
		stock: NewStockService(stockRepo, productRepo, docNoSvc,
			WithStockClock(clock),
			WithStockLogger(slog.New(slog.DiscardHandler))),
		db:      db,
		product: p,
	}
}

func qtyPtr(v float64) *money.Decimal {
	d := money.FromFloat(v)
	return &d
}

// create 建一张草稿单。
func (f stockDocFixture) create(
	t *testing.T, typ model.StockDocType, rows ...dto.StockDocumentItemInput,
) *model.StockDocument {
	t.Helper()
	doc, err := f.docs.Create(context.Background(), dto.StockDocumentInput{
		Type:  int8(typ),
		Items: rows,
	})
	if err != nil {
		t.Fatalf("建出入库单失败: %v", err)
	}
	return doc
}

func (f stockDocFixture) confirm(t *testing.T, id uint) *model.StockDocument {
	t.Helper()
	doc, err := f.docs.Confirm(context.Background(), id)
	if err != nil {
		t.Fatalf("确认出入库单失败: %v", err)
	}
	return doc
}

// qty 读当前库存（只有一个产品，所以不看 product_id）。
func (f stockDocFixture) qty(t *testing.T) money.Decimal {
	t.Helper()
	var s model.StockSnapshot
	if err := f.db.Where("product_id = ?", f.product.ID).First(&s).Error; err != nil {
		t.Fatalf("查快照失败: %v", err)
	}
	return s.Qty
}

func (f stockDocFixture) movementCount(t *testing.T) int64 {
	t.Helper()
	var n int64
	if err := f.db.Model(&model.StockMovement{}).Count(&n).Error; err != nil {
		t.Fatalf("统计流水失败: %v", err)
	}
	return n
}

func TestStockDocumentPurchaseIn(t *testing.T) {
	now := time.Date(2026, 10, 28, 10, 0, 0, 0, time.Local)
	fx := newStockDocFixture(t, &now)

	doc := fx.create(t, model.StockDocPurchaseIn,
		dto.StockDocumentItemInput{ProductID: fx.product.ID, Qty: qtyPtr(100)})

	if doc.DocNo != "IN-202610-0001" {
		t.Errorf("单号 = %q，期望 IN-202610-0001", doc.DocNo)
	}
	if doc.Status != model.DocumentStatusDraft {
		t.Errorf("新单状态 = %d，期望草稿", doc.Status)
	}
	if len(doc.Items) != 1 || doc.Items[0].SKUCode != "BH01-0001" {
		t.Errorf("明细缺产品快照: %+v", doc.Items)
	}
	// 草稿不动库存：这是"先核对后入账"的全部意义。
	if n := fx.movementCount(t); n != 0 {
		t.Errorf("草稿产生了 %d 条流水", n)
	}
	var snap model.StockSnapshot
	err := fx.db.Where("product_id = ?", fx.product.ID).First(&snap).Error
	if err == nil && !snap.Qty.IsZero() {
		t.Errorf("草稿改动了库存：%v", snap.Qty)
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("查快照失败: %v", err)
	}

	posted := fx.confirm(t, doc.ID)
	if posted.Status != model.DocumentStatusConfirmed {
		t.Errorf("确认后状态 = %d，期望已确认", posted.Status)
	}
	if posted.PostedAt == nil {
		t.Error("确认后应记录确认时间")
	}
	if got := fx.qty(t); !approxQty(got, 100) {
		t.Errorf("确认后库存 = %v，期望 100", got)
	}
	if n := fx.movementCount(t); n != 1 {
		t.Fatalf("流水数 = %d，期望 1", n)
	}

	var mv model.StockMovement
	if err := fx.db.First(&mv).Error; err != nil {
		t.Fatalf("查流水失败: %v", err)
	}
	if mv.Type != model.StockMovementPurchase || mv.Direction != model.StockDirectionIn {
		t.Errorf("流水类型 / 方向 = %v / %v", mv.Type, mv.Direction)
	}
	if mv.SourceType != model.StockSourceStockDocument ||
		mv.SourceDocNo != doc.DocNo || mv.SourceID != doc.ID {
		t.Errorf("流水缺来源单据: %+v", mv)
	}

	t.Run("确认后不可改不可删也不可再确认", func(t *testing.T) {
		ctx := context.Background()
		if _, err := fx.docs.Update(ctx, doc.ID, dto.StockDocumentInput{
			Type:  int8(model.StockDocPurchaseIn),
			Items: []dto.StockDocumentItemInput{{ProductID: fx.product.ID, Qty: qtyPtr(1)}},
		}); !isConflict(err) {
			t.Errorf("已确认改单应 409，实际 %v", err)
		}
		if err := fx.docs.Delete(ctx, doc.ID); !isConflict(err) {
			t.Errorf("已确认删单应 409，实际 %v", err)
		}
		if _, err := fx.docs.Confirm(ctx, doc.ID); !isConflict(err) {
			t.Errorf("重复确认应 409，实际 %v", err)
		}
		// 三次被拒之后库存必须原封不动。
		if got := fx.qty(t); !approxQty(got, 100) {
			t.Errorf("被拒的操作改动了库存：%v", got)
		}
	})
}

func TestStockDocumentSalesOut(t *testing.T) {
	now := time.Date(2026, 10, 28, 10, 0, 0, 0, time.Local)
	fx := newStockDocFixture(t, &now)

	fx.confirm(t, fx.create(t, model.StockDocPurchaseIn,
		dto.StockDocumentItemInput{ProductID: fx.product.ID, Qty: qtyPtr(50)}).ID)

	doc := fx.create(t, model.StockDocSalesOut,
		dto.StockDocumentItemInput{ProductID: fx.product.ID, Qty: qtyPtr(80)})
	if doc.DocNo != "OUT-202610-0001" {
		t.Errorf("单号 = %q，期望 OUT-202610-0001", doc.DocNo)
	}
	fx.confirm(t, doc.ID)

	// 允许负库存：直发业务里"先出货、后补入库单"是常态。
	if got := fx.qty(t); !approxQty(got, -30) {
		t.Errorf("库存 = %v，期望 -30", got)
	}
}

func TestStockDocumentCount(t *testing.T) {
	now := time.Date(2026, 10, 28, 10, 0, 0, 0, time.Local)
	fx := newStockDocFixture(t, &now)

	fx.confirm(t, fx.create(t, model.StockDocPurchaseIn,
		dto.StockDocumentItemInput{ProductID: fx.product.ID, Qty: qtyPtr(100)}).ID)

	t.Run("盘亏生成出库流水并回写账面数", func(t *testing.T) {
		doc := fx.create(t, model.StockDocCount,
			dto.StockDocumentItemInput{ProductID: fx.product.ID, Qty: qtyPtr(95)})
		if doc.DocNo != "CT-202610-0001" {
			t.Errorf("单号 = %q，期望 CT-202610-0001", doc.DocNo)
		}
		posted := fx.confirm(t, doc.ID)

		if posted.Items[0].BookQty == nil || !approxQty(*posted.Items[0].BookQty, 100) {
			t.Errorf("未回写账面数: %+v", posted.Items[0])
		}
		if got := fx.qty(t); !approxQty(got, 95) {
			t.Errorf("库存 = %v，期望 95", got)
		}

		var mv model.StockMovement
		if err := fx.db.Where("source_type = ?", model.StockSourceStockDocument).
			Order("id DESC").First(&mv).Error; err != nil {
			t.Fatalf("查流水失败: %v", err)
		}
		if mv.Type != model.StockMovementCount || mv.Direction != model.StockDirectionOut ||
			!approxQty(mv.Qty, 5) {
			t.Errorf("盘亏应生成出库 5，实际 %v / %v / %v", mv.Type, mv.Direction, mv.Qty)
		}
	})

	t.Run("账实相符不产生流水", func(t *testing.T) {
		before := fx.movementCount(t)
		doc := fx.create(t, model.StockDocCount,
			dto.StockDocumentItemInput{ProductID: fx.product.ID, Qty: qtyPtr(95)})
		posted := fx.confirm(t, doc.ID)

		if posted.Items[0].BookQty == nil || !approxQty(*posted.Items[0].BookQty, 95) {
			t.Errorf("无差异也要回写账面数: %+v", posted.Items[0])
		}
		if after := fx.movementCount(t); after != before {
			t.Errorf("账实相符却生成了 %d 条流水", after-before)
		}
		if got := fx.qty(t); !approxQty(got, 95) {
			t.Errorf("库存被无差异的盘点改动了：%v", got)
		}
	})

	t.Run("盘盈生成入库流水", func(t *testing.T) {
		doc := fx.create(t, model.StockDocCount,
			dto.StockDocumentItemInput{ProductID: fx.product.ID, Qty: qtyPtr(99)})
		fx.confirm(t, doc.ID)
		if got := fx.qty(t); !approxQty(got, 99) {
			t.Errorf("库存 = %v，期望 99", got)
		}
	})

	t.Run("盘到 0 是合法结果", func(t *testing.T) {
		doc := fx.create(t, model.StockDocCount,
			dto.StockDocumentItemInput{ProductID: fx.product.ID, Qty: qtyPtr(0)})
		fx.confirm(t, doc.ID)
		if got := fx.qty(t); !approxQty(got, 0) {
			t.Errorf("库存 = %v，期望 0", got)
		}
	})

	t.Run("账面取自确认时的快照而不是建单时", func(t *testing.T) {
		// 先把库存做成 10，再建一张"实盘 20"的盘点草稿；
		// 草稿搁着的时候另一张入库单把库存加到 30。
		fx.confirm(t, fx.create(t, model.StockDocPurchaseIn,
			dto.StockDocumentItemInput{ProductID: fx.product.ID, Qty: qtyPtr(10)}).ID)

		doc := fx.create(t, model.StockDocCount,
			dto.StockDocumentItemInput{ProductID: fx.product.ID, Qty: qtyPtr(20)})
		fx.confirm(t, fx.create(t, model.StockDocPurchaseIn,
			dto.StockDocumentItemInput{ProductID: fx.product.ID, Qty: qtyPtr(20)}).ID)

		posted := fx.confirm(t, doc.ID)
		// 确认那一刻账面是 30，实盘 20 → 盘亏 10，最终库存 20。
		if posted.Items[0].BookQty == nil || !approxQty(*posted.Items[0].BookQty, 30) {
			t.Errorf("账面数应按确认时快照算（30），实际 %+v", posted.Items[0].BookQty)
		}
		if got := fx.qty(t); !approxQty(got, 20) {
			t.Errorf("库存 = %v，期望 20", got)
		}
	})
}

func TestStockDocumentRules(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 10, 28, 10, 0, 0, 0, time.Local)
	fx := newStockDocFixture(t, &now)
	pid := fx.product.ID

	t.Run("调拨还没实现", func(t *testing.T) {
		_, err := fx.docs.Create(ctx, dto.StockDocumentInput{
			Type:  int8(model.StockDocTransfer),
			Items: []dto.StockDocumentItemInput{{ProductID: pid, Qty: qtyPtr(1)}},
		})
		if e, ok := errs.From(err); !ok || e.Code != errs.CodeInvalidParam {
			t.Fatalf("调拨应 400，实际 %v", err)
		}
		if !strings.Contains(err.Error(), "T-16b") {
			t.Errorf("提示应指向未实现的原因：%q", err.Error())
		}
	})

	t.Run("未知类型", func(t *testing.T) {
		_, err := fx.docs.Create(ctx, dto.StockDocumentInput{
			Type:  77,
			Items: []dto.StockDocumentItemInput{{ProductID: pid, Qty: qtyPtr(1)}},
		})
		if e, ok := errs.From(err); !ok || e.Code != errs.CodeInvalidParam {
			t.Errorf("未知类型应 400，实际 %v", err)
		}
	})

	t.Run("入库数量必须大于 0", func(t *testing.T) {
		if _, err := fx.docs.Create(ctx, dto.StockDocumentInput{
			Type:  int8(model.StockDocPurchaseIn),
			Items: []dto.StockDocumentItemInput{{ProductID: pid, Qty: qtyPtr(0)}},
		}); !isBadRequest(err) {
			t.Errorf("数量 0 应 400，实际 %v", err)
		}
	})

	t.Run("数量不能是负数", func(t *testing.T) {
		if _, err := fx.docs.Create(ctx, dto.StockDocumentInput{
			Type:  int8(model.StockDocCount),
			Items: []dto.StockDocumentItemInput{{ProductID: pid, Qty: qtyPtr(-1)}},
		}); !isBadRequest(err) {
			t.Errorf("负数应 400，实际 %v", err)
		}
	})

	t.Run("产品不存在", func(t *testing.T) {
		_, err := fx.docs.Create(ctx, dto.StockDocumentInput{
			Type:  int8(model.StockDocPurchaseIn),
			Items: []dto.StockDocumentItemInput{{ProductID: 9999, Qty: qtyPtr(1)}},
		})
		if e, ok := errs.From(err); !ok || e.Code != errs.CodeNotFound {
			t.Errorf("产品不存在应 404，实际 %v", err)
		}
	})

	t.Run("类型不可修改", func(t *testing.T) {
		doc := fx.create(t, model.StockDocPurchaseIn,
			dto.StockDocumentItemInput{ProductID: pid, Qty: qtyPtr(5)})
		_, err := fx.docs.Update(ctx, doc.ID, dto.StockDocumentInput{
			Type:  int8(model.StockDocCount),
			Items: []dto.StockDocumentItemInput{{ProductID: pid, Qty: qtyPtr(5)}},
		})
		if !isBadRequest(err) {
			t.Errorf("改类型应 400，实际 %v", err)
		}
	})

	t.Run("草稿可改可删，明细整组替换", func(t *testing.T) {
		doc := fx.create(t, model.StockDocPurchaseIn,
			dto.StockDocumentItemInput{ProductID: pid, Qty: qtyPtr(5)},
			dto.StockDocumentItemInput{ProductID: pid, Qty: qtyPtr(6)})
		if len(doc.Items) != 2 {
			t.Fatalf("明细行数 = %d，期望 2", len(doc.Items))
		}

		updated, err := fx.docs.Update(ctx, doc.ID, dto.StockDocumentInput{
			Type:  int8(model.StockDocPurchaseIn),
			Items: []dto.StockDocumentItemInput{{ProductID: pid, Qty: qtyPtr(11)}},
		})
		if err != nil {
			t.Fatalf("改草稿失败: %v", err)
		}
		if len(updated.Items) != 1 || !approxQty(updated.Items[0].Qty, 11) {
			t.Errorf("明细替换后 = %+v", updated.Items)
		}

		if err := fx.docs.Delete(ctx, doc.ID); err != nil {
			t.Fatalf("删草稿失败: %v", err)
		}
		var n int64
		if err := fx.db.Model(&model.StockDocumentItem{}).
			Where("document_id = ?", doc.ID).Count(&n).Error; err != nil {
			t.Fatalf("统计明细失败: %v", err)
		}
		if n != 0 {
			t.Errorf("删单后留下 %d 条孤儿明细", n)
		}
	})
}

func isConflict(err error) bool {
	e, ok := errs.From(err)
	return ok && e.Code == errs.CodeConflict
}

func isBadRequest(err error) bool {
	e, ok := errs.From(err)
	return ok && e.Code == errs.CodeInvalidParam
}
