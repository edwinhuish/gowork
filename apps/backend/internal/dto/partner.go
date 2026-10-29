package dto

import (
	"time"

	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/money"
)

// PartnerRoleInput 是角色集合的可选入参：1 客户 / 2 供应商 / 3 两者。
//
// 用指针区分"没提交"和"提交了某个值"。这条区分是必要的：
// 客户与供应商的兼容入口（/customers、/suppliers）不传 role，
// 若把 nil 当成零值写库，编辑一次就会把"客供两用"主体的角色清成非法值。
type PartnerRoleInput = *int8

// PartnerEntityTypeInput 是主体类型的可选入参：0 公司 / 1 个人。
// nil 表示不提交，新建时按公司处理，编辑时保持原值。
type PartnerEntityTypeInput = *int8

// PartnerQuery 是商业伙伴列表的查询条件。
type PartnerQuery struct {
	PageQuery
	Role       PartnerRoleInput       `form:"role"`
	EntityType PartnerEntityTypeInput `form:"entity_type"`
	Country    string                 `form:"country"`
	Status     EnableStatusInput      `form:"status"`
	// IncludeDeleted 把已软删的主体也列出来（「含已删除」开关），这些行只能做一件事：恢复。
	IncludeDeleted bool `form:"include_deleted"`
	// OwnerID 只看某人负责的主体（主管可用它下钻某个业务员）。
	// 业务员传了也会被自己的 id 覆盖——范围不是前端说了算。
	OwnerID *uint `form:"owner_id"`
}

// PartnerDuplicateQuery 是"这条记录会不会是已有的某一家"的查询条件。
//
// 四个字段与新建表单一一对应：录入时随输入异步查一次，只提示不阻断。
// ExcludeID 供编辑时排除自身，否则每条记录都会命中自己。
type PartnerDuplicateQuery struct {
	Name      string `form:"name"`
	Country   string `form:"country"`
	Email     string `form:"email"`
	Phone     string `form:"phone"`
	ExcludeID uint   `form:"exclude_id"`
}

// PartnerOwnerInput 是转移负责人的入参。
type PartnerOwnerInput struct {
	// OwnerID 是接手的人（sys_user.id）；0 表示退回**未分配**（公共池）。
	OwnerID uint `json:"owner_id"`
	// UpdatedAt 是打开页面时的版本，走乐观锁：先被人改了负责人就别覆盖。
	UpdatedAt *time.Time `json:"updated_at"`
}

// PartnerMergeInput 是一次主体合并的入参。
//
// 路径上的 :id 是**主记录**（保留的那条），SourceID 是被并入并软删的那条。
// 方向写死在 URL 里而不是靠两个 id 的先后顺序：合并不可逆，
// "哪条留下"必须是显式的，不能靠调用方记顺序。
type PartnerMergeInput struct {
	SourceID uint `json:"source_id" binding:"required"`
	// UpdatedAt 是主记录打开时的版本，用于条件更新；期间被别人改过则 409。
	UpdatedAt *time.Time `json:"updated_at"`
}

// PartnerMergeResult 是合并的结果。
type PartnerMergeResult struct {
	// Partner 是合并后的主记录（角色已并集、空字段已补齐）。
	Partner *model.Partner `json:"partner"`
	// Absorbed 是被并入并软删的那条记录，供界面回显"已并入 CUS002"。
	Absorbed *model.Partner `json:"absorbed"`
	// Moved 是各表被迁移的引用条数。合并不可逆，至少要能说清动过哪些表。
	Moved map[string]int64 `json:"moved"`
}

// PartnerInput 是主体新增与编辑共用的入参。
//
// 编辑走全量覆盖（PUT 语义），但 Role 与 EntityType 例外：
// 它们为 nil 时表示"这次不改"，而不是"改成零值"。
type PartnerInput struct {
	Code        string                 `json:"code" binding:"required,max=32"`
	Name        string                 `json:"name" binding:"required,max=128"`
	Role        PartnerRoleInput       `json:"role"`
	EntityType  PartnerEntityTypeInput `json:"entity_type"`
	Country     string                 `json:"country" binding:"max=64"`
	ContactName string                 `json:"contact" binding:"max=64"`
	Phone       string                 `json:"phone" binding:"max=32"`
	Email       string                 `json:"email" binding:"omitempty,email,max=128"`
	Address     string                 `json:"address" binding:"max=255"`
	Status      EnableStatusInput      `json:"status"`
	Remark      string                 `json:"remark" binding:"max=255"`

	// 商务条件全部用指针：nil 表示"这次不改"，与 Role / EntityType 同一套规则。
	//
	// 这一条不是洁癖：客户与供应商共用一行记录，供应商页面没有这些字段。
	// 若把 nil 当成零值写库，在供应商页面保存一次就会把这家的贸易术语与账期
	// 全部清空——而那个页面上根本没有可清空的输入框。
	TradeTerm        *string        `json:"trade_term"`
	SettlementMethod *string        `json:"settlement_method"`
	CreditDays       *int           `json:"credit_days"`
	CreditLimit      *money.Decimal `json:"credit_limit"`
	CurrencyCode     *string        `json:"currency_code"`
	// UpdatedAt 是编辑页打开时读到的版本；带上它做条件更新，期间被别人改过则 409。
	UpdatedAt *time.Time `json:"updated_at"`
}
