package dto

// ProductImageFile 是一张待入库的图片：文件名与字节。
//
// 不写成 JSON 入参：两者都来自 multipart 的 part，客户端声明的名字可以改，
// 字节由服务端嗅探与计数，能进这个结构体时校验都过了。
type ProductImageFile struct {
	FileName string
	Content  []byte
}

// ProductImageOrderInput 是完整排列，ids 的先后即最终顺序。
//
// 不做「上移一格」：那需要客户端与库里的位置对齐，两个人同时拖动就会错位；
// 全量提交让服务端能一次判定集合是否一致，不一致直接 409。
type ProductImageOrderInput struct {
	IDs []uint `json:"ids" binding:"required,min=1"`
}
