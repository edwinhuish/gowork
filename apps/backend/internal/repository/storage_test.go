package repository

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/edwinhuish/gowork/internal/config"
	"github.com/edwinhuish/gowork/internal/infra/database"
	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/errs"
	"github.com/edwinhuish/gowork/internal/pkg/money"
)

// storageTarget 描述一个待验证的数据库实例。
type storageTarget struct {
	name   string
	driver string
	dsn    string
}

// TestStorage 把同一套仓储操作跑在三种数据库上。
//
// sqlite 用临时文件（始终执行）；postgres / mysql 读取环境变量提供的 DSN，
// 连不上就跳过——`task test:storage` 会把三个 DSN 一起传进来。
func TestStorage(t *testing.T) {
	targets := []storageTarget{
		{"sqlite", config.DriverSQLite, filepath.Join(t.TempDir(), "storage.db")},
	}
	if dsn := os.Getenv("GOWORK_TEST_PG_DSN"); dsn != "" {
		targets = append(targets, storageTarget{"postgres", config.DriverPostgres, dsn})
	}
	if dsn := os.Getenv("GOWORK_TEST_MYSQL_DSN"); dsn != "" {
		targets = append(targets, storageTarget{"mysql", config.DriverMySQL, dsn})
	}

	for _, tt := range targets {
		t.Run(tt.name, func(t *testing.T) {
			runStorageSuite(t, tt.driver, tt.dsn)
		})
	}
}

func runStorageSuite(t *testing.T, driver, dsn string) {
	ctx := context.Background()

	db, err := database.Open(config.Database{
		Driver:      driver,
		DSN:         dsn,
		MaxOpen:     5,
		MaxIdle:     2,
		MaxLifetime: 60,
		LogLevel:    "error",
	}, slog.New(slog.DiscardHandler))
	if err != nil {
		if driver == config.DriverSQLite {
			t.Fatalf("连接 %s 失败: %v", driver, err)
		}
		t.Skipf("%s 不可用，跳过: %v", driver, err)
	}
	t.Cleanup(func() { _ = database.Close(db) })

	if got := database.Dialect(db); got != driver {
		t.Fatalf("方言应为 %s，实际 %s", driver, got)
	}

	// 产品编号与货源必须一起迁移：产品列表的关键词条件会分别子查询
	// md_product_code 与 md_product_source，少了任一张表，三库里
	// 只有"搜关键词"那几条会失败，而其他都正常。
	// 主体列表还会 LEFT JOIN sys_user 取负责人登录名，所以账号表也要在。
	if err := database.AutoMigrate(db,
		&model.Partner{}, &model.Product{}, &model.ProductCode{},
		&model.ProductSource{}, &model.User{}); err != nil {
		t.Fatalf("迁移失败: %v", err)
	}

	repo := NewProductRepository(db)
	suffix := time.Now().UnixNano()
	mark := fmt.Sprintf("T%d", suffix)

	// 每次跑用独立主体圈定数据集，避免和库里已有的演示数据混在一起。
	partnerRepo := NewPartnerRepository(db)
	supplier := &model.Partner{
		Code: "SUP-" + mark, Name: "测试供应商",
		Role: model.PartnerRoleSupplier, Status: model.EnableStatusOn,
	}
	if err := partnerRepo.Create(ctx, supplier); err != nil {
		t.Fatalf("创建供应商: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Unscoped().Where("supplier_id = ?", supplier.ID).Delete(&model.Product{}).Error
		_ = db.Unscoped().Where("id = ?", supplier.ID).Delete(&model.Partner{}).Error
	})

	products := []*model.Product{
		{
			SKUCode:    mark + "-AbC",
			NameCN:     "不锈钢螺丝 M8",
			NameEN:     "Stainless Screw",
			Category:   "FS",
			Unit:       "PCS",
			SupplierID: &supplier.ID,
			Status:     model.EnableStatusOn,
		},
		{
			SKUCode:    mark + "-002",
			NameCN:     "六角螺栓 M10",
			Category:   "FS",
			Unit:       "PCS",
			SupplierID: &supplier.ID,
			Status:     model.EnableStatusOn,
		},
	}
	for _, p := range products {
		if err := repo.Create(ctx, p); err != nil {
			t.Fatalf("创建产品 %s: %v", p.SKUCode, err)
		}
	}

	t.Run("按 ID 查询带出关联", func(t *testing.T) {
		got, err := repo.FindByID(ctx, products[0].ID)
		if err != nil {
			t.Fatalf("FindByID: %v", err)
		}
		if got.Supplier == nil || got.Supplier.ID != supplier.ID {
			t.Fatalf("供应商未预加载: %+v", got.Supplier)
		}
	})

	t.Run("唯一编码校验", func(t *testing.T) {
		exists, err := repo.ExistsBySKUCode(ctx, products[0].SKUCode, 0)
		if err != nil || !exists {
			t.Fatalf("应判定为已存在: %v, %v", exists, err)
		}
		if exists, err := repo.ExistsBySKUCode(ctx, products[0].SKUCode, products[0].ID); err != nil || exists {
			t.Fatalf("排除自身后不应命中: %v, %v", exists, err)
		}
	})

	t.Run("角色过滤按包含语义", func(t *testing.T) {
		// 客供两用是合并的核心场景：同一个主体既能在采购侧被选到，
		// 又能在销售侧被选到，而且是同一条记录、同一套联系方式。
		both := &model.Partner{
			Code: "BOTH-" + mark, Name: "客供两用主体",
			Role:   model.PartnerRoleCustomer | model.PartnerRoleSupplier,
			Status: model.EnableStatusOn,
		}
		if err := partnerRepo.Create(ctx, both); err != nil {
			t.Fatalf("创建客供两用主体: %v", err)
		}
		t.Cleanup(func() {
			_ = db.Unscoped().Where("id = ?", both.ID).Delete(&model.Partner{}).Error
		})

		supplierRole := model.PartnerRoleSupplier
		if _, total, err := partnerRepo.List(ctx, PartnerFilter{Role: &supplierRole, Keyword: mark},
			Pagination{Page: 1, PageSize: 20}); err != nil {
			t.Fatalf("List: %v", err)
		} else if total != 2 {
			t.Fatalf("筛供应商应同时包含纯供应商与客供两用主体，得到 %d 条", total)
		}

		customerRole := model.PartnerRoleCustomer
		list, total, err := partnerRepo.List(ctx, PartnerFilter{Role: &customerRole, Keyword: mark},
			Pagination{Page: 1, PageSize: 20})
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if total != 1 || len(list) != 1 || list[0].ID != both.ID {
			t.Fatalf("筛客户应只命中客供两用那一条，得到 %d 条", total)
		}

		// 概览计数与列表必须口径一致：按 role = ? 统计会漏掉 role=3 的主体。
		if n, err := partnerRepo.CountByRole(ctx, model.PartnerRoleSupplier); err != nil || n < 2 {
			t.Fatalf("供应商计数应至少包含纯供应商与两用主体: n=%d err=%v", n, err)
		}
		if n, err := partnerRepo.CountByRole(ctx, model.PartnerRoleCustomer); err != nil || n < 1 {
			t.Fatalf("客户计数应至少包含两用主体: n=%d err=%v", n, err)
		}
	})

	t.Run("关键字大小写不敏感", func(t *testing.T) {
		list, total, err := repo.List(ctx, ProductFilter{
			Keyword:    strings.ToLower(mark) + "-abc",
			SupplierID: &supplier.ID,
		}, Pagination{Page: 1, PageSize: 10})
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if total != 1 || len(list) != 1 || list[0].ID != products[0].ID {
			t.Fatalf("大小写不敏感搜索失败: total=%d len=%d", total, len(list))
		}
	})

	t.Run("中文关键字可搜索", func(t *testing.T) {
		_, total, err := repo.List(ctx, ProductFilter{
			Keyword:    "螺丝",
			SupplierID: &supplier.ID,
		}, Pagination{Page: 1, PageSize: 10})
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if total != 1 {
			t.Fatalf("按中文名搜索应命中 1 条，得到 %d", total)
		}
	})

	t.Run("通配符被转义", func(t *testing.T) {
		// 若不转义，"T123%" 会命中全部三条；转义后按字面量匹配，应为 0。
		_, total, err := repo.List(ctx, ProductFilter{
			Keyword:    mark + "%",
			SupplierID: &supplier.ID,
		}, Pagination{Page: 1, PageSize: 10})
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if total != 0 {
			t.Fatalf("%% 应被当成字面量，得到 %d 条", total)
		}
	})

	t.Run("分页与排序", func(t *testing.T) {
		page1, total, err := repo.List(ctx, ProductFilter{SupplierID: &supplier.ID}, Pagination{Page: 1, PageSize: 1})
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if total != 2 || len(page1) != 1 {
			t.Fatalf("分页结果不对: total=%d len=%d", total, len(page1))
		}
		page2, _, err := repo.List(ctx, ProductFilter{SupplierID: &supplier.ID}, Pagination{Page: 2, PageSize: 1})
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(page2) != 1 || page2[0].ID == page1[0].ID {
			t.Fatalf("第二页应是另一条记录")
		}
	})

	t.Run("按字段更新不清空未提交字段", func(t *testing.T) {
		if err := repo.Update(ctx, products[1].ID, map[string]any{"name_cn": "六角螺栓 M12"}); err != nil {
			t.Fatalf("Update: %v", err)
		}
		got, err := repo.FindByID(ctx, products[1].ID)
		if err != nil {
			t.Fatalf("FindByID: %v", err)
		}
		if got.NameCN != "六角螺栓 M12" || got.SKUCode != products[1].SKUCode {
			t.Fatalf("字段更新异常: %+v", got)
		}
	})

	t.Run("软删除后查不到但数据仍在", func(t *testing.T) {
		if err := repo.Delete(ctx, products[1].ID); err != nil {
			t.Fatalf("Delete: %v", err)
		}
		if _, err := repo.FindByID(ctx, products[1].ID); !errors.Is(err, errs.ErrNotFound) {
			t.Fatalf("软删除后应返回 ErrNotFound，得到 %v", err)
		}
		var raw int64
		if err := db.Model(&model.Product{}).Unscoped().Where("id = ?", products[1].ID).Count(&raw).Error; err != nil {
			t.Fatalf("统计: %v", err)
		}
		if raw != 1 {
			t.Fatalf("软删除不应物理删除，记录数 %d", raw)
		}
		_, total, err := repo.List(ctx, ProductFilter{SupplierID: &supplier.ID}, Pagination{Page: 1, PageSize: 10})
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if total != 1 {
			t.Fatalf("列表应只剩 1 条，得到 %d", total)
		}
	})

	t.Run("软删后同编码可重建", func(t *testing.T) {
		// 未删除的行 deleted_at 恒为 0，唯一索引是 (sku_code, deleted_at)，
		// 表达的就是「未删才唯一」：上一条子测试删掉的 SKU 现在可以再用。
		// 单列唯一索引做不到这一点，所以三库里都必须验一遍。
		reborn := &model.Product{
			SKUCode: products[1].SKUCode, NameCN: "重建的六角螺栓",
			Unit: "PCS", Status: model.EnableStatusOn, SupplierID: &supplier.ID,
		}
		if err := repo.Create(ctx, reborn); err != nil {
			t.Fatalf("软删后重建同 SKU 失败: %v", err)
		}
		if reborn.ID == products[1].ID {
			t.Fatal("重建应是一条新记录")
		}

		var raw int64
		if err := db.Unscoped().Model(&model.Product{}).
			Where("sku_code = ?", products[1].SKUCode).Count(&raw).Error; err != nil {
			t.Fatalf("统计: %v", err)
		}
		if raw != 2 {
			t.Fatalf("同 SKU 应有 2 行（一删一未删），得到 %d", raw)
		}
		t.Cleanup(func() {
			_ = db.Unscoped().Where("id = ?", reborn.ID).Delete(&model.Product{}).Error
		})
	})

	t.Run("定点金额跨库逐位往返", func(t *testing.T) {
		// 三种库对 DECIMAL 的落地方式不同（sqlite 实际存 REAL），
		// 所以定点改造必须在三库上都验一遍：写进去什么样，读出来就什么样，
		// 求和之后也不得多出末位。
		if err := database.AutoMigrate(db,
			&model.Partner{}, &model.Receivable{}, &model.CreditNote{}, &model.DebitNote{}); err != nil {
			t.Fatalf("迁移应收表失败: %v", err)
		}
		receivables := NewReceivableRepository(db)
		customer := &model.Partner{
			Code: "CUS-" + mark, Name: "测试客户",
			Role: model.PartnerRoleCustomer, Status: model.EnableStatusOn,
		}
		if err := partnerRepo.Create(ctx, customer); err != nil {
			t.Fatalf("创建客户: %v", err)
		}
		t.Cleanup(func() {
			_ = db.Unscoped().Where("partner_id = ?", customer.ID).Delete(&model.Receivable{}, &model.CreditNote{}, &model.DebitNote{}).Error
			_ = db.Unscoped().Where("id = ?", customer.ID).Delete(&model.Partner{}).Error
		})

		// 0.1 + 0.2 是浮点的经典反例：定点之后 SUM 必须正好是 0.3。
		// SourceID 用本次新建客户的主键，圈住数据集，SUM 只可能来自这两行。
		for i, amount := range []string{"0.1", "0.2"} {
			row := &model.Receivable{Settlement: model.Settlement{
				DocNo:        fmt.Sprintf("RC-%s-%d", mark, i),
				PartnerID:    customer.ID,
				SourceType:   model.SettlementSourceSalesOrder,
				SourceID:     customer.ID,
				CurrencyCode: "CNY",
				ExchangeRate: money.FromInt(1),
				Amount:       money.MustParse(amount),
				AmountBase:   money.MustParse(amount),
				DueDate:      time.Now(),
				Status:       model.SettlementStatusOpen,
			}}
			if err := receivables.Create(ctx, row); err != nil {
				t.Fatalf("创建应收 %s: %v", amount, err)
			}
		}

		got, err := receivables.SumAmountBySource(
			ctx, model.SettlementSourceSalesOrder, customer.ID, 0)
		if err != nil {
			t.Fatalf("汇总应收: %v", err)
		}
		if got.Cmp(money.MustParse("0.3")) != 0 {
			t.Fatalf("应收合计应为 0.1 + 0.2 = 0.3，得到 %s", got)
		}

		var stored model.Receivable
		if err := db.Where("doc_no = ?", fmt.Sprintf("RC-%s-0", mark)).First(&stored).Error; err != nil {
			t.Fatalf("读回应收: %v", err)
		}
		if stored.Amount.Cmp(money.MustParse("0.1")) != 0 {
			t.Fatalf("读回金额应为 0.1，得到 %s", stored.Amount)
		}
		if stored.Outstanding().Cmp(money.MustParse("0.1")) != 0 {
			t.Fatalf("未核销金额应为 0.1，得到 %s", stored.Outstanding())
		}
	})

	t.Run("库存仓库维度迁移回填", func(t *testing.T) {
		// 这条迁移原先只在 sqlite 上跑过，而它里面有三种引擎各写各的写法：
		// 分批回填的 `UPDATE ... WHERE id IN (...)`、游标分页、以及改唯一索引的 DDL。
		// 「旧三列索引 → 新四列索引」那段只在 sqlite 上装回旧索引来验：
		// 共享测试库里已经有数据，三列唯一索引未必建得起来，而那不是这条要验的东西。
		if err := database.AutoMigrate(db,
			&model.Warehouse{}, &model.StockMovement{}, &model.StockSnapshot{}); err != nil {
			t.Fatalf("迁移库存表失败: %v", err)
		}

		wh := &model.Warehouse{
			Code: "WH-" + mark, Name: "测试默认仓",
			IsDefault: true, Status: model.EnableStatusOn,
		}
		if err := db.Create(wh).Error; err != nil {
			t.Fatalf("建默认仓: %v", err)
		}
		other := &model.Product{
			SKUCode: mark + "-WH", NameCN: "迁移测试件",
			Unit: "PCS", Status: model.EnableStatusOn, SupplierID: &supplier.ID,
		}
		if err := repo.Create(ctx, other); err != nil {
			t.Fatalf("创建产品: %v", err)
		}
		productIDs := []uint{products[0].ID, other.ID}
		t.Cleanup(func() {
			_ = db.Unscoped().Where("product_id IN ?", productIDs).
				Delete(&model.StockMovement{}, &model.StockSnapshot{}).Error
			_ = db.Unscoped().Where("id = ?", wh.ID).Delete(&model.Warehouse{}).Error
		})

		// 迁移按「is_default 且启用、id 最小」挑仓库：共享库里可能已经有一条，
		// 所以断言的对象是它挑中的那条，而不是本测试新建的这条。
		var defaultIDs []uint
		if err := db.Model(&model.Warehouse{}).
			Where("is_default = ? AND status = ?", true, model.EnableStatusOn).
			Order("id ASC").Limit(1).Pluck("id", &defaultIDs).Error; err != nil {
			t.Fatalf("查默认仓: %v", err)
		}
		if len(defaultIDs) == 0 {
			t.Fatal("库里没有默认仓")
		}
		defaultID := defaultIDs[0]

		moves := make([]model.StockMovement, 0, 5)
		for i := 0; i < 5; i++ {
			moves = append(moves, model.StockMovement{
				DocNo:      fmt.Sprintf("MV-%s-%d", mark, i),
				ProductID:  products[0].ID,
				SKUCode:    products[0].SKUCode,
				Type:       model.StockMovementPurchase,
				Direction:  model.StockDirectionIn,
				Qty:        money.FromInt(1),
				OccurredAt: model.DateOnly(time.Now()),
			})
		}
		if err := db.Create(&moves).Error; err != nil {
			t.Fatalf("建存量流水: %v", err)
		}
		snaps := []model.StockSnapshot{
			{ProductID: productIDs[0], Qty: money.FromInt(1)},
			{ProductID: productIDs[1], Qty: money.FromInt(2)},
		}
		if err := db.Create(&snaps).Error; err != nil {
			t.Fatalf("建存量快照: %v", err)
		}

		log, _ := captureLog()
		if err := MigrateStockWarehouse(ctx, db, log); err != nil {
			t.Fatalf("迁移失败: %v", err)
		}

		var left int64
		if err := db.Model(&model.StockMovement{}).
			Where("warehouse_id = 0 AND doc_no LIKE ?", "MV-"+mark+"%").Count(&left).Error; err != nil {
			t.Fatalf("统计未回填流水: %v", err)
		}
		if left != 0 {
			t.Fatalf("还有 %d 条流水没回填", left)
		}
		for _, s := range snaps {
			var got model.StockSnapshot
			if err := db.First(&got, s.ID).Error; err != nil {
				t.Fatalf("读回快照: %v", err)
			}
			if got.WarehouseID != defaultID {
				t.Fatalf("快照应回填到默认仓 %d，实际 %d", defaultID, got.WarehouseID)
			}
		}

		// 索引必须是四列：写入侧的 ON CONFLICT 靠它，而这个索引在三种库上
		// 由 gorm 的 migrator 生成，列没对上就得当场发现。
		indexes, err := db.Migrator().GetIndexes(&model.StockSnapshot{})
		if err != nil {
			t.Fatalf("读索引: %v", err)
		}
		hasWarehouse := false
		for _, ix := range indexes {
			if ix.Name() != "uni_stock_snapshot_product" {
				continue
			}
			for _, col := range ix.Columns() {
				if col == "warehouse_id" {
					hasWarehouse = true
				}
			}
		}
		if !hasWarehouse {
			t.Fatalf("快照唯一索引不含 warehouse_id：%+v", indexes)
		}

		// 幂等：仓库已经回落成默认仓，再跑一次不该报错，也不该改数。
		if err := MigrateStockWarehouse(ctx, db, log); err != nil {
			t.Fatalf("二次迁移失败（应幂等）: %v", err)
		}
		for _, s := range snaps {
			var got model.StockSnapshot
			if err := db.First(&got, s.ID).Error; err != nil {
				t.Fatalf("二次迁移后读回快照: %v", err)
			}
			if got.WarehouseID != defaultID {
				t.Fatalf("二次迁移不该改仓库，实际 %d", got.WarehouseID)
			}
		}
	})
}
