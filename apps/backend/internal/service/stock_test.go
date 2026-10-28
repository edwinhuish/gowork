package service

import (
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/errs"
	"github.com/edwinhuish/gowork/internal/repository"
)

type stockFixture struct {
	stock    StockService
	db       *gorm.DB
	productA *model.Product
	productB *model.Product
}

func newStockFixture(t *testing.T, now *time.Time) stockFixture {
	t.Helper()
	ctx := context.Background()

	db := openTestDB(t,
		&model.Product{}, &model.DocSeq{}, &model.StockMovement{}, &model.StockSnapshot{})

	clock := func() time.Time { return *now }
	productRepo := repository.NewProductRepository(db)
	stockRepo := repository.NewStockRepository(db)
	docNoSvc := NewDocNoService(repository.NewDocSeqRepository(db), WithDocNoClock(clock))

	pA := &model.Product{SKUCode: "BH01-0001", NameCN: "不锈钢合页", Spec: "100×75×3",
		Unit: "PCS", Status: model.EnableStatusOn}
	pB := &model.Product{SKUCode: "FS01-0001", NameCN: "自攻螺丝", Unit: "BOX",
		Status: model.EnableStatusOn}
	for _, p := range []*model.Product{pA, pB} {
		if err := productRepo.Create(ctx, p); err != nil {
			t.Fatalf("建产品失败: %v", err)
		}
	}

	return stockFixture{
		stock: NewStockService(stockRepo, productRepo, docNoSvc,
			WithStockClock(clock),
			WithStockLogger(slog.New(slog.DiscardHandler))),
		db:       db,
		productA: pA,
		productB: pB,
	}
}

// move 录一条流水并断言成功。
func (f stockFixture) move(
	t *testing.T, productID uint, typ model.StockMovementType,
	dir model.StockDirection, qty float64, occurred string,
) *model.StockMovement {
	t.Helper()
	m, err := f.stock.CreateMovement(context.Background(), dto.StockMovementInput{
		ProductID:  productID,
		Type:       int8(typ),
		Direction:  int8(dir),
		Qty:        qty,
		OccurredAt: occurred,
	})
	if err != nil {
		t.Fatalf("录库存流水失败: %v", err)
	}
	return m
}

// snapshotQty 直接读库里的快照值，绕开 service 的舍入，用来看增量维护得对不对。
func (f stockFixture) snapshotQty(t *testing.T, productID uint) float64 {
	t.Helper()
	var s model.StockSnapshot
	if err := f.db.Where("product_id = ?", productID).First(&s).Error; err != nil {
		t.Fatalf("查快照失败: %v", err)
	}
	return s.Qty
}

func TestStockMovementSnapshot(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 10, 28, 10, 0, 0, 0, time.Local)
	fx := newStockFixture(t, &now)
	p := fx.productA

	t.Run("期初入库建立快照", func(t *testing.T) {
		m := fx.move(t, p.ID, model.StockMovementOpening, model.StockDirectionIn, 100, "")
		if m.DocNo != "MV-202610-0001" {
			t.Errorf("流水号 = %q，期望 MV-202610-0001", m.DocNo)
		}
		// 流水带产品快照：产品改名后回看这条流水仍应是当时的说法。
		if m.SKUCode != p.SKUCode || m.NameCN != p.NameCN || m.Unit != p.Unit {
			t.Errorf("流水缺产品快照: %+v", m)
		}
		if !m.OccurredAt.Equal(model.DateOnly(now)) {
			t.Errorf("发生日 = %v，期望取今天", m.OccurredAt)
		}
		if got := fx.snapshotQty(t, p.ID); !approx(got, 100) {
			t.Errorf("快照 = %v，期望 100", got)
		}
	})

	t.Run("出库把库存减回去", func(t *testing.T) {
		fx.move(t, p.ID, model.StockMovementCount, model.StockDirectionOut, 30, "")
		if got := fx.snapshotQty(t, p.ID); !approx(got, 70) {
			t.Errorf("快照 = %v，期望 70", got)
		}
	})

	t.Run("多次入库的尾差不留在快照里", func(t *testing.T) {
		// 0.1 + 0.2 在浮点里是 0.30000000000000004，快照必须仍然干净。
		fx.move(t, p.ID, model.StockMovementOther, model.StockDirectionIn, 0.1, "")
		fx.move(t, p.ID, model.StockMovementOther, model.StockDirectionIn, 0.2, "")
		if got := fx.snapshotQty(t, p.ID); !approx(got, 70.3) {
			t.Errorf("快照 = %v，期望 70.3", got)
		}
	})

	t.Run("删除流水把数量回滚", func(t *testing.T) {
		m := fx.move(t, p.ID, model.StockMovementOther, model.StockDirectionIn, 10, "")
		if got := fx.snapshotQty(t, p.ID); !approx(got, 80.3) {
			t.Fatalf("快照 = %v，期望 80.3", got)
		}
		if err := fx.stock.DeleteMovement(ctx, m.ID); err != nil {
			t.Fatalf("删除手工流水应允许，实际 %v", err)
		}
		if got := fx.snapshotQty(t, p.ID); !approx(got, 70.3) {
			t.Errorf("删除后快照 = %v，期望 70.3", got)
		}
	})

	t.Run("每个产品各有一份快照", func(t *testing.T) {
		fx.move(t, fx.productB.ID, model.StockMovementOpening, model.StockDirectionIn, 8, "")
		if got := fx.snapshotQty(t, fx.productB.ID); !approx(got, 8) {
			t.Errorf("B 的快照 = %v，期望 8", got)
		}
		// 写 B 不该影响 A：upsert 的冲突目标必须是 product_id。
		if got := fx.snapshotQty(t, p.ID); !approx(got, 70.3) {
			t.Errorf("A 的快照被 B 的写入带偏 = %v，期望 70.3", got)
		}
	})

	t.Run("重算结果与增量一致", func(t *testing.T) {
		before := fx.snapshotQty(t, p.ID)
		res, err := fx.stock.Recalc(ctx, nil)
		if err != nil {
			t.Fatalf("重算失败: %v", err)
		}
		if res.Recalculated != 2 {
			t.Errorf("重算了 %d 个产品，期望 2（A 与 B）", res.Recalculated)
		}
		if len(res.Mismatched) != 0 {
			t.Errorf("增量维护的快照与重算结果不该有差异，实际 %+v", res.Mismatched)
		}
		if after := fx.snapshotQty(t, p.ID); !approx(after, before) {
			t.Errorf("重算改动了本来就对的快照：%v → %v", before, after)
		}
	})

	t.Run("绕开流水改快照会被重算抓出来并修正", func(t *testing.T) {
		if err := fx.db.Model(&model.StockSnapshot{}).
			Where("product_id = ?", p.ID).Update("qty", 999).Error; err != nil {
			t.Fatalf("模拟直接改快照失败: %v", err)
		}
		res, err := fx.stock.Recalc(ctx, &p.ID)
		if err != nil {
			t.Fatalf("重算失败: %v", err)
		}
		if len(res.Mismatched) != 1 || !approx(res.Mismatched[0].SnapshotQty, 999) {
			t.Fatalf("应报告一条差异（999 → 流水值），实际 %+v", res.Mismatched)
		}
		if got := fx.snapshotQty(t, p.ID); !approx(got, res.Mismatched[0].MovementQty) {
			t.Errorf("重算后快照 = %v，期望 %v", got, res.Mismatched[0].MovementQty)
		}
	})
}

func TestStockMovementRules(t *testing.T) {
	now := time.Date(2026, 10, 28, 10, 0, 0, 0, time.Local)
	fx := newStockFixture(t, &now)

	cases := []struct {
		name       string
		in         dto.StockMovementInput
		wantCode   errs.Code
		wantSubstr string
	}{
		{
			name: "采购入库不能手工录",
			in: dto.StockMovementInput{ProductID: fx.productA.ID,
				Type: int8(model.StockMovementPurchase), Direction: 1, Qty: 10},
			wantCode:   errs.CodeInvalidParam,
			wantSubstr: "必须由单据产生",
		},
		{
			name: "销售出库不能手工录",
			in: dto.StockMovementInput{ProductID: fx.productA.ID,
				Type: int8(model.StockMovementSales), Direction: -1, Qty: 10},
			wantCode:   errs.CodeInvalidParam,
			wantSubstr: "必须由单据产生",
		},
		{
			name: "未知类型",
			in: dto.StockMovementInput{ProductID: fx.productA.ID,
				Type: 77, Direction: 1, Qty: 10},
			wantCode:   errs.CodeInvalidParam,
			wantSubstr: "未知的库存流水类型",
		},
		{
			name: "方向只能是 ±1",
			in: dto.StockMovementInput{ProductID: fx.productA.ID,
				Type: int8(model.StockMovementOther), Direction: 2, Qty: 10},
			wantCode:   errs.CodeInvalidParam,
			wantSubstr: "出入方向",
		},
		{
			name: "数量必须大于 0",
			in: dto.StockMovementInput{ProductID: fx.productA.ID,
				Type: int8(model.StockMovementOther), Direction: 1, Qty: 0},
			wantCode:   errs.CodeInvalidParam,
			wantSubstr: "数量必须大于 0",
		},
		{
			name: "小于千分之一的数量被舍入为 0 后拒绝",
			in: dto.StockMovementInput{ProductID: fx.productA.ID,
				Type: int8(model.StockMovementOther), Direction: 1, Qty: 0.0001},
			wantCode:   errs.CodeInvalidParam,
			wantSubstr: "数量必须大于 0",
		},
		{
			name: "产品不存在",
			in: dto.StockMovementInput{ProductID: 9999,
				Type: int8(model.StockMovementOther), Direction: 1, Qty: 1},
			wantCode: errs.CodeNotFound,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := fx.stock.CreateMovement(context.Background(), tc.in)
			if err == nil {
				t.Fatal("应被拒绝，实际成功")
			}
			e, ok := errs.From(err)
			if !ok || e.Code != tc.wantCode {
				t.Fatalf("错误码 = %v，期望 %d（err=%v）", e, tc.wantCode, err)
			}
			if tc.wantSubstr != "" && !strings.Contains(err.Error(), tc.wantSubstr) {
				t.Errorf("提示 %q 里没有 %q", err.Error(), tc.wantSubstr)
			}
		})
	}

	// 被拒的请求不该留下流水，也不该建出快照。
	var count int64
	if err := fx.db.Model(&model.StockMovement{}).Count(&count).Error; err != nil {
		t.Fatalf("统计流水失败: %v", err)
	}
	if count != 0 {
		t.Errorf("被拒的录入留下了 %d 条流水", count)
	}
}

func TestStockMovementQuery(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 10, 28, 10, 0, 0, 0, time.Local)
	fx := newStockFixture(t, &now)

	fx.move(t, fx.productA.ID, model.StockMovementOpening, model.StockDirectionIn, 100, "2026-10-01")
	fx.move(t, fx.productA.ID, model.StockMovementOther, model.StockDirectionOut, 20, "2026-10-20")
	fx.move(t, fx.productB.ID, model.StockMovementCount, model.StockDirectionIn, 5, "2026-10-25")

	t.Run("按产品过滤且按发生日倒序", func(t *testing.T) {
		id := fx.productA.ID
		res, err := fx.stock.ListMovements(ctx, dto.StockMovementQuery{ProductID: &id})
		if err != nil {
			t.Fatalf("查流水失败: %v", err)
		}
		if res.Total != 2 {
			t.Fatalf("A 的流水 %d 条，期望 2", res.Total)
		}
		if !res.List[0].OccurredAt.Equal(time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)) {
			t.Errorf("首条发生日 = %v，期望 10-20", res.List[0].OccurredAt)
		}
	})

	t.Run("按类型过滤", func(t *testing.T) {
		typ := int8(model.StockMovementCount)
		res, err := fx.stock.ListMovements(ctx, dto.StockMovementQuery{Type: &typ})
		if err != nil {
			t.Fatalf("查流水失败: %v", err)
		}
		if res.Total != 1 || res.List[0].ProductID != fx.productB.ID {
			t.Errorf("盘点流水 = %+v", res.List)
		}
	})

	t.Run("按日期区间过滤", func(t *testing.T) {
		res, err := fx.stock.ListMovements(ctx, dto.StockMovementQuery{
			From: "2026-10-10", To: "2026-10-31",
		})
		if err != nil {
			t.Fatalf("查流水失败: %v", err)
		}
		if res.Total != 2 {
			t.Errorf("区间内 %d 条，期望 2", res.Total)
		}
	})

	t.Run("日期格式错误", func(t *testing.T) {
		if _, err := fx.stock.ListMovements(ctx, dto.StockMovementQuery{From: "10/10/2026"}); err == nil {
			t.Fatal("非法日期应报 400")
		}
	})

	t.Run("快照列表带出产品信息", func(t *testing.T) {
		res, err := fx.stock.ListSnapshots(ctx, dto.StockSnapshotQuery{})
		if err != nil {
			t.Fatalf("查库存失败: %v", err)
		}
		if res.Total != 2 {
			t.Fatalf("库存行数 = %d，期望 2", res.Total)
		}
		// 按 SKU 升序：BH01-0001 在 FS01-0001 之前。
		first := res.List[0]
		if first.SKUCode != "BH01-0001" || first.NameCN != "不锈钢合页" || first.Unit != "PCS" {
			t.Errorf("首行产品信息 = %+v", first)
		}
		if !approx(first.Qty, 80) {
			t.Errorf("A 的库存 = %v，期望 80（100 − 20）", first.Qty)
		}
		if first.LastOccurredAt == nil {
			t.Error("库存行应带最后一次变动日期")
		}
	})

	t.Run("快照列表可按 SKU 搜", func(t *testing.T) {
		res, err := fx.stock.ListSnapshots(ctx, dto.StockSnapshotQuery{
			PageQuery: dto.PageQuery{Keyword: "FS01"},
		})
		if err != nil {
			t.Fatalf("查库存失败: %v", err)
		}
		if res.Total != 1 || res.List[0].SKUCode != "FS01-0001" {
			t.Errorf("按 SKU 搜到 = %+v", res.List)
		}
	})
}
