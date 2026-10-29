package repository

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/errs"
)

// PartnerFilter 是商业伙伴列表的查询条件。
//
// Role 是"包含"语义而不是"等于"：筛供应商时，既做客户又做供应商的主体
// 必须一起出来，否则它在采购侧的选择列表里会消失。
type PartnerFilter struct {
	Keyword    string
	Role       *model.PartnerRole
	EntityType *model.PartnerEntityType
	Country    string
	Status     *model.EnableStatus
	// IncludeDeleted 把已软删的主体也查出来，供「含已删除」的列表与恢复入口使用。
	IncludeDeleted bool
	// OwnerID 只看某人负责的主体。**owner_id = 0 视为未分配，一并返回**：
	// 刚上线的存量数据与"先录进来还没分人"的客户都在这一堆里，
	// 藏起来会让人以为数据丢了。
	OwnerID *uint
}

func (f PartnerFilter) scopes() []Scope {
	scopes := make([]Scope, 0, 6)

	if f.IncludeDeleted {
		scopes = append(scopes, func(db *gorm.DB) *gorm.DB { return db.Unscoped() })
	}
	// 列名一律带表前缀：列表查询 join 了 sys_user 取负责人登录名，
	// 而 sys_user 也有 role / status / code 之类的同名列，
	// 不限定表名就会得到 "ambiguous column name"。
	if f.Keyword != "" {
		scopes = append(scopes, likeAny(f.Keyword, "md_partner.code", "md_partner.name", "md_partner.contact"))
	}
	if f.Role != nil {
		// 展开成 IN 集合而不是写 `role & ? <> 0`：位运算在 PostgreSQL 和 MySQL
		// 上都用不上 role 上的 btree 索引，而按角色过滤是列表页的默认条件。
		values := model.RoleValuesContaining(*f.Role)
		scopes = append(scopes, func(db *gorm.DB) *gorm.DB {
			return db.Where("md_partner.role IN ?", values)
		})
	}
	if f.EntityType != nil {
		t := *f.EntityType
		scopes = append(scopes, func(db *gorm.DB) *gorm.DB {
			return db.Where("md_partner.entity_type = ?", t)
		})
	}
	if f.Country != "" {
		c := f.Country
		scopes = append(scopes, func(db *gorm.DB) *gorm.DB {
			return db.Where("md_partner.country = ?", c)
		})
	}
	if f.Status != nil {
		st := *f.Status
		scopes = append(scopes, func(db *gorm.DB) *gorm.DB {
			return db.Where("md_partner.status = ?", st)
		})
	}
	if f.OwnerID != nil {
		owner := *f.OwnerID
		scopes = append(scopes, func(db *gorm.DB) *gorm.DB {
			return db.Where("md_partner.owner_id = ? OR md_partner.owner_id = 0", owner)
		})
	}
	return scopes
}

type PartnerRepository interface {
	Create(ctx context.Context, m *model.Partner) error
	Update(ctx context.Context, id uint, fields map[string]any) error
	Delete(ctx context.Context, id uint) error
	FindByID(ctx context.Context, id uint) (*model.Partner, error)
	// FindByIDIncludingDeleted 连已软删的行也查，供恢复入口确认"删的是哪一条"。
	FindByIDIncludingDeleted(ctx context.Context, id uint) (*model.Partner, error)
	// Restore 把软删标记写回 0；编码被占用时由唯一索引拒绝（回调会翻成 409）。
	Restore(ctx context.Context, id uint) error
	ExistsByCode(ctx context.Context, code string, excludeID uint) (bool, error)
	// ListAll 返回全部未删除的主体（id 升序），供查重扫描。
	//
	// 一次取全表而不是分页：查重要跨"这一页之外"的记录比对，分页取数会把同一个
	// 主体的两条重复拆到两页里，谁都看不到它们在一起。单租户、几千行的量级下
	// 一次扫完最省事；真到十万行再考虑把归一值落库（见 model/partner_dedupe.go）。
	ListAll(ctx context.Context) ([]model.Partner, error)
	// Merge 在一个事务里把被合并方的全部引用迁到主记录，再软删被合并方。
	// 返回各表被迁移的行数，供 service 写日志与回显"动过什么"。
	Merge(ctx context.Context, plan PartnerMergePlan) (map[string]int64, error)
	// ProductSourceConflicts 列出"合并后会同产品同货号重复"的货源，供合并前拦下来。
	ProductSourceConflicts(ctx context.Context, survivorID, sourceID uint) ([]string, error)
	List(ctx context.Context, f PartnerFilter, p Pagination) ([]model.Partner, int64, error)
	Count(ctx context.Context) (int64, error)
	CountByRole(ctx context.Context, role model.PartnerRole) (int64, error)
}

type partnerRepo struct {
	*Base[model.Partner]
}

func NewPartnerRepository(db *gorm.DB) PartnerRepository {
	return &partnerRepo{Base: NewBase[model.Partner](db)}
}

func (r *partnerRepo) Create(ctx context.Context, m *model.Partner) error {
	return r.Base.Create(ctx, m)
}

func (r *partnerRepo) Update(ctx context.Context, id uint, fields map[string]any) error {
	return r.Base.UpdateFields(ctx, id, fields)
}

func (r *partnerRepo) Delete(ctx context.Context, id uint) error {
	return r.Base.DeleteByID(ctx, id)
}

func (r *partnerRepo) FindByID(ctx context.Context, id uint) (*model.Partner, error) {
	return r.Base.FindByID(ctx, id)
}

func (r *partnerRepo) FindByIDIncludingDeleted(
	ctx context.Context, id uint,
) (*model.Partner, error) {
	var m model.Partner
	err := r.conn(ctx).Unscoped().First(&m, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errs.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("查询往来单位 id=%d（含已删除）: %w", id, err)
	}
	return &m, nil
}

func (r *partnerRepo) Restore(ctx context.Context, id uint) error {
	err := r.conn(ctx).Unscoped().Model(&model.Partner{}).
		Where("id = ?", id).
		Update("deleted_at", 0).Error
	if err != nil {
		return fmt.Errorf("恢复往来单位 id=%d: %w", id, err)
	}
	return nil
}

// ExistsByCode 做编码唯一性前置校验，excludeID 供编辑时排除自身。
//
// 合并后编码在客户与供应商之间也唯一：同一个主体只有一个编码，
// 这正是合并要解决的问题（原先 CUS001 与 SUP001 可能指向同一家公司）。
func (r *partnerRepo) ExistsByCode(ctx context.Context, code string, excludeID uint) (bool, error) {
	return r.Exists(ctx, func(db *gorm.DB) *gorm.DB {
		q := db.Where("code = ?", code)
		if excludeID > 0 {
			q = q.Where("id <> ?", excludeID)
		}
		return q
	})
}

func (r *partnerRepo) ListAll(ctx context.Context) ([]model.Partner, error) {
	var rows []model.Partner
	if err := r.conn(ctx).Order("id ASC").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("查询全部往来单位: %w", err)
	}
	return rows, nil
}

func (r *partnerRepo) List(ctx context.Context, f PartnerFilter, p Pagination) ([]model.Partner, int64, error) {
	p = p.normalize()
	scopes := f.scopes()

	var total int64
	if err := r.conn(ctx).Model(&model.Partner{}).Scopes(scopes...).Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("统计主体: %w", err)
	}
	if total == 0 {
		return []model.Partner{}, 0, nil
	}

	// 不复用 Paginate：它给 count 与查询用同一组条件，而这里查询要多带一个
	// LEFT JOIN 取负责人姓名——Count 不能带多列 Select，混在一起会把统计弄坏。
	list := make([]model.Partner, 0, p.PageSize)
	err := r.conn(ctx).Model(&model.Partner{}).Scopes(scopes...).
		Select("md_partner.*, COALESCE(sys_user.username, '') AS owner_name").
		Joins("LEFT JOIN sys_user ON sys_user.id = md_partner.owner_id").
		Order("md_partner.id DESC").
		Offset(p.Offset()).
		Limit(p.PageSize).
		Find(&list).Error
	if err != nil {
		return nil, 0, fmt.Errorf("查询主体: %w", err)
	}
	return list, total, nil
}

func (r *partnerRepo) Count(ctx context.Context) (int64, error) {
	return r.Base.Count(ctx)
}

// CountByRole 统计承担了某个角色的主体数（含同时承担另一个角色的）。
//
// 用 RoleValuesContaining 而不是 Count(role = ?)：后者会漏掉 role=3 的"客供两用"
// 主体，导致概览里的供应商数比供应商列表实际少了。
func (r *partnerRepo) CountByRole(ctx context.Context, role model.PartnerRole) (int64, error) {
	values := model.RoleValuesContaining(role)
	return r.Base.Count(ctx, func(db *gorm.DB) *gorm.DB {
		return db.Where("role IN ?", values)
	})
}
