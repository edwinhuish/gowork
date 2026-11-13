package model

// ProductImage 是产品的一张图片。
//
// 为什么不复用 sys_attachment：附件是横切的归档能力，语义是「当时那份文件」，
// 删除要软删并保留字节以便追溯；而产品图片回答的是「这件货现在长什么样」，
// 需要顺序、主图与替换，两种语义混在同一张表上，两边都会被对方的规则绑住。
//
// 主图不落独立字段：**主图就是 sort_order 最小的那张**。设两个来源
// （顺序 + is_primary）迟早会出现「标记是第一张、顺序却在第三位」的记录，
// 界面上写「主图」的那张和列表里显示的那张不是同一张，且无法判断哪个是对的。
// IsPrimary 只是服务端按顺序算出来的响应字段，不落库。
type ProductImage struct {
	Base
	ProductID uint `gorm:"not null;index:idx_product_image_order,priority:1" json:"product_id"`
	// SortOrder 越小越靠前；删除留下的空档不回填，主图只取最小值。
	SortOrder int `gorm:"not null;default:0;index:idx_product_image_order,priority:2" json:"sort_order"`
	// FileName 是上传时的原名，只用于显示与下载；内容不进这条记录。
	FileName string `gorm:"size:255;not null" json:"file_name"`
	// MimeType 是服务端嗅探的结果，不采信客户端声明。
	MimeType string `gorm:"size:64;not null" json:"mime_type"`
	Size     int64  `gorm:"not null" json:"size"`
	// Width 与 Height 在入库前用 DecodeConfig 读出来：前端拿它预留占位，
	// 避免图片加载完成时整页跳动；也是「文件真的是图片」的判据之一。
	Width  int `gorm:"not null" json:"width"`
	Height int `gorm:"not null" json:"height"`
	// SHA256 用来做下载缓存的 ETag，也方便排查同一张图被重复上传。
	SHA256     string `gorm:"size:64;index" json:"sha256"`
	UploadedBy uint   `gorm:"not null;default:0" json:"uploaded_by"`

	// IsPrimary 是「这次响应里它是不是主图」，不落库：它由顺序推导。
	IsPrimary bool `gorm:"-" json:"is_primary"`
	// URL / ThumbURL 是取图路径，不落库：路由换了不该回填全表。
	// 带 v=updated_at 的版本号，替换内容后 URL 随之变化，前端缓存不会拿到旧图。
	URL      string `gorm:"-" json:"url,omitempty"`
	ThumbURL string `gorm:"-" json:"thumb_url,omitempty"`
}

func (ProductImage) TableName() string { return "md_product_image" }

// ProductImageContent 是图片字节，与元数据 1:1。
//
// 拆表与附件同一个理由：列表页只读元数据，把 5MB 原图放在同一行，
// 迟早有人 Select("*") 把一页产品的原图全拖出来。
//
// Thumb 是上传时生成的 400px 缩略图，列表与详情缩略图只取它；
// 原图只在点开预览时取。
type ProductImageContent struct {
	ImageID uint   `gorm:"primaryKey" json:"image_id"`
	Content []byte `gorm:"not null" json:"-"`
	Thumb   []byte `gorm:"not null" json:"-"`
}

func (ProductImageContent) TableName() string { return "md_product_image_content" }
