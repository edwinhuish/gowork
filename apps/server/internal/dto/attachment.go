package dto

// AttachmentQuery 列出某个主体或单据的附件。
//
// 两个条件都必填：附件是全库通用的一张表，不带 entity 的查询等于
// 把所有人的提单摊在同一个响应里。
type AttachmentQuery struct {
	EntityType string `form:"entity_type" binding:"required"`
	EntityID   uint   `form:"entity_id" binding:"required"`
}

// AttachmentInput 是上传时随文件一起提交的表单字段。
//
// 没有文件名与大小：那两个值从 multipart 的 part 里取，由服务端嗅探与计数，
// 让客户端声明等于让它自己决定"我多大、我是什么类型"。
type AttachmentInput struct {
	EntityType string `form:"entity_type" binding:"required"`
	EntityID   uint   `form:"entity_id" binding:"required"`
	Remark     string `form:"remark" binding:"max=255"`
}
