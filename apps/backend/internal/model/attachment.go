package model

// 附件挂载的实体类型。
//
// 用字符串而不是数字编号：附件表是全库通用的一张表，日志或客服截图里出现
// `entity_type=7` 谁也说不清那是出货单还是发票；字符串自解释，新增类型也不会撞号。
const (
	AttachPartner            = "partner"
	AttachProduct            = "product"
	AttachQuotation          = "quotation"
	AttachContract           = "contract"
	AttachSalesOrder         = "sales_order"
	AttachPurchaseOrder      = "purchase_order"
	AttachShipment           = "shipment"
	AttachCustomsDeclaration = "customs_declaration"
	AttachReceivable         = "receivable"
	AttachPayable            = "payable"
	AttachPayment            = "payment"
	AttachStockDocument      = "stock_document"
)

// Attachment 是挂在主体或单据上的文件元数据：提单、报关单扫描件、验货报告。
//
// 内容不在本表，见 AttachmentContent：列表页只读元数据，若字节跟着同一行，
// 打开一票货就等于把它的所有扫描件从库里拖一遍。
//
// 表名用 sys_ 前缀，与术语表一致：它不与任何业务对象一对一，是横切的归档能力。
type Attachment struct {
	Base
	EntityType string `gorm:"size:32;not null;index:idx_attachment_entity,priority:1" json:"entity_type"`
	EntityID   uint   `gorm:"not null;index:idx_attachment_entity,priority:2" json:"entity_id"`
	// FileName 是上传时的原名，下载时按它命名——客户认的是「提单.pdf」而不是一串 id。
	FileName string `gorm:"size:255;not null" json:"file_name"`
	// MimeType 是服务端嗅探的结果，不采信客户端声明：改一个请求头就能让 .exe 变成 image/png。
	MimeType string `gorm:"size:64;not null" json:"mime_type"`
	Size     int64  `gorm:"not null" json:"size"`
	// SHA256 用来回答「这份文件是不是同一份」，也方便排查传到一半的记录。
	SHA256 string `gorm:"size:64;index" json:"sha256"`
	// UploadedBy 是上传人；内部任务上传时为 0。
	UploadedBy uint   `gorm:"not null;default:0" json:"uploaded_by"`
	Remark     string `gorm:"size:255" json:"remark"`
}

func (Attachment) TableName() string { return "sys_attachment" }

// AttachmentContent 是附件内容，与元数据 1:1。
//
// 拆成两张表，是为了让「查列表」不可能顺手把字节拖出来：只要字节和元数据在同一个
// 结构体里，迟早会有人 Select("*")。分表之后，忘记带条件也拿不到内容。
//
// 存库而不是存磁盘：备份跟着数据库走，不会出现「库里有记录、目录里没文件」的漂移；
// 单文件上限 10MB，扫描件够用（上限见 service.maxAttachmentSize）。
type AttachmentContent struct {
	AttachmentID uint   `gorm:"primaryKey" json:"attachment_id"`
	Content      []byte `gorm:"not null" json:"-"`
}

func (AttachmentContent) TableName() string { return "sys_attachment_content" }
