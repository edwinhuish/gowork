package dto

type CustomerQuery struct {
	PageQuery
	Country string `form:"country"`
	Status  *int8  `form:"status"`
}

type CustomerInput struct {
	Code    string `json:"code" binding:"required,max=32"`
	Name    string `json:"name" binding:"required,max=128"`
	Country string `json:"country" binding:"max=64"`
	Contact string `json:"contact" binding:"max=64"`
	Phone   string `json:"phone" binding:"max=32"`
	Email   string `json:"email" binding:"omitempty,email,max=128"`
	Address string `json:"address" binding:"max=255"`
	Status  *int8  `json:"status"`
	Remark  string `json:"remark" binding:"max=255"`
}
