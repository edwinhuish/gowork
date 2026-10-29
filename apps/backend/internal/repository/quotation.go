package repository

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/errs"
)

// QuotationFilter 是报价单列表的查询条件。
type QuotationFilter struct {
	// Keyword 只匹配单号与备注：客户名在列表里是冗余展示，
	// 按客户找报价走 partner_id，两个入口搜同一件事会让人分不清自己在哪。
	Keyword   string
	PartnerID *uint
	Status    *model.DocumentStatus
	// OwnerID 只看归属该业务员的客户的报价（含未分配客户的）。
	OwnerID *uint
}

func (f QuotationFilter) scopes() []Scope {
	scopes := make([]Scope, 0, 3)
	if f.Keyword != "" {
		scopes = append(scopes, likeAny(f.Keyword, "doc_no", "remark"))
	}
	if f.PartnerID != nil {
		pid := *f.PartnerID
		scopes = append(scopes, func(db *gorm.DB) *gorm.DB {
			return db.Where("partner_id = ?", pid)
		})
	}
	if f.Status != nil {
		st := *f.Status
		scopes = append(scopes, func(db *gorm.DB) *gorm.DB {
			return db.Where("status = ?", st)
		})
	}
	if f.OwnerID != nil {
		scopes = append(scopes, OwnerScope("partner_id", *f.OwnerID))
	}
	return scopes
}

type QuotationRepository interface {
	// CreateWithItems 在一个事务里写表头与明细。
	//
	// 拆成两次调用会留下"有单头没有明细"的半张单：那张单总额为 0，
	// 看起来像客户白拿了货，而原因是一次网络抖动。
	CreateWithItems(ctx context.Context, q *model.Quotation, items []model.QuotationItem) error
	// UpdateWithItems 整体替换明细：旧行软删、新行插入，都在同一个事务里。
	// 不按行 id 做增量更新——报价单的明细行没有稳定身份，用户加一行删一行之后
	// 哪行是哪行只能靠猜，猜错就把别人的备注挪到了另一行上。
	UpdateWithItems(ctx context.Context, id uint, fields map[string]any, items []model.QuotationItem) error
	Update(ctx context.Context, id uint, fields map[string]any) error
	Delete(ctx context.Context, id uint) error
	FindByID(ctx context.Context, id uint) (*model.Quotation, error)
	List(ctx context.Context, f QuotationFilter, p Pagination) ([]model.Quotation, int64, error)
	// MaxVersionByDocNo 取同一份报价已有的最大版本号，供"另存新版本"。
	MaxVersionByDocNo(ctx context.Context, docNo string) (int, error)
	// MarkConverted 抢占式回填"已转成哪张订单"，返回是否抢到。
	//
	// 条件更新（WHERE sales_order_id IS NULL）是幂等的唯一闸门：
	// 先查后写挡不住并发双击，两个人同时点"转订单"会得到两张订单。
	MarkConverted(ctx context.Context, quotationID, orderID uint, orderDocNo string) (bool, error)
}

type quotationRepo struct {
	*Base[model.Quotation]
}

func NewQuotationRepository(db *gorm.DB) QuotationRepository {
	return &quotationRepo{Base: NewBase[model.Quotation](db)}
}

func (r *quotationRepo) CreateWithItems(
	ctx context.Context, q *model.Quotation, items []model.QuotationItem,
) error {
	err := r.conn(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(q).Error; err != nil {
			return err
		}
		for i := range items {
			items[i].QuotationID = q.ID
		}
		if len(items) == 0 {
			return nil
		}
		return tx.Create(&items).Error
	})
	if err != nil {
		return fmt.Errorf("创建报价单: %w", err)
	}
	q.Items = items
	return nil
}

func (r *quotationRepo) UpdateWithItems(
	ctx context.Context, id uint, fields map[string]any, items []model.QuotationItem,
) error {
	err := r.conn(ctx).Transaction(func(tx *gorm.DB) error {
		q := tx.Model(&model.Quotation{}).Where("id = ?", id)
		// 与 Base.UpdateFields 同一套乐观锁：明细的改动也要建立在同一份底稿上。
		expected, hasLock := takeOptimisticLock(fields)
		if hasLock {
			q = q.Where("updated_at = ?", expected)
		}
		res := q.Updates(fields)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			if hasLock {
				return errs.ErrConflict.WithMsg("这条记录已被他人修改，请刷新后重试")
			}
			return errs.ErrNotFound
		}
		// 软删旧明细：报价单的历史版本仍在，硬删会连带把可追溯的明细抹掉。
		if err := tx.Where("quotation_id = ?", id).Delete(&model.QuotationItem{}).Error; err != nil {
			return err
		}
		if len(items) == 0 {
			return nil
		}
		for i := range items {
			items[i].QuotationID = id
		}
		return tx.Create(&items).Error
	})
	if err != nil {
		if errors.Is(err, errs.ErrNotFound) {
			return errs.ErrNotFound
		}
		return fmt.Errorf("更新报价单 id=%d: %w", id, err)
	}
	return nil
}

func (r *quotationRepo) Update(ctx context.Context, id uint, fields map[string]any) error {
	return r.Base.UpdateFields(ctx, id, fields)
}

func (r *quotationRepo) Delete(ctx context.Context, id uint) error {
	return r.Base.DeleteByID(ctx, id)
}

func (r *quotationRepo) FindByID(ctx context.Context, id uint) (*model.Quotation, error) {
	var m model.Quotation
	err := r.conn(ctx).
		Preload("Partner").
		Preload("Items", func(db *gorm.DB) *gorm.DB { return db.Order("id ASC") }).
		Preload("Items.Product").
		First(&m, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errs.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("查询报价单 id=%d: %w", id, err)
	}
	return &m, nil
}

// List 按创建时间倒序：最近发出的报价在最上面。
// 列表不预加载明细——一页 20 张单、每张十几行，全部带出来只为显示一个毛利率不值得。
func (r *quotationRepo) List(
	ctx context.Context, f QuotationFilter, p Pagination,
) ([]model.Quotation, int64, error) {
	scopes := append(f.scopes(), func(db *gorm.DB) *gorm.DB {
		return db.Preload("Partner")
	})
	return r.PaginateOrdered(ctx, p, "id DESC", scopes...)
}

func (r *quotationRepo) MarkConverted(
	ctx context.Context, quotationID, orderID uint, orderDocNo string,
) (bool, error) {
	res := r.conn(ctx).Model(&model.Quotation{}).
		Where("id = ? AND sales_order_id IS NULL", quotationID).
		Updates(map[string]any{
			"sales_order_id":     orderID,
			"sales_order_doc_no": orderDocNo,
		})
	if res.Error != nil {
		return false, fmt.Errorf("回填报价单转单引用 id=%d: %w", quotationID, res.Error)
	}
	return res.RowsAffected > 0, nil
}

func (r *quotationRepo) MaxVersionByDocNo(ctx context.Context, docNo string) (int, error) {
	var max int
	err := r.conn(ctx).Model(&model.Quotation{}).
		Where("doc_no = ?", docNo).
		Select("COALESCE(MAX(version), 0)").
		Scan(&max).Error
	if err != nil {
		return 0, fmt.Errorf("查询报价单最大版本 doc_no=%s: %w", docNo, err)
	}
	return max, nil
}
