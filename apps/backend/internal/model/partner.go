package model

import (
	"fmt"
	"strings"

	"github.com/edwinhuish/gowork/internal/pkg/money"
)

// PartnerRole 是商业伙伴在业务中承担的角色，按位定义。
//
// 用位掩码而不是 1=客户 / 2=供应商 / 3=两者 这样的连续枚举，原因是角色天然是集合：
// 外贸里同一个主体既卖原料给你、又买你的成品，是很常见的（"客供两用"）。
// 连续枚举在引入第三种角色（货代、报关行）时会组合爆炸 —— 3 位角色需要
// 7 个枚举值，而且每加一种角色都要回头改所有判断分支；位掩码只加一个常量。
type PartnerRole int8

const (
	// PartnerRoleCustomer 客户：向我方采购的主体。
	PartnerRoleCustomer PartnerRole = 1 << iota // 1
	// PartnerRoleSupplier 供应商：向我方供货的主体。
	PartnerRoleSupplier // 2
	// PartnerRoleForwarder 货代 / 报关行：替我方向承运人订舱、报关的主体。
	//
	// 它复用的是同一张 md_partner，而不是新建货代表：货代的联系方式、国家、
	// 联系人字段与供应商几乎一样，唯一差别是它不参与采购与报价。
	// 这也是当初选位掩码的直接收益 —— 连续枚举下这里要把 3 插成 7。
	PartnerRoleForwarder // 4
)

// AllPartnerRoles 是已定义角色的位全集。新增角色时只改这一行，
// 合法性校验与查询展开都会跟着覆盖。
const AllPartnerRoles = PartnerRoleCustomer | PartnerRoleSupplier | PartnerRoleForwarder

// roleNames 是各角色位的展示名，按位从低到高排列。
//
// 单独列表而不是在 String 里写 switch：switch 需要枚举"客户+供应商"这类组合，
// 三位角色就有 7 种，加第四位是 15 种；按位拼接只需要给新角色补一行。
var roleNames = []struct {
	bit  PartnerRole
	name string
}{
	{PartnerRoleCustomer, "customer"},
	{PartnerRoleSupplier, "supplier"},
	{PartnerRoleForwarder, "forwarder"},
}

// Has 报告角色集合是否包含目标的全部位。
// target 传单个角色时等价于"是否具备该角色"。
func (r PartnerRole) Has(target PartnerRole) bool {
	return target != 0 && r&target == target
}

// IsValid 拒绝零值与未定义的位。
//
// 零值不合法是有意的：角色决定这个主体出现在哪个业务的候选列表里，
// 一个什么角色都没有的主体没有任何入口可以访问到，属于脏数据。
func (r PartnerRole) IsValid() bool {
	return r != 0 && r&^AllPartnerRoles == 0
}

// String 让日志和错误信息里能直接看到角色名，而不是一个裸数字。
//
// 零值与含未定义位的值都退化成数字：前者是脏数据，后者说明有人写进了
// 一个还没在这里登记的角色位，打印出 role(8) 比打印 "customer" 更容易定位。
func (r PartnerRole) String() string {
	if r == 0 || r&^AllPartnerRoles != 0 {
		return fmt.Sprintf("role(%d)", int8(r))
	}

	parts := make([]string, 0, len(roleNames))
	for _, n := range roleNames {
		if r.Has(n.bit) {
			parts = append(parts, n.name)
		}
	}
	return strings.Join(parts, "+")
}

// RoleValuesContaining 列出所有包含目标角色的取值，供 `role IN (...)` 查询。
//
// 直接写 `role & ? <> 0` 在 PostgreSQL 与 MySQL 上都无法命中 role 上的 btree 索引，
// 而"只看供应商"是列表页的默认条件，这条查询必须走索引。展开成有限枚举值后，
// 它就退化成了普通的 IN 查询，三库行为一致。
func RoleValuesContaining(target PartnerRole) []PartnerRole {
	// 取全 1 的掩码作为循环上限，而不是直接用 AllPartnerRoles。
	// 后者只有在角色位连续无空缺时才等于全 1，一旦将来跳着定义（如省掉 supplier
	// 直接加 4），上限会漏掉那些以最高位为组成部分的取值。
	ceiling := PartnerRole(1)
	for ceiling < AllPartnerRoles {
		ceiling = ceiling<<1 | 1
	}

	values := make([]PartnerRole, 0, int(ceiling))
	for v := PartnerRole(1); v <= ceiling; v++ {
		if v.Has(target) {
			values = append(values, v)
		}
	}
	return values
}

// PartnerEntityType 区分主体是法人还是自然人。
//
// 零值即公司，而不是反过来。两个理由：现有数据全部是企业，迁移时不必回填；
// 外贸场景里新增主体也以公司为主。若把"个人"设成零值，
// "忘记填"和"明确选了个人"就无法区分了。
type PartnerEntityType int8

const (
	// PartnerEntityCompany 公司 / 工厂 / 店铺等法人主体。
	PartnerEntityCompany PartnerEntityType = 0
	// PartnerEntityPerson 个人：个体户、货代个人、私账收款的买手。
	// 报关资料、发票抬头与公司不同，所以要单独标记而不是塞进备注。
	PartnerEntityPerson PartnerEntityType = 1
)

// IsValid 拒绝未定义的取值。
func (t PartnerEntityType) IsValid() bool {
	return t == PartnerEntityCompany || t == PartnerEntityPerson
}

// Partner 是商业伙伴（往来单位）：客户与供应商统一在这一张表里。
//
// 命名用 Partner 而不是 Party：Party 在合同语境里指"当事方"，日常又常被
// 读成"聚会"，拿来当主数据实体名两头都不贴切；Partner 才是"业务伙伴"的
// 行业惯用说法，也与 commercial partner 这个业务概念对得上。
//
// 原先 md_customer 与 md_supplier 两张表字段几乎相同，只有客户多一个 country。
// 分开建表的代价在"同一个主体既是客户又是供应商"时立刻显现：两行数据、
// 两份联系人、两套编码，对账时还要人工确认它们是同一家公司。合并后靠 role 区分，
// 一笔业务引用的是同一个 id，往来余额与对账可以直接按主体归集。
//
// 归属：共享主数据（md_ 前缀）。ERP 拿它做交易对手，CRM 拿它做关系主体，
// 两侧引用同一行，不做同步。
type Partner struct {
	Base
	// 遮蔽 Base 里的同名字段，把它排进「未删才唯一」的复合索引，见 Product 的说明。
	DeletedAt DeletedAt `gorm:"softDelete:milli;not null;default:0;uniqueIndex:uni_partner_code,priority:2" json:"deleted_at,omitempty"`
	Code      string    `gorm:"size:32;uniqueIndex:uni_partner_code,priority:1;not null" json:"code"`
	Name      string    `gorm:"size:128;not null" json:"name"`
	// Role 决定这个主体出现在哪些业务的候选列表里，至少一位。
	Role PartnerRole `gorm:"not null;index" json:"role"`
	// EntityType 区分公司与个人。
	EntityType PartnerEntityType `gorm:"not null;index" json:"entity_type"`
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

	// 以下五项是商务条件，主要对客户角色有意义（供应商通常另有采购协议）。
	//
	// 放在主体上而不是每张订单上：它们是"这家客户一贯怎么结算"的默认值，
	// 每张订单再各自覆盖。反过来（只存在订单上）会让新建订单的人每次都要重填，
	// 而重填的账期与信用额度迟早会跟约定的不一致。
	TradeTerm        TradeTerm        `gorm:"size:8" json:"trade_term"`
	SettlementMethod SettlementMethod `gorm:"size:8" json:"settlement_method"`
	// CreditDays 账期天数：从出货或开票起算，多少天内应当收到款。
	CreditDays int `gorm:"not null;default:0" json:"credit_days"`
	// CreditLimit 信用额度，用本币计。0 表示不限（不是"没有额度"）——
	// 绝大多数客户不设额度，把 0 解读成"一分不给"会让系统默认拒绝一切订单。
	CreditLimit money.Decimal `gorm:"type:decimal(14,2);not null;default:0" json:"credit_limit"`
	// CurrencyCode 默认结算币种，空表示按报价单或本币。
	CurrencyCode string `gorm:"size:8" json:"currency_code"`

	// OwnerID 是负责这家的业务员（sys_user.id）。0 表示**未分配**：
	// 刚上线的存量数据、以及"先录进来还没分人"的客户都在这一堆里。
	//
	// 归属只存在主体上、不复制到每张单据：客户换了负责人，历史和未来的单据一起跟着走，
	// 不会出现"订单归属张三、客户归属李四"这种必须靠人解释的状态。
	OwnerID uint `gorm:"index;not null;default:0" json:"owner_id"`
	// OwnerName 是列表 join 出来的负责人登录名。
	//
	// 用 `->;-:migration` 而不是 `gorm:"-"`：前者是"只读、不建列"，
	// 后者会让 gorm 完全忽略这个字段，join 出来的值扫不进来（表现为永远空串）。
	OwnerName string `gorm:"->;-:migration" json:"owner_name,omitempty"`
}

func (Partner) TableName() string { return "md_partner" }
