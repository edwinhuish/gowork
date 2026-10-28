package dto

type SupplierQuery struct {
	PageQuery
	Status EnableStatusInput `form:"status"`
}

type SupplierInput struct {
	Code        string            `json:"code" binding:"required,max=32"`
	Name        string            `json:"name" binding:"required,max=128"`
	ContactName string            `json:"contact" binding:"max=64"`
	Phone       string            `json:"phone" binding:"max=32"`
	Email       string            `json:"email" binding:"omitempty,email,max=128"`
	Address     string            `json:"address" binding:"max=255"`
	Status      EnableStatusInput `json:"status"`
	Remark      string            `json:"remark" binding:"max=255"`
}
