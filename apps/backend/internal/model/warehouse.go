package model

// Warehouse 是存放地点（T-56 第一步）。
//
// 在此之前库存只有一个池子：备货的货与直发的货混在一起，
// "这个 SKU 还能卖多少"算不准，调拨也只能返回 400。
//
// **这一步只加了维度，没有改唯一键**：快照的唯一索引仍是 `(product_id, deleted_at)`，
// `warehouse_id` 先落成默认值，读写行为与单仓完全等价。改唯一索引要显式
// DROP + CREATE（gorm 的 AutoMigrate 不会重建已存在的索引，而 ON CONFLICT
// 必须匹配到一个真实存在的唯一约束），单独一步做，见 docs/architecture.md。
type Warehouse struct {
	Base
	// 遮蔽 Base 里的同名字段，把它排进「未删才唯一」的复合索引，见 Product 的说明。
	DeletedAt DeletedAt `gorm:"softDelete:milli;not null;default:0;uniqueIndex:uni_warehouse_code,priority:2" json:"deleted_at,omitempty"`
	Code      string    `gorm:"size:32;uniqueIndex:uni_warehouse_code,priority:1;not null" json:"code"`
	Name      string    `gorm:"size:64;not null" json:"name"`
	// Address 是提货 / 送仓地址，打印在出入库单上。
	Address string `gorm:"size:255" json:"address"`
	// IsDefault 标记默认仓库：新流水不指定仓库时落在这里。
	// 允许多条为 false，但只能有一条为 true（应用层事务保证，与默认地址同一套路）。
	IsDefault bool         `gorm:"not null;default:false;index" json:"is_default"`
	Status    EnableStatus `gorm:"not null;default:1;index" json:"status"`
	Remark    string       `gorm:"size:255" json:"remark"`
}

func (Warehouse) TableName() string { return "md_warehouse" }
