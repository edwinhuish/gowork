package dto

// PartnerImportRow 是导入文件里的一行（已按表头映射到字段）。
type PartnerImportRow struct {
	// Line 是文件里的行号（含表头，从 1 起）：报错要能让人回到那一行去改。
	Line int
	Code string
	Name string
	Role string
	// Country / ContactName / Phone / Email / Address 都是可选。
	Country     string
	ContactName string
	Phone       string
	Email       string
	Address     string
	CreditDays  string
	CreditLimit string
}

// PartnerImportFailure 是一行没导进去的原因。
type PartnerImportFailure struct {
	Line   int    `json:"line"`
	Code   string `json:"code"`
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

// PartnerImportDuplicate 是一行"能导，但库里已经有一家很像"的提示。
//
// 只提示不阻断：外贸同名公司跨国家确实存在，拦住等于让人把询盘丢掉。
type PartnerImportDuplicate struct {
	Line    int      `json:"line"`
	Code    string   `json:"code"`
	Name    string   `json:"name"`
	Reasons []string `json:"reasons"`
}

// PartnerImportReport 是导入报告。
//
// 先出报告（dry_run）再落库：一份几百行的文件里错两三行是常态，
// 直接导进去之后"哪几行没进来"就没人说得清了。
type PartnerImportReport struct {
	// DryRun 为 true 表示只校验、没有落库。
	DryRun  bool `json:"dry_run"`
	Total   int  `json:"total"`
	Created int  `json:"created"`
	// Skipped 是通过校验但没落库的行数（dry_run 时等于 Valid）。
	Skipped int `json:"skipped"`

	Failures   []PartnerImportFailure   `json:"failures"`
	Duplicates []PartnerImportDuplicate `json:"duplicates"`
}
