package repository

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/errs"
)

// ProductImageRepository 管产品图片：元数据、字节与顺序。
//
// 顺序用「全量重排」而不是「上移一格」：后者要先把相邻两张的序号读出来再写回去，
// 两个人同时拖动时必然撞在一起；全量提交时调用方给出完整排列，
// 仓储只负责按数组下标重写，谁也插不进来。
type ProductImageRepository interface {
	// Create 在一个事务里写元数据与字节：只写进一半，列表上就会出现一张打不开的图。
	Create(ctx context.Context, m *model.ProductImage, content, thumb []byte) error
	// ListByProduct 按 sort_order、id 升序返回，第一张即主图。
	ListByProduct(ctx context.Context, productID uint) ([]model.ProductImage, error)
	FindByID(ctx context.Context, id uint) (*model.ProductImage, error)
	// FindContent 只取字节。thumb=true 取缩略图，否则取原图。
	FindContent(ctx context.Context, id uint, thumb bool) ([]byte, error)
	// Replace 覆盖字节与元数据，不动 sort_order：替换是「换一个外观」，不是「换一个位置」。
	Replace(ctx context.Context, id uint, fields map[string]any, content, thumb []byte) error
	// Delete 软删元数据、物理删除字节：产品图不是凭证，长期留着只会让库持续膨胀。
	Delete(ctx context.Context, id uint) error
	// Reorder 按 ids 的下标重写 sort_order。调用方负责保证 ids 与库中集合一致。
	Reorder(ctx context.Context, productID uint, ids []uint) error
	CountByProduct(ctx context.Context, productID uint) (int64, error)
	// PrimaryByProducts 一次取出一批产品的主图，供产品列表避免逐条查询。
	PrimaryByProducts(ctx context.Context, productIDs []uint) (map[uint]model.ProductImage, error)
}

type productImageRepo struct {
	*Base[model.ProductImage]
}

func NewProductImageRepository(db *gorm.DB) ProductImageRepository {
	return &productImageRepo{Base: NewBase[model.ProductImage](db)}
}

func (r *productImageRepo) Create(
	ctx context.Context, m *model.ProductImage, content, thumb []byte,
) error {
	return r.conn(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(m).Error; err != nil {
			return fmt.Errorf("保存产品图片: %w", err)
		}
		row := &model.ProductImageContent{ImageID: m.ID, Content: content, Thumb: thumb}
		if err := tx.Create(row).Error; err != nil {
			return fmt.Errorf("保存产品图片内容: %w", err)
		}
		return nil
	})
}

func (r *productImageRepo) ListByProduct(
	ctx context.Context, productID uint,
) ([]model.ProductImage, error) {
	list := make([]model.ProductImage, 0, 8)
	err := r.conn(ctx).
		Where("product_id = ?", productID).
		// id 兜底：两次排序结果相同不该在刷新时换位置。
		Order("sort_order ASC, id ASC").
		Find(&list).Error
	if err != nil {
		return nil, fmt.Errorf("查询产品 %d 的图片: %w", productID, err)
	}
	return list, nil
}

func (r *productImageRepo) FindByID(
	ctx context.Context, id uint,
) (*model.ProductImage, error) {
	var m model.ProductImage
	err := r.conn(ctx).First(&m, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errs.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("查询产品图片 id=%d: %w", id, err)
	}
	return &m, nil
}

func (r *productImageRepo) FindContent(
	ctx context.Context, id uint, thumb bool,
) ([]byte, error) {
	column := "content"
	if thumb {
		column = "thumb"
	}
	var row model.ProductImageContent
	err := r.conn(ctx).
		Select("image_id", column).
		Where("image_id = ?", id).
		Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		// 元数据在、字节不在，只可能是写到一半或数据被手工改过。
		return nil, fmt.Errorf("产品图片 id=%d 的内容缺失", id)
	}
	if err != nil {
		return nil, fmt.Errorf("查询产品图片内容 id=%d: %w", id, err)
	}
	if thumb {
		return row.Thumb, nil
	}
	return row.Content, nil
}

func (r *productImageRepo) Replace(
	ctx context.Context, id uint, fields map[string]any, content, thumb []byte,
) error {
	return r.conn(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.ProductImage{}).Where("id = ?", id).
			Updates(fields).Error; err != nil {
			return fmt.Errorf("更新产品图片 id=%d: %w", id, err)
		}
		res := tx.Model(&model.ProductImageContent{}).Where("image_id = ?", id).
			Updates(map[string]any{"content": content, "thumb": thumb})
		if res.Error != nil {
			return fmt.Errorf("更新产品图片内容 id=%d: %w", id, res.Error)
		}
		if res.RowsAffected == 0 {
			return fmt.Errorf("产品图片 id=%d 的内容缺失", id)
		}
		return nil
	})
}

func (r *productImageRepo) Delete(ctx context.Context, id uint) error {
	return r.conn(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("image_id = ?", id).
			Delete(&model.ProductImageContent{}).Error; err != nil {
			return fmt.Errorf("删除产品图片内容 id=%d: %w", id, err)
		}
		// 元数据仍走软删：审计里要能回答「这张图是谁、什么时候传的」。
		res := tx.Delete(&model.ProductImage{}, id)
		if res.Error != nil {
			return fmt.Errorf("删除产品图片 id=%d: %w", id, res.Error)
		}
		if res.RowsAffected == 0 {
			return errs.ErrNotFound
		}
		return nil
	})
}

func (r *productImageRepo) Reorder(
	ctx context.Context, productID uint, ids []uint,
) error {
	return r.conn(ctx).Transaction(func(tx *gorm.DB) error {
		for i, id := range ids {
			res := tx.Model(&model.ProductImage{}).
				Where("id = ? AND product_id = ?", id, productID).
				Update("sort_order", i)
			if res.Error != nil {
				return fmt.Errorf("调整产品图片顺序 id=%d: %w", id, res.Error)
			}
			if res.RowsAffected == 0 {
				return errs.ErrNotFound
			}
		}
		return nil
	})
}

func (r *productImageRepo) CountByProduct(ctx context.Context, productID uint) (int64, error) {
	return r.Base.Count(ctx, func(db *gorm.DB) *gorm.DB {
		return db.Where("product_id = ?", productID)
	})
}

// PrimaryByProducts 取每个产品 sort_order 最小的那张。
//
// 不用窗口函数取第一行：为了一个「取第一张」引入子查询不值得，
// 而一页 20 个产品最多 240 行元数据，按 product_id 排好扫一遍取首次出现即可。
func (r *productImageRepo) PrimaryByProducts(
	ctx context.Context, productIDs []uint,
) (map[uint]model.ProductImage, error) {
	out := make(map[uint]model.ProductImage, len(productIDs))
	if len(productIDs) == 0 {
		return out, nil
	}

	rows := make([]model.ProductImage, 0, len(productIDs)*2)
	err := r.conn(ctx).
		Select("id", "product_id", "sort_order", "file_name", "mime_type",
			"size", "width", "height", "sha256", "created_at", "updated_at").
		Where("product_id IN ?", productIDs).
		Order("product_id ASC, sort_order ASC, id ASC").
		Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("查询产品主图: %w", err)
	}
	for _, row := range rows {
		if _, ok := out[row.ProductID]; !ok {
			out[row.ProductID] = row
		}
	}
	return out, nil
}
