package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"

	"github.com/edwinhuish/gowork/internal/model"
	codenorm "github.com/edwinhuish/gowork/internal/pkg/code"
	"github.com/edwinhuish/gowork/internal/pkg/errs"
)

// ProductFilter 是产品列表的查询条件。
type ProductFilter struct {
	Keyword    string
	Category   string
	SupplierID *uint
	Status     *model.EnableStatus
	// IncludeDeleted 把已软删的行也查出来，供「含已删除」的列表与恢复入口使用。
	// 默认 false：其余所有查询都不该看见已删数据。
	IncludeDeleted bool

	// SortField / SortOrder 由列表页传入。合规性在这里判，不靠调用方：
	// 白名单映射成列名，不在表里的一律退回默认顺序。
	SortField string
	SortOrder string
}

// sortableProductFields 是列表允许排序的字段 → 列名。
//
// 用映射而不是直接拼列名：字段名来自 HTTP 查询串，直接拼就是注入面；
// 也只有映射能让"前端多传一个字段"退化成"按默认顺序排"，而不是 500。
var sortableProductFields = map[string]string{
	"sku_code":   "sku_code",
	"name_cn":    "name_cn",
	"category":   "category",
	"cost_price": "cost_price",
	"sale_price": "sale_price",
	"created_at": "id",
}

// order 返回 ORDER BY 子句。
//
// 一律带 id DESC 作为次级排序：价格相同的行（同类标准件很常见）若没有稳定的
// 第二排序键，翻页时同一行可能重复出现、另一行永远看不到。
func (f ProductFilter) order() string {
	col, ok := sortableProductFields[f.SortField]
	if !ok {
		return "id DESC"
	}
	if strings.EqualFold(f.SortOrder, "desc") {
		return col + " DESC, id DESC"
	}
	return col + " ASC, id DESC"
}

func (f ProductFilter) scopes() []Scope {
	scopes := make([]Scope, 0, 5)

	if f.IncludeDeleted {
		scopes = append(scopes, func(db *gorm.DB) *gorm.DB { return db.Unscoped() })
	}
	if f.Keyword != "" {
		scopes = append(scopes, keywordScope(f.Keyword))
	}
	if f.Category != "" {
		cat := f.Category
		scopes = append(scopes, func(db *gorm.DB) *gorm.DB {
			return db.Where("category = ?", cat)
		})
	}
	if f.SupplierID != nil {
		sid := *f.SupplierID
		scopes = append(scopes, func(db *gorm.DB) *gorm.DB {
			return db.Where("supplier_id = ?", sid)
		})
	}
	if f.Status != nil {
		st := *f.Status
		scopes = append(scopes, func(db *gorm.DB) *gorm.DB {
			return db.Where("status = ?", st)
		})
	}
	return scopes
}

// keywordScope 是产品列表的关键词条件：SKU、中英文品名，以及**任何编号**（旧 SKU、
// 客户物料号、工厂号、供应商货号）。编号分散在两张表：md_product_code 存没有别处
// 可挂的别名，md_product_source 存跟着供应商走的货号，两路都要查。
//
// 编号走子查询而不是 JOIN：一个产品可能同时命中多个编号，JOIN 会让它在一页里出现
// 好几行，count 也跟着变大——用户看到的是「搜一个词，同一条货出现三遍」。
//
// 编号那一路的关键词要先归一化（去分隔符、转大写），与 md_product_code.code_norm
// 用同一个函数：两边口径不同就会出现"有，但搜不到"。
func keywordScope(keyword string) Scope {
	norm := codenorm.Normalize(keyword)

	return func(db *gorm.DB) *gorm.DB {
		parts := []string{
			likeClause(db, "sku_code"),
			likeClause(db, "name_cn"),
			likeClause(db, "name_en"),
		}
		args := []any{likePattern(keyword), likePattern(keyword), likePattern(keyword)}

		// 关键词里全是分隔符时归一化结果是空串：此时不能去查编号，
		// 否则 LIKE '%%' 会把所有有编号的产品都带出来。
		if norm != "" {
			parts = append(parts,
				"id IN (SELECT product_id FROM md_product_code WHERE "+
					likeClause(db, "code_norm")+")")
			args = append(args, likePattern(norm))

			// 供应商货号放在货源表上，不在编号表里：它跟着供应商与采购价走，
			// 复制一份到编号表就是两处真相，改价时得记得同步两遍。
			// 子查询显式排除软删行——原生的 IN 子查询不经过 gorm 的软删条件，
			// 少了这一句，删掉的货源仍会把产品带出来。
			parts = append(parts,
				"id IN (SELECT product_id FROM md_product_source WHERE "+
					likeClause(db, "supplier_sku_norm")+" AND deleted_at = 0)")
			args = append(args, likePattern(norm))
		}
		return db.Where(strings.Join(parts, " OR "), args...)
	}
}

type ProductRepository interface {
	Create(ctx context.Context, m *model.Product) error
	Update(ctx context.Context, id uint, fields map[string]any) error
	Delete(ctx context.Context, id uint) error
	FindByID(ctx context.Context, id uint) (*model.Product, error)
	// FindByIDIncludingDeleted 连已软删的行也查。恢复入口要先看清"删的是哪一条"，
	// 普通 FindByID 对已删记录只会返回 ErrNotFound。
	FindByIDIncludingDeleted(ctx context.Context, id uint) (*model.Product, error)
	// Restore 把软删标记写回 0。编码已被别的记录占用时由唯一索引拒绝，
	// 错误会经 gorm 回调翻成 409，不需要在这里再判一次。
	Restore(ctx context.Context, id uint) error
	ExistsBySKUCode(ctx context.Context, skuCode string, excludeID uint) (bool, error)
	List(ctx context.Context, f ProductFilter, p Pagination) ([]model.Product, int64, error)
	Count(ctx context.Context) (int64, error)
	CountByEnableStatus(ctx context.Context, status model.EnableStatus) (int64, error)
	CountBySupplierID(ctx context.Context, supplierID uint) (int64, error)
}

type productRepo struct {
	*Base[model.Product]
}

func NewProductRepository(db *gorm.DB) ProductRepository {
	return &productRepo{Base: NewBase[model.Product](db)}
}

func (r *productRepo) Create(ctx context.Context, m *model.Product) error {
	return r.Base.Create(ctx, m)
}

func (r *productRepo) Update(ctx context.Context, id uint, fields map[string]any) error {
	return r.Base.UpdateFields(ctx, id, fields)
}

func (r *productRepo) Delete(ctx context.Context, id uint) error {
	return r.Base.DeleteByID(ctx, id)
}

// FindByID 覆盖基类实现，带出供应商信息供列表展示。
func (r *productRepo) FindByID(ctx context.Context, id uint) (*model.Product, error) {
	var m model.Product
	err := r.conn(ctx).Preload("Supplier").First(&m, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errs.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("查询产品 id=%d: %w", id, err)
	}
	return &m, nil
}

func (r *productRepo) FindByIDIncludingDeleted(
	ctx context.Context, id uint,
) (*model.Product, error) {
	var m model.Product
	err := r.conn(ctx).Unscoped().Preload("Supplier").First(&m, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errs.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("查询产品 id=%d（含已删除）: %w", id, err)
	}
	return &m, nil
}

func (r *productRepo) Restore(ctx context.Context, id uint) error {
	err := r.conn(ctx).Unscoped().Model(&model.Product{}).
		Where("id = ?", id).
		Update("deleted_at", 0).Error
	if err != nil {
		return fmt.Errorf("恢复产品 id=%d: %w", id, err)
	}
	return nil
}

// ExistsBySKUCode 做 SKU 编码的唯一性前置校验，excludeID 供编辑时排除自身。
//
// 名字里必须带 SKU：客户与供应商的仓储也有一个查重方法，但查的是各自的
// 业务编码 code。三个都叫 ExistsByCode 时，调用方只能靠记参数分辨。
func (r *productRepo) ExistsBySKUCode(ctx context.Context, skuCode string, excludeID uint) (bool, error) {
	return r.Exists(ctx, func(db *gorm.DB) *gorm.DB {
		q := db.Where("sku_code = ?", skuCode)
		if excludeID > 0 {
			q = q.Where("id <> ?", excludeID)
		}
		return q
	})
}

// List 列表带上供应商，便于前端直接展示名称。
// Preload 只在 Find 时生效，Count 阶段会被 gorm 忽略，不影响统计。
func (r *productRepo) List(ctx context.Context, f ProductFilter, p Pagination) ([]model.Product, int64, error) {
	// 排序走 PaginateOrderedWithPreload 而不是 Paginate：后者写死 id DESC，
	// 而列表页要能按编码、品名、价格排。
	return r.PaginateOrderedWithPreload(ctx, p, f.order(), []string{"Supplier"}, f.scopes()...)
}

func (r *productRepo) Count(ctx context.Context) (int64, error) {
	return r.Base.Count(ctx)
}

func (r *productRepo) CountByEnableStatus(ctx context.Context, status model.EnableStatus) (int64, error) {
	return r.Base.Count(ctx, func(db *gorm.DB) *gorm.DB {
		return db.Where("status = ?", status)
	})
}

// CountBySupplierID 统计挂在某个供应商下的产品数。
//
// 主体合并后，供应商不再是独立的一张表，"能不能删这个供应商"变成一个要
// 跨聚合根判断的问题：有产品在引用它就不能删，也不能把它身上的 supplier 角色摘掉。
func (r *productRepo) CountBySupplierID(ctx context.Context, supplierID uint) (int64, error) {
	return r.Base.Count(ctx, func(db *gorm.DB) *gorm.DB {
		return db.Where("supplier_id = ?", supplierID)
	})
}
