package model

// Clause 是可复用的**合同条款模板**（主数据）。
//
// 名字要小心：`ContractTerm`（biz_contract_term）已经是**付款分期**（30% 订金、
// 70% 见提单副本），不是文本条款。所以条款库用 `md_clause`，
// 合同上引用它得到的是 `biz_contract_clause`，两套东西不共用一张表。
//
// 前缀按仓库约定：md_ 主数据、biz_ 业务单据。
type Clause struct {
	Base
	// 遮蔽 Base 里的同名字段，把它排进「未删才唯一」的复合索引，见 Product 的说明。
	DeletedAt DeletedAt `gorm:"softDelete:milli;not null;default:0;uniqueIndex:uni_clause_code,priority:2" json:"deleted_at,omitempty"`
	Code      string    `gorm:"size:32;uniqueIndex:uni_clause_code,priority:1;not null" json:"code"`
	Title     string    `gorm:"size:128;not null" json:"title"`
	// Body 是条款正文。用 text 而不是定长：质量异议期这类条款能写到几百字，
	// 截断一次就意味着这句话在法律上不完整。
	Body string `gorm:"type:text;not null" json:"body"`
	// Category 只用来分类（付款 / 质量 / 违约 / 其他），不做枚举校验：
	// 条款的类别是业务自己长出来的，写死一组值迟早不够用。
	Category string `gorm:"size:32;index" json:"category"`
	Remark   string `gorm:"size:255" json:"remark"`
}

func (Clause) TableName() string { return "md_clause" }

// ContractClause 是合同上的一条条款**快照**。
//
// 为什么不是只存 ClauseID：模板改了，已经签出去的合同不能跟着变。
// 所以生成合同时把标题与正文抄一份存下来，ClauseID 只用来回答
// "这条当初是从哪个模板来的"（模板可能后来被停用甚至删掉）。
type ContractClause struct {
	Base
	ContractID uint `gorm:"index;not null" json:"contract_id"`
	// ClauseID 是来源模板，可空：手写的条款没有模板。
	ClauseID *uint `gorm:"index" json:"clause_id"`
	// Seq 是展示顺序，从 1 开始：条款顺序影响阅读，也影响"以第几条为准"的解释。
	Seq   int    `gorm:"not null;default:1" json:"seq"`
	Title string `gorm:"size:128;not null" json:"title"`
	Body  string `gorm:"type:text;not null" json:"body"`
}

func (ContractClause) TableName() string { return "biz_contract_clause" }
