package dto

// WarehouseInput 是仓库的建单 / 改单入参（T-56）。
//
// 没有"停用也要传"的必填项：仓库是主数据，改一个名字不该逼人重填全部字段。
type WarehouseInput struct {
	Code      string `json:"code" binding:"required,max=32"`
	Name      string `json:"name" binding:"required,max=64"`
	Address   string `json:"address" binding:"max=255"`
	IsDefault bool   `json:"is_default"`
	Remark    string `json:"remark" binding:"max=255"`
}
