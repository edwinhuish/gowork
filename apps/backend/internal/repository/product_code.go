package repository

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/code"
	"github.com/edwinhuish/gowork/internal/pkg/errs"
)

type ProductCodeRepository interface {
	Create(ctx context.Context, m *model.ProductCode) error
	FindByID(ctx context.Context, id uint) (*model.ProductCode, error)
	ListByProductID(ctx context.Context, productID uint) ([]model.ProductCode, error)
	// Delete 是**硬删**：编号是一张标签，不是单据。
	//
	// 软删会让「删掉再加回来」撞上 (product_id, code_norm) 的唯一索引——
	// 表现是加不回去，而列表里又看不见它，只能去库里找那行已删记录。
	Delete(ctx context.Context, id uint) error
	// DeleteByProductID 硬删某个产品的全部编号，供产品删除时清理。
	//
	// 与 Delete 同为硬删：编号是标签，产品没了标签也不该留着。留下孤儿行
	// 的后果不是"多几行"，而是「新 SKU 不得等于别人用过的编号」会把那个号
	// 永久占住——而界面上再也看不见它（编号面板按 product_id 查，产品已不在）。
	DeleteByProductID(ctx context.Context, productID uint) error
	// ExistsNorm 判断同一产品下是否已有这个归一化编号（excludeID 供编辑排除自身）。
	ExistsNorm(ctx context.Context, productID uint, codeNorm string, excludeID uint) (bool, error)
	// ExistsNormAnywhere 判断**任何产品**是否已有这个归一化编号，供「新 SKU 不得
	// 等于别人用过的编号」使用。excludeProductID 排除自己：自己的旧码不算冲突。
	ExistsNormAnywhere(ctx context.Context, codeNorm string, excludeProductID uint) (bool, error)
	// MatchByKeyword 取这些产品里命中关键词的编号，用于列表上的「命中编号」提示。
	//
	// 判定与列表筛选共用同一条 LIKE：两处写得不一样，会出现"列表命中了但看不出为什么"。
	MatchByKeyword(ctx context.Context, productIDs []uint, keyword string) (map[uint][]string, error)
}

type productCodeRepo struct {
	*Base[model.ProductCode]
}

func NewProductCodeRepository(db *gorm.DB) ProductCodeRepository {
	return &productCodeRepo{Base: NewBase[model.ProductCode](db)}
}

func (r *productCodeRepo) Create(ctx context.Context, m *model.ProductCode) error {
	if err := r.Base.Create(ctx, m); err != nil {
		return err
	}
	return nil
}

func (r *productCodeRepo) FindByID(ctx context.Context, id uint) (*model.ProductCode, error) {
	var m model.ProductCode
	err := r.conn(ctx).First(&m, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errs.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("查询产品编号 id=%d: %w", id, err)
	}
	return &m, nil
}

func (r *productCodeRepo) ListByProductID(
	ctx context.Context, productID uint,
) ([]model.ProductCode, error) {
	list := make([]model.ProductCode, 0, 4)
	err := r.conn(ctx).
		Where("product_id = ?", productID).
		// 新的在上：刚加的那条应当第一眼看到。
		Order("id DESC").
		Find(&list).Error
	if err != nil {
		return nil, fmt.Errorf("查询产品编号列表 product_id=%d: %w", productID, err)
	}
	return list, nil
}

func (r *productCodeRepo) Delete(ctx context.Context, id uint) error {
	res := r.conn(ctx).Unscoped().Delete(&model.ProductCode{}, id)
	if res.Error != nil {
		return fmt.Errorf("删除产品编号 id=%d: %w", id, res.Error)
	}
	if res.RowsAffected == 0 {
		return errs.ErrNotFound
	}
	return nil
}

func (r *productCodeRepo) DeleteByProductID(ctx context.Context, productID uint) error {
	err := r.conn(ctx).Unscoped().
		Where("product_id = ?", productID).
		Delete(&model.ProductCode{}).Error
	if err != nil {
		return fmt.Errorf("清理产品 id=%d 的编号: %w", productID, err)
	}
	return nil
}

func (r *productCodeRepo) ExistsNorm(
	ctx context.Context, productID uint, codeNorm string, excludeID uint,
) (bool, error) {
	return r.Exists(ctx, func(db *gorm.DB) *gorm.DB {
		q := db.Where("product_id = ? AND code_norm = ?", productID, codeNorm)
		if excludeID > 0 {
			q = q.Where("id <> ?", excludeID)
		}
		return q
	})
}

func (r *productCodeRepo) ExistsNormAnywhere(
	ctx context.Context, codeNorm string, excludeProductID uint,
) (bool, error) {
	return r.Exists(ctx, func(db *gorm.DB) *gorm.DB {
		q := db.Where("code_norm = ?", codeNorm).
			// 已软删产品留下的编号不该再占号：清理只在删除那条路径上做，
			// 历史数据里可能已经有孤儿行，而它们没有任何入口能删掉。
			//
			// 子查询手写而不交给 gorm：在 scope 里用同一个 *gorm.DB 构造子查询
			// 会污染外层语句（gorm 把 IN 的参数当成同一张表的延续），
			// 生成的 SQL 会自引用并带上残缺的 IN ()。
			Where("product_id IN (SELECT id FROM md_product WHERE deleted_at = 0)")
		if excludeProductID > 0 {
			q = q.Where("product_id <> ?", excludeProductID)
		}
		return q
	})
}

func (r *productCodeRepo) MatchByKeyword(
	ctx context.Context, productIDs []uint, keyword string,
) (map[uint][]string, error) {
	out := make(map[uint][]string, len(productIDs))
	if len(productIDs) == 0 || keyword == "" {
		return out, nil
	}

	var rows []model.ProductCode
	err := r.conn(ctx).
		Where("product_id IN ?", productIDs).
		Scopes(func(db *gorm.DB) *gorm.DB {
			return db.Where(likeClause(db, "code_norm"), likePattern(code.Normalize(keyword)))
		}).
		Order("id ASC").
		Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("查询命中编号: %w", err)
	}
	for _, row := range rows {
		out[row.ProductID] = append(out[row.ProductID], row.Code)
	}
	return out, nil
}
