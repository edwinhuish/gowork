package migrate

import (
	"fmt"
	"log/slog"
	"time"

	"gorm.io/gorm"

	"github.com/edwinhuish/gowork/internal/model"
)

// mergeLegacyRecord 是这条迁移在 sys_migration 里的登记名。
//
// 名字里的 "party" 保持原样，不随模型改名成 Partner 而改：
// 它是已经落库的历史标识，换了字面量，已有库会认为这条迁移没跑过而重跑一遍。
const mergeLegacyRecord = "20261028_merge_customer_supplier_into_party"

// legacyPartner 是旧表 md_customer / md_supplier 的读取载体。
//
// 旧模型已随合并删除，不能复用 model，所以这里按列名把两张表读进同一个结构体
// （供应商表没有 country，读出来就是空串）。
//
// 读取时**必须加 Unscoped**：软删除条件是由目标 struct 的 schema 决定的，
// 只要结构体里有 DeletedAt 字段，gorm 就会自动追加条件，
// 用 Table() 指定表名也绕不过去。软删除的主体必须一起搬过去，
// 否则引用它们的历史单据会指向一个查不到的主体。
//
// DeletedAt 这里仍用 gorm.DeletedAt（timestamp + IS NULL），不跟新模型统一：
// 旧表就是这么建的表，列类型是时间戳，换成整数类型反而读不出来。
// 搬到新模型时再换算成 Unix 毫秒，见 mergeLegacyTable 里的转换。
type legacyPartner struct {
	ID        uint
	Code      string
	Name      string
	Country   string
	Contact   string
	Phone     string
	Email     string
	Address   string
	Status    model.EnableStatus
	Remark    string
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt
}

// PreparePartnerTable 在合并旧主体之前先把 md_partner 建出来。
//
// 合并必须排在 AutoMigrate 之前：AutoMigrate 会按模型给 md_product.supplier_id
// 建指向 md_partner 的外键，而老库存的是 md_supplier 的 id——建外键那一刻
// 数据就已经违反约束，整个升级当场失败。
//
// 但合并要往 md_partner 里写，表得先存在，所以这里只建这一张。
// 其余结构仍交给 AutoMigrate，避免建表逻辑分叉。
func PreparePartnerTable(db *gorm.DB) error {
	if db.Migrator().HasTable(&model.Partner{}) {
		return nil
	}
	if err := db.Migrator().CreateTable(&model.Partner{}); err != nil {
		return fmt.Errorf("创建 md_partner: %w", err)
	}
	return nil
}

// MergeLegacyCustomersAndSuppliers 把 md_customer 与 md_supplier 合并进 md_partner。
//
// 函数名按"搬什么"来取，而不是叫 MergePartners：Partner 只是落点，
// 这样命名读起来不会被误解成"在合并多个伙伴"。
//
// 三件事必须一起做，否则数据会处于互相矛盾的状态：
//  1. 两张旧表的主体搬进新表，客户 role=customer、供应商 role=supplier；
//  2. 记录旧 id → 新 id 的对应关系；
//  3. 用这张对应关系重写 md_product.supplier_id。
//
// 两表的 id 都从 1 开始，合并后不可能都沿用，所以只能重新分配再重映射。
// 供应商先导入，于是新表的前若干条 id 恰好与原供应商 id 相同，
// 大部分产品引用实际上不用改，但代码不能依赖这个巧合。
func MergeLegacyCustomersAndSuppliers(db *gorm.DB, log *slog.Logger) error {
	if err := ensureRecordTable(db); err != nil {
		return err
	}

	hasSupplier := db.Migrator().HasTable("md_supplier")
	hasCustomer := db.Migrator().HasTable("md_customer")
	if !hasSupplier && !hasCustomer {
		// 全新库：没有可搬的数据。刻意不写 applied 记录，
		// 这样将来导入一份旧库（自带两张旧表）时迁移仍会正常触发。
		return nil
	}

	done, err := applied(db, mergeLegacyRecord)
	if err != nil {
		return err
	}
	if done {
		return nil
	}

	var existing int64
	if err := db.Unscoped().Model(&model.Partner{}).Count(&existing).Error; err != nil {
		return fmt.Errorf("统计 md_partner 记录数: %w", err)
	}
	if existing > 0 {
		// 没有 applied 记录但目标表已有数据：上一次运行在中途被打断，
		// 或者有人手工导过数据。继续插入只会产生重复主体，
		// 而这种重复很难事后识别，所以宁可停在这里让人确认。
		return fmt.Errorf(
			"md_partner 已有 %d 条记录但缺少迁移记录 %q，为避免重复导入已中止；"+
				"确认数据无误后手工执行 INSERT INTO sys_migration(name, applied_at) 补记录",
			existing, mergeLegacyRecord)
	}

	return db.Transaction(func(tx *gorm.DB) error {
		taken := make(map[string]bool)
		var codes []string
		if err := tx.Unscoped().Model(&model.Partner{}).Pluck("code", &codes).Error; err != nil {
			return fmt.Errorf("读取已有主体编码: %w", err)
		}
		for _, c := range codes {
			taken[c] = true
		}

		var supplierMap map[uint]uint
		if hasSupplier {
			// 供应商先导入并优先占用编码：产品的 supplier_id 指向供应商，
			// 且供应商编码是对账用的外部标识，比客户编码更不该被改写。
			if supplierMap, err = importLegacy(tx, "md_supplier", model.PartnerRoleSupplier, taken, log); err != nil {
				return err
			}
		}
		if hasCustomer {
			if _, err := importLegacy(tx, "md_customer", model.PartnerRoleCustomer, taken, log); err != nil {
				return err
			}
		}

		if err := remapProductSupplier(tx, supplierMap, log); err != nil {
			return err
		}
		return markApplied(tx, mergeLegacyRecord)
	})
}

// importLegacy 把一张旧表的数据搬进 md_partner，返回 旧 id → 新 id 的映射。
func importLegacy(
	tx *gorm.DB,
	table string,
	role model.PartnerRole,
	taken map[string]bool,
	log *slog.Logger,
) (map[uint]uint, error) {
	var rows []legacyPartner
	if err := tx.Table(table).Unscoped().Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("读取旧表 %s: %w", table, err)
	}

	idMap := make(map[uint]uint, len(rows))
	for i := range rows {
		row := &rows[i]

		code := uniqueCode(row.Code, taken)
		taken[code] = true
		if code != row.Code {
			// 两张旧表的编码各自唯一，合并后要全局唯一，冲突只能加后缀。
			// 这是唯一会改写业务编码的地方，所以日志级别提到 Warn。
			log.Warn("旧编码在合并后重复，已加后缀",
				"table", table, "old_id", row.ID, "old_code", row.Code, "new_code", code)
		}

		p := &model.Partner{
			Code: code,
			Name: row.Name,
			Role: role,
			// 旧表只有企业主体，个人类型是这次新增的能力，迁移不涉及。
			EntityType:  model.PartnerEntityCompany,
			Country:     row.Country,
			ContactName: row.Contact,
			Phone:       row.Phone,
			Email:       row.Email,
			Address:     row.Address,
			Status:      row.Status,
			Remark:      row.Remark,
		}
		// 保留原始时间戳与软删除状态：搬运不是业务操作，
		// 不应该让所有记录看起来都是"今天刚建的"，也不该让已删除的主体复活。
		p.CreatedAt = row.CreatedAt
		p.UpdatedAt = row.UpdatedAt
		// 旧表的 deleted_at 是 timestamp（gorm.DeletedAt），新表是 Unix 毫秒整数，
		// 这里换算一次：直接赋值类型不同，而丢掉删除标记会让已删主体复活。
		if row.DeletedAt.Valid {
			p.DeletedAt = model.DeletedAt(row.DeletedAt.Time.UnixMilli())
		}

		if err := tx.Create(p).Error; err != nil {
			return nil, fmt.Errorf("导入 %s id=%d: %w", table, row.ID, err)
		}
		idMap[row.ID] = p.ID
	}

	log.Info("已合并旧表到 md_partner", "table", table, "role", role.String(), "rows", len(rows))
	return idMap, nil
}

// remapProductSupplier 用映射表重写产品上的供应商引用。
//
// 逐条按主键更新而不是 `UPDATE ... SET supplier_id = CASE ...`：
// 后者在 old_id 与 new_id 有交集时会二次命中（改完某行后它可能又落进
// 后面某个 old_id 的条件里）。按主键定位不存在这个问题，
// 数据量也只是产品条数，不值得为省几次往返引入正确性风险。
func remapProductSupplier(tx *gorm.DB, idMap map[uint]uint, log *slog.Logger) error {
	if len(idMap) == 0 {
		return nil
	}

	type productRef struct {
		ID         uint
		SupplierID *uint
	}
	var refs []productRef
	// 软删除的产品也要重映射，否则它将来被恢复时，供应商指向会是一条旧 id。
	// productRef 不带 DeletedAt 字段本来就不会触发软删除条件，这里显式写 Unscoped
	// 是为了把意图固化下来 —— 哪天有人给它加了这个字段，行为不会悄悄变。
	if err := tx.Unscoped().Table("md_product").Select("id", "supplier_id").
		Where("supplier_id IS NOT NULL").Find(&refs).Error; err != nil {
		return fmt.Errorf("读取产品供应商引用: %w", err)
	}

	moved := 0
	for _, ref := range refs {
		newID, ok := idMap[*ref.SupplierID]
		if !ok {
			// 指向了一个不存在的供应商（历史脏数据）。保留原值不动：
			// 保留会表现为"供应商缺失"，清空则会变成"未指定供应商"，
			// 后者看起来像正常数据，更难被发现。
			log.Warn("产品的供应商引用找不到对应旧记录，已保留原值",
				"product_id", ref.ID, "supplier_id", *ref.SupplierID)
			continue
		}
		if newID == *ref.SupplierID {
			continue
		}
		if err := tx.Table("md_product").Where("id = ?", ref.ID).
			Update("supplier_id", newID).Error; err != nil {
			return fmt.Errorf("更新产品 %d 的供应商引用: %w", ref.ID, err)
		}
		moved++
	}

	log.Info("已重映射产品供应商引用", "updated", moved, "total", len(refs))
	return nil
}

// uniqueCode 在编码冲突时追加 -2、-3 这样的后缀，直到不冲突。
func uniqueCode(code string, taken map[string]bool) string {
	if !taken[code] {
		return code
	}
	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s-%d", code, i)
		if !taken[candidate] {
			return candidate
		}
	}
}
