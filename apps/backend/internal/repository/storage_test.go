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

	if err := database.AutoMigrate(db, &model.Supplier{}, &model.Product{}); err != nil {
		t.Fatalf("迁移失败: %v", err)
	}

	repo := NewProductRepository(db)
	suffix := time.Now().UnixNano()
	mark := fmt.Sprintf("T%d", suffix)

	// 每次跑用独立供应商圈定数据集，避免和库里已有的演示数据混在一起。
	supplierRepo := NewSupplierRepository(db)
	supplier := &model.Supplier{Code: "SUP-" + mark, Name: "测试供应商", Status: model.StatusEnabled}
	if err := supplierRepo.Create(ctx, supplier); err != nil {
		t.Fatalf("创建供应商: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Unscoped().Where("supplier_id = ?", supplier.ID).Delete(&model.Product{}).Error
		_ = db.Unscoped().Where("id = ?", supplier.ID).Delete(&model.Supplier{}).Error
	})

	products := []*model.Product{
		{
			SkuCode:    mark + "-AbC",
			NameCN:     "不锈钢螺丝 M8",
			NameEN:     "Stainless Screw",
			Category:   "FS",
			Unit:       "PCS",
			SupplierID: &supplier.ID,
			Status:     model.StatusEnabled,
		},
		{
			SkuCode:    mark + "-002",
			NameCN:     "六角螺栓 M10",
			Category:   "FS",
			Unit:       "PCS",
			SupplierID: &supplier.ID,
			Status:     model.StatusEnabled,
		},
	}
	for _, p := range products {
		if err := repo.Create(ctx, p); err != nil {
			t.Fatalf("创建产品 %s: %v", p.SkuCode, err)
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
		exists, err := repo.ExistsByCode(ctx, products[0].SkuCode, 0)
		if err != nil || !exists {
			t.Fatalf("应判定为已存在: %v, %v", exists, err)
		}
		if exists, err := repo.ExistsByCode(ctx, products[0].SkuCode, products[0].ID); err != nil || exists {
			t.Fatalf("排除自身后不应命中: %v, %v", exists, err)
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
		if got.NameCN != "六角螺栓 M12" || got.SkuCode != products[1].SkuCode {
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
}
