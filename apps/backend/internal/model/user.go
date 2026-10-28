package model

// 系统角色。骨架期用固定枚举，后续可换成 role / permission 表。
const (
	RoleAdmin string = "admin"
	RoleStaff string = "staff"
)

// User 系统账号。
type User struct {
	Base
	Username string `gorm:"size:64;uniqueIndex;not null" json:"username"`
	// Password 存 bcrypt 摘要，任何情况下都不出口。
	Password string `gorm:"size:128;not null" json:"-"`
	Nickname string `gorm:"size:64" json:"nickname"`
	Role     string `gorm:"size:32;not null;default:staff;index" json:"role"`
	Status   int8   `gorm:"not null;default:1;index" json:"status"`
}

func (User) TableName() string { return "sys_user" }

func (u *User) IsActive() bool { return u.Status == StatusEnabled }
