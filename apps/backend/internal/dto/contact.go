package dto

// ContactQuery 是联系人列表的查询条件。
//
// PartnerID 是主要维度：联系人几乎总是"按主体看"的，
// 不分主体地列全部联系人是通讯录，不是 CRM。
type ContactQuery struct {
	PageQuery
	PartnerID *uint `form:"partner_id"`
	Status    *int8 `form:"status"`
}

// ContactInput 新建与编辑共用。
//
// PartnerID 在编辑时不生效（见 service.ContactService.Update）：
// 把联系人挪到另一个主体，会让他的历史跟进挂到别人名下，
// 而跟进记录本身并不知道这件事。
type ContactInput struct {
	PartnerID uint   `json:"partner_id" binding:"required"`
	Name      string `json:"name" binding:"required,max=64"`
	Title     string `json:"title" binding:"max=64"`
	Phone     string `json:"phone" binding:"max=32"`
	Email     string `json:"email" binding:"omitempty,email,max=128"`
	Language  string `json:"language" binding:"max=32"`
	Channel   string `json:"channel" binding:"max=32"`
	// IsPrimary 用指针：nil 表示"这次不改主联系人身份"，与主体上的 role 同一套规则。
	IsPrimary *bool             `json:"is_primary"`
	Status    EnableStatusInput `json:"status"`
	Remark    string            `json:"remark" binding:"max=255"`
}
