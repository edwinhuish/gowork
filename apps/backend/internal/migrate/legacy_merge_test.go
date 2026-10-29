package migrate

import (
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/edwinhuish/gowork/internal/config"
	"github.com/edwinhuish/gowork/internal/infra/database"
	"github.com/edwinhuish/gowork/internal/model"
)

func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := database.Open(config.Database{
		Driver:      config.DriverSQLite,
		DSN:         filepath.Join(t.TempDir(), "migrate.db"),
		MaxOpen:     2,
		MaxIdle:     1,
		MaxLifetime: 60,
		LogLevel:    "error",
	}, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatalf("打开测试数据库: %v", err)
	}
	t.Cleanup(func() { _ = database.Close(db) })
	return db
}

// legacyDDL 复刻合并前两张表的形状。
//
// 刻意不用 model 来建表：旧模型已经从代码里删掉了，迁移要面对的正是
// "模型已经不存在、但库里还留着表"这个状态。用原始 DDL 建表更贴近真实。
var legacyDDL = []string{
	`CREATE TABLE md_supplier (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		created_at datetime, updated_at datetime, deleted_at datetime,
		code varchar(32), name varchar(128), contact varchar(64), phone varchar(32),
		email varchar(128), address varchar(255), status integer, remark varchar(255))`,
	`CREATE TABLE md_customer (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		created_at datetime, updated_at datetime, deleted_at datetime,
		code varchar(32), name varchar(128), country varchar(64), contact varchar(64),
		phone varchar(32), email varchar(128), address varchar(255), status integer, remark varchar(255))`,
}

func TestMergeLegacyCustomersAndSuppliers(t *testing.T) {
	db := newTestDB(t)
	log := slog.New(slog.DiscardHandler)

	if err := database.AutoMigrate(db, &model.Partner{}, &model.Product{}); err != nil {
		t.Fatalf("同步新表结构: %v", err)
	}

	// 先把 md_partner 的自增序列推高一格，再物理删掉占位记录。
	//
	// 不做这一步的话，新表是空的，供应商导入后 id 恰好从 1 开始、与原表完全重合，
	// 重映射即使写错了（比如根本没执行）测试也照样通过。推高之后新 id 全程偏移，
	// 产品的 supplier_id 必须真的被改写才能对上。
	placeholder := &model.Partner{
		Code: "__seq__", Name: "占位", Role: model.PartnerRoleSupplier, Status: model.EnableStatusOn,
	}
	if err := db.Create(placeholder).Error; err != nil {
		t.Fatalf("写入占位记录: %v", err)
	}
	if err := db.Unscoped().Delete(&model.Partner{}, placeholder.ID).Error; err != nil {
		t.Fatalf("物理删除占位记录: %v", err)
	}

	for _, ddl := range legacyDDL {
		if err := db.Exec(ddl).Error; err != nil {
			t.Fatalf("创建旧表: %v", err)
		}
	}

	// 用一个明确的过去时间，才能验证迁移保留了原始时间戳而不是重新生成。
	legacyAt := time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)
	seed := []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO md_supplier (id, code, name, contact, status, created_at, updated_at)
		  VALUES (1, 'SUP001', '永康市华信五金厂', '陈先生', 1, ?, ?)`, []any{legacyAt, legacyAt}},
		// 软删除的供应商必须一起搬走，否则引用它的历史单据会指向空。
		{`INSERT INTO md_supplier (id, code, name, status, deleted_at, created_at, updated_at)
		  VALUES (2, 'SUP002', '揭阳铭泰不锈钢制品', 1, ?, ?, ?)`, []any{legacyAt, legacyAt, legacyAt}},
		{`INSERT INTO md_customer (id, code, name, country, status, created_at, updated_at)
		  VALUES (1, 'CUS001', 'Ferreteria El Sol', 'Ecuador', 1, ?, ?)`, []any{legacyAt, legacyAt}},
		// 与供应商同码：两张表各自唯一，合并后要全局唯一，必须自动加后缀。
		{`INSERT INTO md_customer (id, code, name, status, created_at, updated_at)
		  VALUES (2, 'SUP001', '撞码的客户', 1, ?, ?)`, []any{legacyAt, legacyAt}},
		{`INSERT INTO md_product (id, sku_code, name_cn, supplier_id, status) VALUES (1, 'P1', '产品一', 1, 1)`, nil},
		{`INSERT INTO md_product (id, sku_code, name_cn, supplier_id, status) VALUES (2, 'P2', '产品二', 2, 1)`, nil},
		{`INSERT INTO md_product (id, sku_code, name_cn, supplier_id, status) VALUES (3, 'P3', '产品三', NULL, 1)`, nil},
	}
	for _, s := range seed {
		if err := db.Exec(s.sql, s.args...).Error; err != nil {
			t.Fatalf("写入旧数据: %v", err)
		}
	}

	if err := MergeLegacyCustomersAndSuppliers(db, log); err != nil {
		t.Fatalf("执行合并迁移: %v", err)
	}

	var partners []model.Partner
	if err := db.Unscoped().Order("id").Find(&partners).Error; err != nil {
		t.Fatalf("读取 md_partner: %v", err)
	}
	if len(partners) != 4 {
		t.Fatalf("应合并出 4 条主体，得到 %d 条", len(partners))
	}

	byCode := make(map[string]model.Partner, len(partners))
	for _, p := range partners {
		byCode[p.Code] = p
	}

	t.Run("角色按来源表赋值", func(t *testing.T) {
		if p := byCode["SUP001"]; p.Role != model.PartnerRoleSupplier {
			t.Fatalf("SUP001 的角色应为供应商，得到 %s", p.Role)
		}
		if p := byCode["CUS001"]; p.Role != model.PartnerRoleCustomer {
			t.Fatalf("CUS001 的角色应为客户，得到 %s", p.Role)
		}
	})

	t.Run("旧表只保留企业类型", func(t *testing.T) {
		for _, p := range partners {
			if p.EntityType != model.PartnerEntityCompany {
				t.Fatalf("%s 应迁移为公司类型，得到 %d", p.Code, p.EntityType)
			}
		}
	})

	t.Run("客户的国家被带过来", func(t *testing.T) {
		if p := byCode["CUS001"]; p.Country != "Ecuador" {
			t.Fatalf("CUS001 的国家应为 Ecuador，得到 %q", p.Country)
		}
	})

	t.Run("软删除状态保留", func(t *testing.T) {
		p, ok := byCode["SUP002"]
		if !ok {
			t.Fatal("软删除的供应商没有被迁移")
		}
		// 新模型的删除标记是 Unix 毫秒整数，0 表示未删除。
		if p.DeletedAt == 0 {
			t.Fatal("软删除标记丢失，已删除的主体复活了")
		}
	})

	t.Run("原始时间戳保留", func(t *testing.T) {
		if p := byCode["SUP001"]; p.CreatedAt.Year() != 2020 {
			t.Fatalf("创建时间应保留为 2020 年，得到 %s", p.CreatedAt)
		}
	})

	t.Run("编码冲突时加后缀", func(t *testing.T) {
		p, ok := byCode["SUP001-2"]
		if !ok {
			t.Fatalf("撞码的客户应被改名为 SUP001-2，实际编码有: %v", keys(byCode))
		}
		if p.Role != model.PartnerRoleCustomer {
			t.Fatalf("加后缀的应是客户，得到 %s", p.Role)
		}
		if p.Name != "撞码的客户" {
			t.Fatalf("改名不应影响名称，得到 %q", p.Name)
		}
	})

	t.Run("产品的供应商引用被重映射", func(t *testing.T) {
		supplier := byCode["SUP001"]
		if supplier.ID == 1 {
			t.Fatalf("测试前提被破坏：新 id 与原 id 相同（%d），无法验证重映射", supplier.ID)
		}

		var p1, p2, p3 model.Product
		if err := db.First(&p1, 1).Error; err != nil {
			t.Fatalf("读取产品 1: %v", err)
		}
		if p1.SupplierID == nil || *p1.SupplierID != supplier.ID {
			t.Fatalf("产品 1 的供应商应重映射到 %d，得到 %v", supplier.ID, p1.SupplierID)
		}

		if err := db.First(&p2, 2).Error; err != nil {
			t.Fatalf("读取产品 2: %v", err)
		}
		if p2.SupplierID == nil || *p2.SupplierID != byCode["SUP002"].ID {
			t.Fatalf("产品 2 的供应商应重映射到 %d，得到 %v", byCode["SUP002"].ID, p2.SupplierID)
		}

		// 没有供应商的产品不该被顺手填上一个。
		if err := db.First(&p3, 3).Error; err != nil {
			t.Fatalf("读取产品 3: %v", err)
		}
		if p3.SupplierID != nil {
			t.Fatalf("产品 3 的供应商应保持为空，得到 %v", p3.SupplierID)
		}
	})

	t.Run("重复执行不产生重复数据", func(t *testing.T) {
		if err := MergeLegacyCustomersAndSuppliers(db, log); err != nil {
			t.Fatalf("第二次执行应直接跳过: %v", err)
		}
		var n int64
		if err := db.Unscoped().Model(&model.Partner{}).Count(&n).Error; err != nil {
			t.Fatalf("统计: %v", err)
		}
		if n != 4 {
			t.Fatalf("重复执行后应仍是 4 条，得到 %d 条", n)
		}

		var records int64
		if err := db.Model(&Record{}).Where("name = ?", mergeLegacyRecord).Count(&records).Error; err != nil {
			t.Fatalf("统计迁移记录: %v", err)
		}
		if records != 1 {
			t.Fatalf("迁移记录应恰好一条，得到 %d 条", records)
		}
	})
}

// TestMergeLegacyCustomersAndSuppliersSkipsWithoutLegacyTables 覆盖全新库的启动路径：
// 没有旧表时迁移应当安静跳过，且不写迁移记录 ——
// 这样将来导入一份旧库，迁移仍然会正常触发。
func TestMergeLegacyCustomersAndSuppliersSkipsWithoutLegacyTables(t *testing.T) {
	db := newTestDB(t)
	log := slog.New(slog.DiscardHandler)

	if err := database.AutoMigrate(db, &model.Partner{}, &model.Product{}); err != nil {
		t.Fatalf("同步新表结构: %v", err)
	}
	if err := db.AutoMigrate(&Record{}); err != nil {
		t.Fatalf("创建迁移记录表: %v", err)
	}

	if err := MergeLegacyCustomersAndSuppliers(db, log); err != nil {
		t.Fatalf("没有旧表时应直接返回: %v", err)
	}

	var records int64
	if err := db.Model(&Record{}).Count(&records).Error; err != nil {
		t.Fatalf("统计迁移记录: %v", err)
	}
	if records != 0 {
		t.Fatalf("全新库不应写入迁移记录，得到 %d 条", records)
	}
}

// TestMergeLegacyCustomersAndSuppliersRefusesOnExistingData 验证"目标表已有数据却没有迁移记录"时
// 会停下来报错，而不是继续插入造成重复主体。
func TestMergeLegacyCustomersAndSuppliersRefusesOnExistingData(t *testing.T) {
	db := newTestDB(t)
	log := slog.New(slog.DiscardHandler)

	if err := database.AutoMigrate(db, &model.Partner{}, &model.Product{}); err != nil {
		t.Fatalf("同步新表结构: %v", err)
	}
	for _, ddl := range legacyDDL {
		if err := db.Exec(ddl).Error; err != nil {
			t.Fatalf("创建旧表: %v", err)
		}
	}
	if err := db.Create(&model.Partner{
		Code: "EXIST", Name: "已存在", Role: model.PartnerRoleSupplier, Status: model.EnableStatusOn,
	}).Error; err != nil {
		t.Fatalf("写入已有主体: %v", err)
	}

	err := MergeLegacyCustomersAndSuppliers(db, log)
	if err == nil {
		t.Fatal("目标表已有数据时应中止并报错，而不是重复导入")
	}
	// 必须是因为"目标表已有数据"而中止，而不是其它任何故障。
	// 只断言"有报错"会让这个用例在连接失败、建表失败时也变绿 —— 那是假阳性。
	if !strings.Contains(err.Error(), "已有") {
		t.Fatalf("中止原因不对，应提示目标表已有数据，实际: %v", err)
	}
}

func keys(m map[string]model.Partner) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
