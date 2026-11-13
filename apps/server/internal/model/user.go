package model

// 系统角色。骨架期用固定枚举，后续可换成 role / permission 表。
//
// 三个角色只差两件事：能不能看全部数据、能不能进系统设置类的接口。
//   - admin  ：全部数据 + 系统设置（用户、审计日志、主体合并这类不可逆操作）
//   - manager：全部数据，但系统设置仍归 admin
//   - staff  ：只有自己负责（或未分配）的数据
//
// 不做字段级权限（T-36）：那要在响应里藏字段，与"谁看哪些行"是两回事。
const (
	RoleAdmin string = "admin"
	// RoleManager 是主管：可见范围与管理员相同，用来表达"业务负责人"这一层，
	// 而不是把主管也提成管理员——管理员能改系统设置，主管不该能。
	RoleManager string = "manager"
	RoleStaff   string = "staff"
)

// RoleSeesAllData 报告这个角色能否看到全部数据。
//
// 认不出的角色一律按"只看自己的"处理：角色是字符串，写错一个字母不会报错，
// 而把它当成"能看全部"就等于一次拼写错误变成了越权。
func RoleSeesAllData(role string) bool {
	switch role {
	case RoleAdmin, RoleManager:
		return true
	default:
		return false
	}
}

// RoleIsValid 校验角色取值，供用户维护接口在写入前拦下。
func RoleIsValid(role string) bool {
	switch role {
	case RoleAdmin, RoleManager, RoleStaff:
		return true
	default:
		return false
	}
}

// User 系统账号。
type User struct {
	Base
	// 遮蔽 Base 里的同名字段，把它排进「未删才唯一」的复合索引，见 Product 的说明。
	DeletedAt DeletedAt `gorm:"softDelete:milli;not null;default:0;uniqueIndex:uni_user_username,priority:2" json:"deleted_at,omitempty"`
	Username  string    `gorm:"size:64;uniqueIndex:uni_user_username,priority:1;not null" json:"username"`
	// Password 存 bcrypt 摘要，任何情况下都不出口。
	Password string       `gorm:"size:128;not null" json:"-"`
	Nickname string       `gorm:"size:64" json:"nickname"`
	Role     string       `gorm:"size:32;not null;default:staff;index" json:"role"`
	Status   EnableStatus `gorm:"not null;default:1;index" json:"status"`
}

func (User) TableName() string { return "sys_user" }

// IsEnabled 报告账号是否可用。
//
// 用 Enabled 而不是 Active 命名：Active 在本项目里曾被概览指标用过（在售），
// 与启用状态是同一件事的两种说法，留着会让人以为是两个不同的判断。
func (u *User) IsEnabled() bool { return u.Status == EnableStatusOn }
