package repository

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/errs"
)

// AttachmentFilter 是附件列表条件。两个字段总是一起用：
// 只有"某个主体 / 某张单据的附件"这种问法，没有"全库附件"。
type AttachmentFilter struct {
	EntityType string
	EntityID   uint
}

type AttachmentRepository interface {
	// Create 在一个事务里写元数据与内容：只写进一半，列表里就会出现一条打不开的附件。
	Create(ctx context.Context, m *model.Attachment, content []byte) error
	FindByID(ctx context.Context, id uint) (*model.Attachment, error)
	// FindContent 单独取内容。与 FindByID 分开，是为了让列表接口不可能顺手带出字节。
	FindContent(ctx context.Context, id uint) ([]byte, error)
	List(ctx context.Context, f AttachmentFilter) ([]model.Attachment, error)
	// LatestImages 一次取回一批实体各自**最新的一张图片**附件。
	//
	// 存在的理由与产品主图那次一样：列表页要给 20 个主体显示 Logo，
	// 逐条 List 就是 20 次往返，而调用方手里已经有整页 id。
	LatestImages(ctx context.Context, entityType string, entityIDs []uint) (map[uint]model.Attachment, error)
	Delete(ctx context.Context, id uint) error
	// EntityExists 报告附件要挂的东西是否存在；未知类型返回 false。
	EntityExists(ctx context.Context, entityType string, id uint) (bool, error)
}

type attachmentRepo struct {
	*Base[model.Attachment]
}

func NewAttachmentRepository(db *gorm.DB) AttachmentRepository {
	return &attachmentRepo{Base: NewBase[model.Attachment](db)}
}

// attachTargets 是「附件能挂在哪」的唯一清单：类型字符串 → 模型实例。
//
// 用模型而不是表名：表名会随重构改，模型指针是编译期认的。
// 用一条 Count 判存在，比给十来张表各接一个仓储轻得多；
// 模型带软删除时 gorm 会自动过滤掉已删记录，挂到已删单据上的附件也就写不进去。
var attachTargets = map[string]any{
	model.AttachPartner:            &model.Partner{},
	model.AttachProduct:            &model.Product{},
	model.AttachQuotation:          &model.Quotation{},
	model.AttachContract:           &model.Contract{},
	model.AttachSalesOrder:         &model.SalesOrder{},
	model.AttachPurchaseOrder:      &model.PurchaseOrder{},
	model.AttachInspection:         &model.Inspection{},
	model.AttachShipment:           &model.Shipment{},
	model.AttachCustomsDeclaration: &model.CustomsDeclaration{},
	model.AttachReceivable:         &model.Receivable{},
	model.AttachPayable:            &model.Payable{},
	model.AttachPayment:            &model.Payment{},
	model.AttachStockDocument:      &model.StockDocument{},
}

func (r *attachmentRepo) Create(ctx context.Context, m *model.Attachment, content []byte) error {
	err := r.conn(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(m).Error; err != nil {
			return fmt.Errorf("保存附件: %w", err)
		}
		row := &model.AttachmentContent{AttachmentID: m.ID, Content: content}
		if err := tx.Create(row).Error; err != nil {
			return fmt.Errorf("保存附件内容: %w", err)
		}
		return nil
	})
	return err
}

func (r *attachmentRepo) FindByID(ctx context.Context, id uint) (*model.Attachment, error) {
	var m model.Attachment
	err := r.conn(ctx).First(&m, id).Error
	if err == gorm.ErrRecordNotFound {
		return nil, errs.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("查询附件 id=%d: %w", id, err)
	}
	return &m, nil
}

func (r *attachmentRepo) FindContent(ctx context.Context, id uint) ([]byte, error) {
	var row model.AttachmentContent
	err := r.conn(ctx).
		Where("attachment_id = ?", id).
		Take(&row).Error
	if err == gorm.ErrRecordNotFound {
		// 元数据在、内容不在，只可能是写到一半或数据被手工改过。
		return nil, fmt.Errorf("附件 id=%d 的内容缺失", id)
	}
	if err != nil {
		return nil, fmt.Errorf("查询附件内容 id=%d: %w", id, err)
	}
	return row.Content, nil
}

func (r *attachmentRepo) List(ctx context.Context, f AttachmentFilter) ([]model.Attachment, error) {
	list := make([]model.Attachment, 0, 8)
	err := r.conn(ctx).
		Where("entity_type = ? AND entity_id = ?", f.EntityType, f.EntityID).
		// 新的在上：单据页面里刚传完的那份应当第一眼看到。
		Order("id DESC").
		Find(&list).Error
	if err != nil {
		return nil, fmt.Errorf("查询附件列表 %s=%d: %w", f.EntityType, f.EntityID, err)
	}
	return list, nil
}

// LatestImages 见接口注释。返回 key 是 entity_id，没有图片的主体不在 map 里。
func (r *attachmentRepo) LatestImages(
	ctx context.Context, entityType string, entityIDs []uint,
) (map[uint]model.Attachment, error) {
	out := make(map[uint]model.Attachment, len(entityIDs))
	if len(entityIDs) == 0 {
		return out, nil
	}

	rows := make([]model.Attachment, 0, len(entityIDs))
	// 只碰元数据表，字节在 sys_attachment_content 里，这里取不到也不需要。
	err := r.conn(ctx).
		Where("entity_type = ? AND entity_id IN ? AND mime_type IN ?",
			entityType, entityIDs, model.AttachmentImageTypes).
		Order("id DESC").
		Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("查询 %s 的图片附件: %w", entityType, err)
	}

	// id DESC 之后每个实体第一次出现即为最新那张，其余跳过。
	// 在 Go 里去重而不是写窗口函数：三方方言的写法各不相同，
	// 而一个主体的附件通常只有几条，多拉这几十行比多运维一份 SQL 划算。
	for _, row := range rows {
		if _, seen := out[row.EntityID]; seen {
			continue
		}
		out[row.EntityID] = row
	}
	return out, nil
}

// Delete 只软删元数据，内容留着。
//
// 追溯要能拿到原文件：删错了还能捞回来，而 10MB 上限下留着这点空间的代价
// 远小于"报关单扫描件点了删除就再也找不着"。
func (r *attachmentRepo) Delete(ctx context.Context, id uint) error {
	return r.Base.DeleteByID(ctx, id)
}

func (r *attachmentRepo) EntityExists(
	ctx context.Context, entityType string, id uint,
) (bool, error) {
	target, ok := attachTargets[entityType]
	if !ok {
		return false, nil
	}
	var n int64
	err := r.conn(ctx).Model(target).Where("id = ?", id).Count(&n).Error
	if err != nil {
		return false, fmt.Errorf("查询附件目标 %s=%d: %w", entityType, id, err)
	}
	return n > 0, nil
}
