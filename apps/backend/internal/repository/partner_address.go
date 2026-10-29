package repository

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/errs"
)

// PartnerAddressRepository 读写主体的多地址。
//
// 默认地址的唯一性是这一层的责任，不是数据库的：三库对"条件唯一索引"
// 的支持各不相同，应用层事务是唯一一致的做法（与 Contact 的主联系人同一套路）。
type PartnerAddressRepository interface {
	// ListByPartner 返回某主体的地址，默认地址排在前，其余按 id 升序。
	ListByPartner(ctx context.Context, partnerID uint, typ model.PartnerAddressType) ([]model.PartnerAddress, error)
	FindByID(ctx context.Context, id uint) (*model.PartnerAddress, error)
	CountByPartner(ctx context.Context, partnerID uint) (int64, error)
	Create(ctx context.Context, m *model.PartnerAddress) error
	// Update 与 Delete 收当前行而不只是 id：
	// 默认地址的接替与取消都要看原值，service 已经查过一次，不必再查第二遍。
	Update(ctx context.Context, current *model.PartnerAddress, fields map[string]any) error
	Delete(ctx context.Context, current *model.PartnerAddress) error
}

type partnerAddressRepo struct {
	*Base[model.PartnerAddress]
}

func NewPartnerAddressRepository(db *gorm.DB) PartnerAddressRepository {
	return &partnerAddressRepo{Base: NewBase[model.PartnerAddress](db)}
}

func (r *partnerAddressRepo) ListByPartner(
	ctx context.Context, partnerID uint, typ model.PartnerAddressType,
) ([]model.PartnerAddress, error) {
	q := r.conn(ctx).Where("partner_id = ?", partnerID)
	if typ != "" {
		q = q.Where("type = ?", typ)
	}
	var rows []model.PartnerAddress
	// 默认的在前面，其次按 id：列表第一行就是单证会取的那一条，
	// 让人一眼看出"打印出去的是哪个地址"。
	if err := q.Order("is_default DESC, id ASC").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("查询主体 %d 的地址: %w", partnerID, err)
	}
	return rows, nil
}

func (r *partnerAddressRepo) FindByID(ctx context.Context, id uint) (*model.PartnerAddress, error) {
	return r.Base.FindByID(ctx, id)
}

func (r *partnerAddressRepo) CountByPartner(ctx context.Context, partnerID uint) (int64, error) {
	return r.Base.Count(ctx, func(db *gorm.DB) *gorm.DB {
		return db.Where("partner_id = ?", partnerID)
	})
}

// Create 新增一条地址；该用途下还没有地址时自动成为默认。
//
// 自动置默认不是"顺手"：如果不这么做，新建完第一条地址后单证仍然取不到它，
// 人会以为地址没保存成功。少一次点击，也少一次"为什么打印出来是空的"。
func (r *partnerAddressRepo) Create(ctx context.Context, m *model.PartnerAddress) error {
	return r.conn(ctx).Transaction(func(tx *gorm.DB) error {
		if !m.IsDefault {
			var count int64
			if err := tx.Unscoped().Model(&model.PartnerAddress{}).
				Where("partner_id = ? AND type = ?", m.PartnerID, m.Type).
				Count(&count).Error; err != nil {
				return fmt.Errorf("统计同用途地址: %w", err)
			}
			m.IsDefault = count == 0
		}
		if m.IsDefault {
			if err := clearAddressDefault(tx, m.PartnerID, m.Type, 0); err != nil {
				return err
			}
		}
		if err := tx.Create(m).Error; err != nil {
			return fmt.Errorf("新增主体地址: %w", err)
		}
		return nil
	})
}

// Update 全量覆盖一条地址，并把"默认"的切换放在同一事务里。
//
// current 是 service 查到的那一行，用来判断两个边界：
// 一是同用途下不能没有默认（取消唯一一条默认要被拒绝），
// 二是 is_default 从假改真时要清掉别人的默认。
func (r *partnerAddressRepo) Update(
	ctx context.Context, current *model.PartnerAddress, fields map[string]any,
) error {
	return r.conn(ctx).Transaction(func(tx *gorm.DB) error {
		if v, ok := fields["is_default"]; ok {
			next, _ := v.(bool)
			if next {
				if err := clearAddressDefault(tx, current.PartnerID, current.Type, current.ID); err != nil {
					return err
				}
			} else if current.IsDefault {
				// 取消默认会导致"这个用途有地址、但没有默认"，单证取址时又得靠猜。
				// 正确做法是把另一条设为默认，它会顺带把这条降级。
				return errs.BadRequest("请把另一条地址设为默认，而不是取消这一条")
			}
		}
		res := tx.Model(&model.PartnerAddress{}).Where("id = ?", current.ID).Updates(fields)
		if res.Error != nil {
			return fmt.Errorf("更新主体地址 id=%d: %w", current.ID, res.Error)
		}
		if res.RowsAffected == 0 {
			return errs.ErrNotFound
		}
		return nil
	})
}

// Delete 删除一条地址；删掉的若是默认，把同用途剩下最早的一条提为默认。
//
// 不做"最后一条不能删"的限制：地址可以整批重录，限制它只会逼人先建再删。
// 但没有默认是不行的——单证取址时不能靠猜。
func (r *partnerAddressRepo) Delete(ctx context.Context, current *model.PartnerAddress) error {
	return r.conn(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Delete(&model.PartnerAddress{}, current.ID).Error; err != nil {
			return fmt.Errorf("删除主体地址 id=%d: %w", current.ID, err)
		}
		if !current.IsDefault {
			return nil
		}
		var next model.PartnerAddress
		err := tx.Where("partner_id = ? AND type = ?", current.PartnerID, current.Type).
			Order("id ASC").First(&next).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// 这个用途下已经没有地址了，没有默认是正确状态。
			return nil
		}
		if err != nil {
			return fmt.Errorf("查找接替的默认地址: %w", err)
		}
		return tx.Model(&model.PartnerAddress{}).Where("id = ?", next.ID).
			Update("is_default", true).Error
	})
}

// clearAddressDefault 清掉同主体同用途的默认标记，exceptID 用于排除自身。
func clearAddressDefault(tx *gorm.DB, partnerID uint, typ model.PartnerAddressType, exceptID uint) error {
	q := tx.Model(&model.PartnerAddress{}).
		Where("partner_id = ? AND type = ? AND is_default = ?", partnerID, typ, true)
	if exceptID > 0 {
		q = q.Where("id <> ?", exceptID)
	}
	if err := q.Update("is_default", false).Error; err != nil {
		return fmt.Errorf("清除原默认地址: %w", err)
	}
	return nil
}
