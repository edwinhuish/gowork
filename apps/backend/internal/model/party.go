package model

import "fmt"

// PartyRole 是商业伙伴在业务中承担的角色，按位定义。
//
// 用位掩码而不是 1=客户 / 2=供应商 / 3=两者 这样的连续枚举，原因是角色天然是集合：
// 外贸里同一个主体既卖原料给你、又买你的成品，是很常见的（"客供两用"）。
// 连续枚举在引入第三种角色（货代、报关行）时会组合爆炸 —— 3 位角色需要
// 7 个枚举值，而且每加一种角色都要回头改所有判断分支；位掩码只加一个常量。
type PartyRole int8

const (
	// PartyRoleCustomer 客户：向我方采购的主体。
	PartyRoleCustomer PartyRole = 1 << iota // 1
	// PartyRoleSupplier 供应商：向我方供货的主体。
	PartyRoleSupplier // 2
)

// AllPartyRoles 是已定义角色的位全集。新增角色时只改这一行，
// 合法性校验与查询展开都会跟着覆盖。
const AllPartyRoles = PartyRoleCustomer | PartyRoleSupplier

// Has 报告角色集合是否包含目标的全部位。
// target 传单个角色时等价于"是否具备该角色"。
func (r PartyRole) Has(target PartyRole) bool {
	return target != 0 && r&target == target
}

// IsValid 拒绝零值与未定义的位。
//
// 零值不合法是有意的：角色决定这个主体出现在哪个业务的候选列表里，
// 一个什么角色都没有的主体没有任何入口可以访问到，属于脏数据。
func (r PartyRole) IsValid() bool {
	return r != 0 && r&^AllPartyRoles == 0
}

// String 让日志和错误信息里能直接看到角色名，而不是一个裸数字。
func (r PartyRole) String() string {
	switch r {
	case PartyRoleCustomer:
		return "customer"
	case PartyRoleSupplier:
		return "supplier"
	case PartyRoleCustomer | PartyRoleSupplier:
		return "customer+supplier"
	default:
		return fmt.Sprintf("role(%d)", int8(r))
	}
}

// RoleValuesContaining 列出所有包含目标角色的取值，供 `role IN (...)` 查询。
//
// 直接写 `role & ? <> 0` 在 PostgreSQL 与 MySQL 上都无法命中 role 上的 btree 索引，
// 而"只看供应商"是列表页的默认条件，这条查询必须走索引。展开成有限枚举值后，
// 它就退化成了普通的 IN 查询，三库行为一致。
func RoleValuesContaining(target PartyRole) []PartyRole {
	// 取全 1 的掩码作为循环上限，而不是直接用 AllPartyRoles。
	// 后者只有在角色位连续无空缺时才等于全 1，一旦将来跳着定义（如省掉 supplier
	// 直接加 4），上限会漏掉那些以最高位为组成部分的取值。
	ceiling := PartyRole(1)
	for ceiling < AllPartyRoles {
		ceiling = ceiling<<1 | 1
	}

	values := make([]PartyRole, 0, int(ceiling))
	for v := PartyRole(1); v <= ceiling; v++ {
		if v.Has(target) {
			values = append(values, v)
		}
	}
	return values
}

// PartyEntityType 区分主体是法人还是自然人。
//
// 零值即公司，而不是反过来。两个理由：现有数据全部是企业，迁移时不必回填；
// 外贸场景里新增主体也以公司为主。若把"个人"设成零值，
// "忘记填"和"明确选了个人"就无法区分了。
type PartyEntityType int8

const (
	// PartyEntityCompany 公司 / 工厂 / 店铺等法人主体。
	PartyEntityCompany PartyEntityType = 0
	// PartyEntityPerson 个人：个体户、货代个人、私账收款的买手。
	// 报关资料、发票抬头与公司不同，所以要单独标记而不是塞进备注。
	PartyEntityPerson PartyEntityType = 1
)

// IsValid 拒绝未定义的取值。
func (t PartyEntityType) IsValid() bool {
	return t == PartyEntityCompany || t == PartyEntityPerson
}

// Party 是商业伙伴（往来单位）：客户与供应商统一在这一张表里。
//
// 原先 md_customer 与 md_supplier 两张表字段几乎相同，只有客户多一个 country。
// 分开建表的代价在"同一个主体既是客户又是供应商"时立刻显现：两行数据、
// 两份联系人、两套编码，对账时还要人工确认它们是同一家公司。合并后靠 role 区分，
// 一笔业务引用的是同一个 id，往来余额与对账可以直接按主体归集。
//
// 归属：共享主数据（md_ 前缀）。ERP 拿它做交易对手，CRM 拿它做关系主体，
// 两侧引用同一行，不做同步。
type Party struct {
	Base
	Code string `gorm:"size:32;uniqueIndex;not null" json:"code"`
	Name string `gorm:"size:128;not null" json:"name"`
	// Role 决定这个主体出现在哪些业务的候选列表里，至少一位。
	Role PartyRole `gorm:"not null;index" json:"role"`
	// EntityType 区分公司与个人。
	EntityType PartyEntityType `gorm:"not null;index" json:"entity_type"`
	// Country 主要对客户有意义（外贸按目的国分组），供应商留空即可。
	Country string `gorm:"size:64" json:"country"`
	// ContactName 是主联系人姓名。完整的联系人（多个、各有电话邮箱）
	// 在 CRM 里是独立实体，届时 contact 列的语义就是"主联系人"。
	ContactName string `gorm:"size:64;column:contact" json:"contact"`
	Phone       string `gorm:"size:32" json:"phone"`
	Email       string `gorm:"size:128" json:"email"`
	Address     string `gorm:"size:255" json:"address"`
	// Status 刻意不写 gorm 的 default tag。
	//
	// gorm 对带 default 的字段会在值为零值时省略该列、让数据库填默认值，
	// 而 EnableStatusOff 恰好就是零值 —— 结果是"显式提交停用"被静默存成启用。
	// 不写 default，插入时总是带上实际值，由 service 负责 nil 的归一化。
	Status EnableStatus `gorm:"not null;index" json:"status"`
	Remark string       `gorm:"size:255" json:"remark"`
}

func (Party) TableName() string { return "md_party" }
