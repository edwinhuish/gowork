package migrate

import (
	"fmt"
	"log/slog"

	"gorm.io/gorm"
)

const (
	// legacyPartnerTable 是模型改名前用过的表名（Party）。
	legacyPartnerTable = "md_party"
	// partnerTable 是当前表名（Partner）。
	partnerTable = "md_partner"
)

// RenameLegacyPartnerTable 把旧表 md_party 改名为 md_partner。
//
// 必须在 AutoMigrate **之前**调用。AutoMigrate 只认模型，看到模型叫 md_partner
// 就会新建一张空表；若此时旧表还在，改名会撞名失败，而旧数据就被晾在
// md_party 里无人过问 —— 表面上启动成功，实际上列表全空。
//
// 不需要 sys_migration 记录：判断条件本身就是"旧表在、新表不在"，
// 改名完成后条件不再成立，重复执行什么也不做，天然幂等。
//
// 继承下来的索引名仍是 idx_md_party_*：三种库的 RENAME 都不会改索引名，
// AutoMigrate 随后会按新表名再建一套等价索引。对几十行的主数据表，
// 多一套索引的代价可以忽略，不值得为此写一套三库行为不一致的索引重命名。
func RenameLegacyPartnerTable(db *gorm.DB, log *slog.Logger) error {
	m := db.Migrator()

	if !m.HasTable(legacyPartnerTable) {
		return nil
	}
	if m.HasTable(partnerTable) {
		// 两张表同时存在：可能是建了新表却没搬数据，也可能有人手工建过。
		// 无法判断哪张才是权威数据，改名会覆盖或丢失其中一份，所以停下来让人确认。
		return fmt.Errorf(
			"表 %s 与 %s 同时存在，无法判断以哪张为准，已中止改名；"+
				"确认 %s 里的数据可以丢弃后手工删除该表再启动",
			legacyPartnerTable, partnerTable, partnerTable)
	}

	if err := m.RenameTable(legacyPartnerTable, partnerTable); err != nil {
		return fmt.Errorf("把 %s 改名为 %s: %w", legacyPartnerTable, partnerTable, err)
	}
	log.Info("已把旧表改名", "from", legacyPartnerTable, "to", partnerTable)
	return nil
}
