package dto

// UserInput 是建号入参。
//
// 用户名与密码只在建号时提交：用户名是登录凭据，改了等于换了个人；
// 密码走单独的字段，编辑时不传表示「不改」。
type UserInput struct {
	Username string `json:"username" binding:"required,min=3,max=64"`
	// Password 上限 72 是 bcrypt 的硬限制：超出的部分会被静默截断，
	// 于是「密码 100 位」的人改前 72 位以外都能登录。
	Password string `json:"password" binding:"required,min=6,max=72"`
	Nickname string `json:"nickname" binding:"max=64"`
	Role     string `json:"role" binding:"required,max=32"`
}

// UserUpdateInput 是账号维护入参，三组字段都可选。
//
// 全用指针：nil 表示「这次不改」。用零值语义的话，编辑时只想改昵称
// 会把角色清空、把账号停用——这类字段一旦被误清，人第二天就进不来。
type UserUpdateInput struct {
	Nickname *string `json:"nickname" binding:"omitempty,max=64"`
	Role     *string `json:"role" binding:"omitempty,max=32"`
	// Status 只有 0 停用与 1 启用两个取值。
	Status *int8 `json:"status" binding:"omitempty,oneof=0 1"`
	// Password 留空表示不改；改了之后该账号已签发的令牌全部作废。
	Password *string `json:"password" binding:"omitempty,min=6,max=72"`
}
