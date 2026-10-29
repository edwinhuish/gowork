package database

import (
	"fmt"
	"log/slog"
	"strings"

	"gorm.io/gorm"

	"github.com/edwinhuish/gowork/internal/model"
)

// legacyUniqueIndexes 是改造前那些**只含业务列**的唯一索引。
//
// 新定义把 deleted_at 排进了复合索引（未删除的行这一列恒为 0，等价于"未删才唯一"），
// 而 AutoMigrate 建索引是 `IF NOT EXISTS` 语义：名字不同就再建一个，定义变了却不动老的。
// 留着旧索引的后果不是"多一个索引"，而是旧的单列唯一继续拦着——删掉产品后同 SKU
// 仍然建不回来，看起来像改造没生效。
//
// 应收应付（biz_receivable / biz_payable）不在这里：它们的 DocNo 在嵌入的 Settlement 里，
// 两张表共用同一个 tag，命名索引会撞名，因此保持单列唯一。
var legacyUniqueIndexes = []struct {
	model any
	name  string
}{
	{&model.Product{}, "idx_md_product_sku_code"},
	{&model.Partner{}, "idx_md_partner_code"},
	{&model.Currency{}, "idx_md_currency_code"},
	{&model.User{}, "idx_sys_user_username"},
	{&model.ExchangeRate{}, "uni_exchange_rate_pair"},
	{&model.ProductCode{}, "uni_product_code"},
	{&model.Quotation{}, "uni_quotation_doc_version"},
	{&model.SalesOrder{}, "idx_biz_sales_order_doc_no"},
	{&model.PurchaseOrder{}, "idx_biz_purchase_order_doc_no"},
	{&model.Shipment{}, "idx_biz_shipment_doc_no"},
	{&model.Payment{}, "idx_biz_payment_doc_no"},
	{&model.Contract{}, "idx_biz_contract_doc_no"},
	{&model.StockMovement{}, "idx_inv_stock_movement_doc_no"},
	{&model.StockDocument{}, "idx_inv_stock_document_doc_no"},
	{&model.StockSnapshot{}, "idx_inv_stock_snapshot_product_id"},
}

// prepareSoftDelete 把存量库的软删标记从 timestamp 换成 Unix 毫秒整数，
// 并删掉只含业务列的旧唯一索引，好让随后的 AutoMigrate 按新定义重建。
//
// 放在 AutoMigrate 里而不是单独一步：列类型不匹配时 PG 的 AutoMigrate 会直接
// ALTER 失败（timestamp 不能自动转 bigint），sqlite 又不允许删掉带索引的列——
// 调用方只要漏调一次，升级就在半路炸开。
//
// 幂等靠"看列的实际类型"：已是整数就跳过，不需要额外的记账表。
func prepareSoftDelete(db *gorm.DB, models ...any) error {
	log := slog.Default()
	dialect := db.Dialector.Name()

	for _, item := range legacyUniqueIndexes {
		if !db.Migrator().HasTable(item.model) || !db.Migrator().HasIndex(item.model, item.name) {
			continue
		}
		if err := db.Migrator().DropIndex(item.model, item.name); err != nil {
			return fmt.Errorf("删除旧的唯一索引 %s: %w", item.name, err)
		}
		log.Info("已删除旧的唯一索引，稍后按新定义重建", "index", item.name)
	}

	for _, m := range models {
		table := tableOf(db, m)
		if table == "" {
			continue
		}
		columnType, err := deletedAtColumnType(db, dialect, table)
		if err != nil {
			return fmt.Errorf("读取 %s.deleted_at 的类型: %w", table, err)
		}
		// 空串表示列还不存在（新表），含 int 表示已经改造过，两种情况都跳过。
		if columnType == "" || strings.Contains(columnType, "int") {
			continue
		}

		// Base 在 deleted_at 上还有个普通索引，也要先删：sqlite 删列时
		// 只要还有索引引用它就直接报错（PG / MySQL 会自己重建，先删无害）。
		if idx := "idx_" + table + "_deleted_at"; db.Migrator().HasIndex(m, idx) {
			if err := db.Migrator().DropIndex(m, idx); err != nil {
				return fmt.Errorf("删除 %s 上的索引 %s: %w", table, idx, err)
			}
		}

		if err := convertDeletedAtToMillis(db, dialect, table); err != nil {
			return fmt.Errorf("改造 %s.deleted_at: %w", table, err)
		}
		log.Info("软删标记已改为 Unix 毫秒", "table", table, "原类型", columnType)
	}
	return nil
}

func tableOf(db *gorm.DB, m any) string {
	stmt := &gorm.Statement{DB: db}
	if err := stmt.Parse(m); err != nil || stmt.Schema == nil {
		return ""
	}
	return stmt.Schema.Table
}

// deletedAtColumnType 返回列的声明类型；列不存在时返回空串。
func deletedAtColumnType(db *gorm.DB, dialect, table string) (string, error) {
	switch dialect {
	case "sqlite":
		// PRAGMA 不接受参数占位符，表名只能拼进去；它来自模型定义，不是外部输入。
		var rows []struct {
			Name string
			Type string
		}
		if err := db.Raw(fmt.Sprintf("PRAGMA table_info('%s')", table)).Scan(&rows).Error; err != nil {
			return "", err
		}
		for _, r := range rows {
			if r.Name == "deleted_at" {
				return strings.ToLower(r.Type), nil
			}
		}
		return "", nil
	case "postgres":
		return queryColumnType(db,
			`SELECT data_type FROM information_schema.columns
			  WHERE table_schema = current_schema() AND table_name = ? AND column_name = 'deleted_at'`,
			table)
	default:
		return queryColumnType(db,
			`SELECT data_type FROM information_schema.columns
			  WHERE table_schema = DATABASE() AND table_name = ? AND column_name = 'deleted_at'`,
			table)
	}
}

func queryColumnType(db *gorm.DB, query, table string) (string, error) {
	var types []string
	if err := db.Raw(query, table).Scan(&types).Error; err != nil {
		return "", err
	}
	if len(types) == 0 {
		return "", nil
	}
	return strings.ToLower(types[0]), nil
}

// convertDeletedAtToMillis 就地把旧的软删时间戳换成 Unix 毫秒。
//
// 未删除（NULL）统一成 0，已删除保留时间点——只换表示，不改变"哪些行被删了"。
func convertDeletedAtToMillis(db *gorm.DB, dialect, table string) error {
	var stmts []string
	switch dialect {
	case "sqlite":
		// sqlite 的 ALTER 能力最弱：加临时列 → 转值 → 删旧列 → 改名。
		// 列声明为 datetime 也能存整数（动态类型），但声明与实际不符会让后来的
		// 结构比对反复尝试改建，不如一次做干净。
		stmts = []string{
			fmt.Sprintf("ALTER TABLE %s ADD COLUMN deleted_at_new INTEGER NOT NULL DEFAULT 0", table),
			// 用 julianday 而不是 strftime('%s')：后者只到秒，
			// 会把同一秒内删掉的记录抹成同一个时间点。
			fmt.Sprintf(`UPDATE %s SET deleted_at_new = CASE
				WHEN deleted_at IS NULL THEN 0
				ELSE CAST(ROUND((julianday(deleted_at) - 2440587.5) * 86400000) AS INTEGER) END`, table),
			fmt.Sprintf("ALTER TABLE %s DROP COLUMN deleted_at", table),
			fmt.Sprintf("ALTER TABLE %s RENAME COLUMN deleted_at_new TO deleted_at", table),
		}
	case "postgres":
		stmts = []string{
			fmt.Sprintf("ALTER TABLE %s ALTER COLUMN deleted_at DROP DEFAULT", table),
			fmt.Sprintf(`ALTER TABLE %s ALTER COLUMN deleted_at TYPE bigint
				USING (CASE WHEN deleted_at IS NULL THEN 0::bigint
				            ELSE (EXTRACT(EPOCH FROM deleted_at) * 1000)::bigint END)`, table),
		}
	default: // mysql
		stmts = []string{
			fmt.Sprintf("ALTER TABLE %s ADD COLUMN deleted_at_new BIGINT NOT NULL DEFAULT 0", table),
			fmt.Sprintf(`UPDATE %s SET deleted_at_new = IFNULL(UNIX_TIMESTAMP(deleted_at) * 1000, 0)`, table),
			fmt.Sprintf("ALTER TABLE %s DROP COLUMN deleted_at", table),
			fmt.Sprintf("ALTER TABLE %s CHANGE COLUMN deleted_at_new deleted_at BIGINT NOT NULL DEFAULT 0", table),
		}
	}

	for _, s := range stmts {
		if err := db.Exec(s).Error; err != nil {
			return err
		}
	}
	return nil
}
