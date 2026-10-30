package bootstrap_test

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/edwinhuish/gowork/internal/bootstrap"
	"github.com/edwinhuish/gowork/internal/config"
	"github.com/edwinhuish/gowork/internal/infra/database"
	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/money"
)

// TestStartupMigrations 在真实数据库上走一遍启动期迁移链。
//
// 只调 bootstrap.New：它做完装配就返回，不监听端口——不需要起进程，也就不需要杀进程。
// 验的是**顺序**：EnsureDefault 先建出默认仓，MigrateStockWarehouse 再拿它的 id 回填；
// 这两步反了，迁移函数查不到默认仓就直接返回，一行都不回填，而启动照常成功
// （错误只进日志）。那种坏法在任何单测里都看不见，只有整条启动链能撞出来。
//
// 顺序这一条只在**还没有默认仓**的库上具决定性：库里早就有一条默认仓时，
// 即便两步调换，迁移也能找到仓、也能把行回填掉。所以这条测试的强度随库的状态变，
// 真正稳定那部分是「每次启动都真的会去回填一行埋进去的存量数据」。
//
// sqlite 侧这条链由 e2e 那组覆盖；这里补的是另外两种引擎，DSN 由 task test:storage 传。
func TestStartupMigrations(t *testing.T) {
	cases := []struct {
		name   string
		driver string
		dsnEnv string
	}{
		{"postgres", config.DriverPostgres, "GOWORK_TEST_PG_DSN"},
		{"mysql", config.DriverMySQL, "GOWORK_TEST_MYSQL_DSN"},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			dsn := os.Getenv(c.dsnEnv)
			if dsn == "" {
				t.Skipf("未设置 %s", c.dsnEnv)
			}

			// 走真实配置路径（写临时 YAML → Load）而不是拼 Config 字面量：
			// 字面量会绕过 applyDefaults，auto_migrate 于是是零值 false，表都不会建。
			dir := t.TempDir()
			cfgPath := filepath.Join(dir, "config.yaml")
			raw := fmt.Sprintf(`
app:
  name: gowork-startup-migrate
  env: test
database:
  driver: %s
  dsn: %q
  auto_migrate: true
  seed: false
cache:
  driver: memory
  memory:
    persist: false
jwt:
  secret: startup-migrate-test-secret
`, c.driver, dsn)
			if err := os.WriteFile(cfgPath, []byte(raw), 0o600); err != nil {
				t.Fatalf("写配置: %v", err)
			}
			cfg, err := config.Load(cfgPath)
			if err != nil {
				t.Fatalf("加载配置: %v", err)
			}

			// 先在库里埋一行 warehouse_id = 0 的流水，再启动。
			// 不埋的话，这个库上一次启动早就把存量回填完了，断言"没有 0 仓行"
			// 即使顺序反了也照样通过——那这条测试就只是在读上一次的结果。
			planted, plantedProduct := plantLegacyMovement(t, cfg)

			app, err := bootstrap.New(cfg)
			if err != nil {
				t.Fatalf("启动装配: %v", err)
			}
			t.Cleanup(app.Close)

			// 默认仓必须已经有了：没有它，回填那一步是空转。
			var defaults int64
			if err := app.DB.Model(&model.Warehouse{}).
				Where("is_default = ?", true).
				Count(&defaults).Error; err != nil {
				t.Fatalf("统计默认仓: %v", err)
			}
			if defaults == 0 {
				t.Fatal("启动后没有默认仓，回填无从下手")
			}

			if planted != "" {
				var got model.StockMovement
				if err := app.DB.Where("doc_no = ?", planted).First(&got).Error; err != nil {
					t.Fatalf("读回埋入的流水: %v", err)
				}
				if got.WarehouseID == 0 {
					t.Errorf("埋入的流水仍是 0 号仓：启动期回填没生效（默认仓与迁移的顺序反了？）")
				}
				t.Cleanup(func() {
					_ = app.DB.Unscoped().Where("doc_no = ?", planted).
						Delete(&model.StockMovement{}).Error
					if plantedProduct != 0 {
						_ = app.DB.Unscoped().Where("id = ?", plantedProduct).
							Delete(&model.Product{}).Error
					}
				})
			}

			// warehouse_id = 0 是迁移没跑的铁证：老数据没归到任何仓，
			// 与新写入的默认仓分成两组，库存看起来凭空翻倍。
			for _, table := range []string{"inv_stock_movement", "inv_stock_snapshot"} {
				var left int64
				if err := app.DB.Table(table).Where("warehouse_id = 0").Count(&left).Error; err != nil {
					t.Fatalf("统计 %s: %v", table, err)
				}
				if left != 0 {
					t.Errorf("%s 里还有 %d 行 warehouse_id = 0：启动期回填没生效", table, left)
				}
			}
		})
	}
}

// plantLegacyMovement 用独立连接往库里埋一行「迁移前」的流水，返回单号与所属产品 id。
//
// 表还不存在（第一次在这个库上跑）时返回空串：那时没有存量可埋，
// 启动期迁移本来也没什么可回填的。
//
// 流水要带一个真产品：md_product 上有外键（fk_inv_stock_movement_product），
// 随便编一个 id 会被 PG 当场拒掉——第一版就是这么撞的。
func plantLegacyMovement(t *testing.T, cfg *config.Config) (string, uint) {
	t.Helper()

	db, err := database.Open(cfg.Database, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatalf("连接数据库（埋数据）: %v", err)
	}
	defer func() { _ = database.Close(db) }()

	if !db.Migrator().HasTable(&model.StockMovement{}) ||
		!db.Migrator().HasTable(&model.Product{}) {
		return "", 0
	}

	suffix := time.Now().UnixNano()
	product := &model.Product{
		SKUCode: fmt.Sprintf("PLANT-%d", suffix),
		NameCN:  "迁移演练件",
		Unit:    "PCS",
	}
	if err := db.Create(product).Error; err != nil {
		t.Fatalf("埋产品: %v", err)
	}

	docNo := fmt.Sprintf("MV-PLANT-%d", suffix)
	row := &model.StockMovement{
		DocNo:      docNo,
		ProductID:  product.ID,
		SKUCode:    product.SKUCode,
		Type:       model.StockMovementPurchase,
		Direction:  model.StockDirectionIn,
		Qty:        money.FromInt(1),
		OccurredAt: model.DateOnly(time.Now()),
	}
	if err := db.Create(row).Error; err != nil {
		t.Fatalf("埋存量流水: %v", err)
	}
	if row.WarehouseID != 0 {
		t.Fatalf("埋入的流水仓库应为 0，实际 %d", row.WarehouseID)
	}
	return docNo, product.ID
}
