package repository

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"github.com/edwinhuish/gowork/internal/model"
)

// ClauseFilter 是条款模板的查询条件。
type ClauseFilter struct {
	Keyword  string
	Category string
	// IncludeDeleted 把已软删的条款也列出来，供恢复与"这个模板还在被谁引用"的排查。
	IncludeDeleted bool
}

// ClauseRepository 是条款模板的持久化入口。
type ClauseRepository interface {
	Create(ctx context.Context, m *model.Clause) error
	Update(ctx context.Context, id uint, fields map[string]any) error
	Delete(ctx context.Context, id uint) error
	// Restore 把软删标记写回 0；编码被占用时由唯一索引拒绝（上层翻成 409）。
	Restore(ctx context.Context, id uint) error
	FindByID(ctx context.Context, id uint) (*model.Clause, error)
	// FindByIDs 按 id 批量取：生成合同时一次性把选中的模板取全，
	// 而不是一条一条查——十条条款就是十次往返。
	FindByIDs(ctx context.Context, ids []uint) ([]model.Clause, error)
	ExistsByCode(ctx context.Context, code string, excludeID uint) (bool, error)
	List(ctx context.Context, f ClauseFilter, p Pagination) ([]model.Clause, int64, error)
}

type clauseRepo struct {
	*Base[model.Clause]
}

func NewClauseRepository(db *gorm.DB) ClauseRepository {
	return &clauseRepo{Base: NewBase[model.Clause](db)}
}

func (r *clauseRepo) Create(ctx context.Context, m *model.Clause) error {
	return r.Base.Create(ctx, m)
}

func (r *clauseRepo) Update(ctx context.Context, id uint, fields map[string]any) error {
	return r.UpdateFields(ctx, id, fields)
}

func (r *clauseRepo) Delete(ctx context.Context, id uint) error {
	return r.DeleteByID(ctx, id)
}

func (r *clauseRepo) Restore(ctx context.Context, id uint) error {
	err := r.conn(ctx).Unscoped().Model(&model.Clause{}).
		Where("id = ?", id).
		Update("deleted_at", 0).Error
	if err != nil {
		return fmt.Errorf("恢复条款 id=%d: %w", id, err)
	}
	return nil
}

func (r *clauseRepo) FindByID(ctx context.Context, id uint) (*model.Clause, error) {
	return r.Base.FindByID(ctx, id)
}

func (r *clauseRepo) FindByIDs(ctx context.Context, ids []uint) ([]model.Clause, error) {
	if len(ids) == 0 {
		return []model.Clause{}, nil
	}
	list := make([]model.Clause, 0, len(ids))
	if err := r.conn(ctx).Where("id IN ?", ids).Find(&list).Error; err != nil {
		return nil, fmt.Errorf("按 id 查条款: %w", err)
	}
	return list, nil
}

func (r *clauseRepo) ExistsByCode(ctx context.Context, code string, excludeID uint) (bool, error) {
	q := r.conn(ctx).Model(&model.Clause{}).Where("code = ?", code)
	if excludeID != 0 {
		q = q.Where("id <> ?", excludeID)
	}
	var count int64
	if err := q.Count(&count).Error; err != nil {
		return false, fmt.Errorf("检查条款编码: %w", err)
	}
	return count > 0, nil
}

func (r *clauseRepo) List(
	ctx context.Context, f ClauseFilter, p Pagination,
) ([]model.Clause, int64, error) {
	scopes := make([]Scope, 0, 4)
	if f.IncludeDeleted {
		scopes = append(scopes, func(db *gorm.DB) *gorm.DB { return db.Unscoped() })
	}
	if f.Keyword != "" {
		scopes = append(scopes, likeAny(f.Keyword, "code", "title", "body"))
	}
	if f.Category != "" {
		category := f.Category
		scopes = append(scopes, func(db *gorm.DB) *gorm.DB {
			return db.Where("category = ?", category)
		})
	}
	return r.Paginate(ctx, p, scopes...)
}

// FindContractClauses 取一份合同上的条款快照（按 Seq 升序）。
func FindContractClauses(ctx context.Context, db *gorm.DB, contractID uint) ([]model.ContractClause, error) {
	list := make([]model.ContractClause, 0, 8)
	err := dbFor(ctx, db).
		Where("contract_id = ?", contractID).
		Order("seq ASC, id ASC").
		Find(&list).Error
	if err != nil {
		return nil, fmt.Errorf("查询合同条款: %w", err)
	}
	return list, nil
}
