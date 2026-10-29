package dto

import (
	"time"

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
	TradeTerm        *string  `json:"trade_term"`
	SettlementMethod *string  `json:"settlement_method"`
	CreditDays       *int           `json:"credit_days"`
	CreditLimit      *money.Decimal `json:"credit_limit"`
	CurrencyCode     *string        `json:"currency_code"`
	// UpdatedAt 是编辑页打开时读到的版本；带上它做条件更新，期间被别人改过则 409。
	UpdatedAt *time.Time `json:"updated_at"`
}
