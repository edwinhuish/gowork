package migrate

import (
	"log/slog"
	"strings"
	"testing"
)

// legacyPartyDDL 复刻改名前 md_party 的形状。
//
// 与 legacyDDL 同样的理由：要验证的正是"表还在库里、模型已经改名"这个状态，
// 用原始 DDL 建表比用（已经不存在的）旧模型更贴近真实。
const legacyPartyDDL = `CREATE TABLE md_party (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	created_at datetime, updated_at datetime, deleted_at datetime,
	code varchar(32), name varchar(128), role integer, entity_type integer,
	country varchar(64), contact varchar(64), phone varchar(32),
	email varchar(128), address varchar(255), status integer, remark varchar(255))`

func TestRenameLegacyPartnerTable(t *testing.T) {
	t.Run("旧表在、新表不在时改名并保留数据", func(t *testing.T) {
		db := newTestDB(t)
		log := slog.New(slog.DiscardHandler)

		if err := db.Exec(legacyPartyDDL).Error; err != nil {
			t.Fatalf("建旧表: %v", err)
		}
		if err := db.Exec(
			`INSERT INTO md_party (code, name, role, status) VALUES ('SUP001', '旧供应商', 2, 1)`,
		).Error; err != nil {
			t.Fatalf("写入旧表: %v", err)
		}

		if err := RenameLegacyPartnerTable(db, log); err != nil {
			t.Fatalf("改名失败: %v", err)
		}

		m := db.Migrator()
		if m.HasTable(legacyPartnerTable) {
			t.Errorf("改名后 %s 仍然存在", legacyPartnerTable)
		}
		if !m.HasTable(partnerTable) {
			t.Fatalf("改名后 %s 不存在", partnerTable)
		}

		var name string
		if err := db.Table(partnerTable).Select("name").Where("code = ?", "SUP001").
			Scan(&name).Error; err != nil {
			t.Fatalf("读取改名后的数据: %v", err)
		}
		if name != "旧供应商" {
			t.Errorf("数据没有跟着改名走: 读到 %q", name)
		}
	})

	t.Run("没有旧表时什么也不做", func(t *testing.T) {
		db := newTestDB(t)
		log := slog.New(slog.DiscardHandler)

		if err := RenameLegacyPartnerTable(db, log); err != nil {
			t.Fatalf("空库上调用不应该报错: %v", err)
		}
		if db.Migrator().HasTable(partnerTable) {
			t.Errorf("空库上不应该凭空建出 %s", partnerTable)
		}
	})

	t.Run("两张表同时存在时报错且都不动", func(t *testing.T) {
		db := newTestDB(t)
		log := slog.New(slog.DiscardHandler)

		if err := db.Exec(legacyPartyDDL).Error; err != nil {
			t.Fatalf("建旧表: %v", err)
		}
		if err := db.Exec(`CREATE TABLE md_partner (
			id INTEGER PRIMARY KEY AUTOINCREMENT, code varchar(32), name varchar(128))`).Error; err != nil {
			t.Fatalf("建新表: %v", err)
		}

		err := RenameLegacyPartnerTable(db, log)
		if err == nil {
			t.Fatal("两张表同时存在时应该报错中止")
		}
		if !strings.Contains(err.Error(), partnerTable) {
			t.Errorf("错误信息应该点出冲突的表名，实际: %v", err)
		}

		m := db.Migrator()
		if !m.HasTable(legacyPartnerTable) || !m.HasTable(partnerTable) {
			t.Errorf("报错时不应该动任何一张表")
		}
	})

	t.Run("重复执行是幂等的", func(t *testing.T) {
		db := newTestDB(t)
		log := slog.New(slog.DiscardHandler)

		if err := db.Exec(legacyPartyDDL).Error; err != nil {
			t.Fatalf("建旧表: %v", err)
		}
		if err := RenameLegacyPartnerTable(db, log); err != nil {
			t.Fatalf("第一次改名失败: %v", err)
		}
		if err := RenameLegacyPartnerTable(db, log); err != nil {
			t.Fatalf("第二次调用应该直接返回: %v", err)
		}
	})
}
