package repository

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/errs"
)

// ContractFilter 是合同列表的查询条件。
//
// 不按"来源单号"筛：那份单号的持有者是报价或订单，按它找合同应该
// 从来源单据详情点过来，而不是在合同列表里手输一遍。
type ContractFilter struct {
	Keyword   string
	Type      *model.ContractType
	PartnerID *uint
	Status    *model.DocumentStatus
}

func (f ContractFilter) scopes() []Scope {
	scopes := make([]Scope, 0, 4)
	if f.Keyword != "" {
		scopes = append(scopes, likeAny(f.Keyword, "doc_no", "source_doc_no", "remark"))
	}
	if f.Type != nil {
		t := *f.Type
		scopes = append(scopes, func(db *gorm.DB) *gorm.DB { return db.Where("type = ?", t) })
	}
	if f.PartnerID != nil {
		pid := *f.PartnerID
		scopes = append(scopes, func(db *gorm.DB) *gorm.DB { return db.Where("partner_id = ?", pid) })
	}
	if f.Status != nil {
		st := *f.Status
		scopes = append(scopes, func(db *gorm.DB) *gorm.DB { return db.Where("status = ?", st) })
	}
	return scopes
}

type ContractRepository interface {
	// CreateWithTerms 在一个事务里写表头与付款条款。
	//
	// 拆成两次调用会留下"有合同没有分期"的半张单，而分期合计等于总额
	// 正是这份单据的验收条件——半张单看起来像条款被谁删了。
	CreateWithTerms(ctx context.Context, c *model.Contract, terms []model.ContractTerm) error
	// UpdateWithTerms 整体替换条款：旧行软删、新行插入，同一事务。
	// 条款行没有稳定身份（用户删一行加一行后谁是谁只能靠猜），不做增量更新。
	UpdateWithTerms(ctx context.Context, id uint, fields map[string]any, terms []model.ContractTerm) error
	Update(ctx context.Context, id uint, fields map[string]any) error
	Delete(ctx context.Context, id uint) error
	FindByID(ctx context.Context, id uint) (*model.Contract, error)
	List(ctx context.Context, f ContractFilter, p Pagination) ([]model.Contract, int64, error)
	// FindBySource 找同一来源单据生成的同类合同，用于拦住重复生成。
	// 没有时返回 (nil, nil)：调用方只关心"有没有"。
	FindBySource(
		ctx context.Context, sourceType model.ContractSourceType, sourceID uint,
		ct model.ContractType,
	) (*model.Contract, error)
}

type contractRepo struct {
	*Base[model.Contract]
}

func NewContractRepository(db *gorm.DB) ContractRepository {
	return &contractRepo{Base: NewBase[model.Contract](db)}
}

func (r *contractRepo) CreateWithTerms(
	ctx context.Context, c *model.Contract, terms []model.ContractTerm,
) error {
	err := r.conn(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(c).Error; err != nil {
			return err
		}
		if len(terms) == 0 {
			return nil
		}
		for i := range terms {
			terms[i].ContractID = c.ID
		}
		return tx.Create(&terms).Error
	})
	if err != nil {
		return fmt.Errorf("创建合同: %w", err)
	}
	c.Terms = terms
	return nil
}

func (r *contractRepo) UpdateWithTerms(
	ctx context.Context, id uint, fields map[string]any, terms []model.ContractTerm,
) error {
	err := r.conn(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&model.Contract{}).Where("id = ?", id).Updates(fields)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return errs.ErrNotFound
		}
		// 软删旧条款：合同的旧版本要能追溯，硬删会把签过的那份分期抹掉。
		if err := tx.Where("contract_id = ?", id).Delete(&model.ContractTerm{}).Error; err != nil {
			return err
		}
		if len(terms) == 0 {
			return nil
		}
		for i := range terms {
			terms[i].ContractID = id
		}
		return tx.Create(&terms).Error
	})
	if err != nil {
		if errors.Is(err, errs.ErrNotFound) {
			return errs.ErrNotFound
		}
		return fmt.Errorf("更新合同 id=%d: %w", id, err)
	}
	return nil
}

func (r *contractRepo) Update(ctx context.Context, id uint, fields map[string]any) error {
	return r.Base.UpdateFields(ctx, id, fields)
}

func (r *contractRepo) Delete(ctx context.Context, id uint) error {
	return r.Base.DeleteByID(ctx, id)
}

func (r *contractRepo) FindByID(ctx context.Context, id uint) (*model.Contract, error) {
	var m model.Contract
	err := r.conn(ctx).
		Preload("Partner").
		// 条款按支付顺序排，而不是按 id：先订金后尾款，插入顺序不保证就是付款顺序。
		Preload("Terms", func(db *gorm.DB) *gorm.DB { return db.Order("seq ASC, id ASC") }).
		First(&m, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errs.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("查询合同 id=%d: %w", id, err)
	}
	return &m, nil
}

// List 按签订日倒序：最近出具的在最上面。
// 不预加载条款——一页 20 份、每份几期，带出来只为显示"分几期"不值得。
func (r *contractRepo) List(
	ctx context.Context, f ContractFilter, p Pagination,
) ([]model.Contract, int64, error) {
	scopes := append(f.scopes(), func(db *gorm.DB) *gorm.DB {
		return db.Preload("Partner")
	})
	return r.PaginateOrdered(ctx, p, "sign_date DESC, id DESC", scopes...)
}

func (r *contractRepo) FindBySource(
	ctx context.Context, sourceType model.ContractSourceType, sourceID uint,
	ct model.ContractType,
) (*model.Contract, error) {
	var m model.Contract
	err := r.conn(ctx).
		Where("source_type = ? AND source_id = ? AND type = ?", sourceType, sourceID, ct).
		First(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("按来源查合同: %w", err)
	}
	return &m, nil
}
