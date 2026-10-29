package model

// 系统角色。骨架期用固定枚举，后续可换成 role / permission 表。
const (
	RoleAdmin string = "admin"
	RoleStaff string = "staff"
)

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
