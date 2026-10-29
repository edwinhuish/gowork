package dto

// PartnerAddressInput 是地址新增与编辑共用的入参。
//
// 不用指针：地址是独立实体，编辑走全量覆盖（PUT 语义），
// 与主体上那些"共用一行、供应商页面不显示"的字段处境不同——
// 地址只有地址这一个表单能改，不存在"另一个页面把它清空"的风险。
type PartnerAddressInput struct {
	// Type 只接受 invoice / delivery / consignee / notify。
	Type string `json:"type" binding:"required,max=16"`
	// IsDefault 置真时，同主体同用途的其它地址会被取消默认（同一事务）。
	IsDefault  bool   `json:"is_default"`
	Contact    string `json:"contact" binding:"max=64"`
	Phone      string `json:"phone" binding:"max=32"`
	Country    string `json:"country" binding:"max=64"`
	Province   string `json:"province" binding:"max=64"`
	City       string `json:"city" binding:"max=64"`
	PostalCode string `json:"postal_code" binding:"max=16"`
	// Address 必填：只有国家没有街道的地址寄不到货，也印不满单证。
	Address string `json:"address" binding:"required,max=255"`
	Remark  string `json:"remark" binding:"max=255"`
}

// PartnerAddressQuery 是地址列表的筛选条件。
type PartnerAddressQuery struct {
	// Type 为空表示不筛；地址通常只有几条，界面也很少按类型筛，
	// 保留它主要是给单证取址时按类型精确查。
	Type string `form:"type"`
}
