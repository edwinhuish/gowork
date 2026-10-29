package repository

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/errs"
	"github.com/edwinhuish/gowork/internal/pkg/money"
)

// PaymentFilter 是收付款单的查询条件。
type PaymentFilter struct {
	Direction *model.PaymentDirection
	PartnerID *uint
	// OwnerID 只看归属该业务员的对方主体的收付款（含未分配的）。
	OwnerID *uint
}

func (f PaymentFilter) scopes() []Scope {
	scopes := make([]Scope, 0, 2)
	if f.Direction != nil {
		d := *f.Direction
		scopes = append(scopes, func(db *gorm.DB) *gorm.DB {
			return db.Where("direction = ?", d)
		})
	}
	if f.PartnerID != nil {
		pid := *f.PartnerID
		scopes = append(scopes, func(db *gorm.DB) *gorm.DB {
			return db.Where("partner_id = ?", pid)
		})
	}
	if f.OwnerID != nil {
		scopes = append(scopes, OwnerScope("partner_id", *f.OwnerID))
	}
	return scopes
}

type PaymentRepository interface {
	// Create 在一个事务里写入收付款单、核销行，并回写目标的已核销金额与状态。
	//
	// 校验之所以也在事务里：超收的判定依赖"当前已核销多少"，
	// 在事务外读完再写，中间隔着另一个请求就会双写成功。
	Create(ctx context.Context, p *model.Payment) error
	// Delete 在一个事务里删除收付款单与核销行，并把目标的已核销金额减回去。
	Delete(ctx context.Context, id uint) error
	// RemoveAllocation 撤销一条核销行（T-52 的第一块）。
	RemoveAllocation(ctx context.Context, paymentID, allocationID uint) error
	// Update 只改不涉及金额的字段（日期、方式、水单号、备注）。
	Update(ctx context.Context, id uint, fields map[string]any) error
	FindByID(ctx context.Context, id uint) (*model.Payment, error)
	List(ctx context.Context, f PaymentFilter, p Pagination) ([]model.Payment, int64, error)
}

type paymentRepo struct {
	*Base[model.Payment]
}

func NewPaymentRepository(db *gorm.DB) PaymentRepository {
	return &paymentRepo{Base: NewBase[model.Payment](db)}
}

func (r *paymentRepo) Create(ctx context.Context, p *model.Payment) error {
	return r.conn(ctx).Transaction(func(tx *gorm.DB) error {
		// Omit 掉关联：gorm 默认会在 Create 时顺手把 Allocations 也插进去，
		// 那样下面的循环会对着同一批行再插一次（主键冲突），而且拿不到
		// 「逐行校验再回写目标」的机会——核销必须自己控制写入顺序。
		if err := tx.Omit("Allocations").Create(p).Error; err != nil {
			return fmt.Errorf("写入收付款单: %w", err)
		}
		for i := range p.Allocations {
			a := &p.Allocations[i]
			a.PaymentID = p.ID
			if err := tx.Create(a).Error; err != nil {
				return fmt.Errorf("写入核销行: %w", err)
			}
			if err := applyAllocation(tx, *a); err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *paymentRepo) Delete(ctx context.Context, id uint) error {
	return r.conn(ctx).Transaction(func(tx *gorm.DB) error {
		var p model.Payment
		err := tx.Preload("Allocations").First(&p, id).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errs.ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("查询收付款单 id=%d: %w", id, err)
		}

		for _, a := range p.Allocations {
			// 回滚方向与核销相反：把已核销金额减回去，状态由新金额重算。
			if err := reverseAllocation(tx, a); err != nil {
				return err
			}
		}
		if err := tx.Where("payment_id = ?", id).Delete(&model.PaymentAllocation{}).Error; err != nil {
			return fmt.Errorf("删除核销行: %w", err)
		}
		if err := tx.Delete(&model.Payment{}, id).Error; err != nil {
			return fmt.Errorf("删除收付款单 id=%d: %w", id, err)
		}
		return nil
	})
}

// RemoveAllocation 撤销单条核销（T-52）。
//
// 在此之前撤销核销只有一条路：删掉整张收付款单。于是一笔核销录错，
// 同一张单上其它正确的核销也一起消失——审计里只剩一条"删除"。
//
// 只反转这一行、只删这一行：收付款单本身不动，金额也不必回写——
// 单子上的已核销总额是查询时现算的，没有一份会过期的冗余。
func (r *paymentRepo) RemoveAllocation(ctx context.Context, paymentID, allocationID uint) error {
	return r.conn(ctx).Transaction(func(tx *gorm.DB) error {
		var a model.PaymentAllocation
		err := tx.Where("id = ? AND payment_id = ?", allocationID, paymentID).First(&a).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errs.ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("查询核销行 id=%d: %w", allocationID, err)
		}
		if err := reverseAllocation(tx, a); err != nil {
			return err
		}
		if err := tx.Delete(&model.PaymentAllocation{}, a.ID).Error; err != nil {
			return fmt.Errorf("删除核销行 id=%d: %w", a.ID, err)
		}
		return nil
	})
}

func (r *paymentRepo) Update(ctx context.Context, id uint, fields map[string]any) error {
	return r.Base.UpdateFields(ctx, id, fields)
}

func (r *paymentRepo) FindByID(ctx context.Context, id uint) (*model.Payment, error) {
	var m model.Payment
	err := r.conn(ctx).
		Preload("Partner").
		Preload("Allocations", func(db *gorm.DB) *gorm.DB { return db.Order("id ASC") }).
		First(&m, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errs.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("查询收付款单 id=%d: %w", id, err)
	}
	return &m, nil
}

// List 按到账日倒序：这张表是"最近的钱都在哪"，与应收应付的"哪天该收"是两种读法。
func (r *paymentRepo) List(
	ctx context.Context, f PaymentFilter, p Pagination,
) ([]model.Payment, int64, error) {
	scopes := append(f.scopes(), func(db *gorm.DB) *gorm.DB {
		return db.Preload("Partner").
			Preload("Allocations", func(db *gorm.DB) *gorm.DB { return db.Order("id ASC") })
	})
	return r.PaginateOrdered(ctx, p, "pay_date DESC, id DESC", scopes...)
}

// applyAllocation 校验一行核销并回写目标的已核销金额。
//
// 三条校验都在这里而不是 service：它们要读"目标当前的已核销金额"，
// 而这个值就在同一个事务里被改。放到事务外做，检查与写入之间会开一个窗口。
func applyAllocation(tx *gorm.DB, a model.PaymentAllocation) error {
	switch a.TargetType {
	case model.SettlementTargetReceivable:
		var m model.Receivable
		err := tx.First(&m, a.TargetID).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errs.BadRequest(fmt.Sprintf("应收单 %d 不存在", a.TargetID))
		}
		if err != nil {
			return fmt.Errorf("查询应收单 id=%d: %w", a.TargetID, err)
		}
		if m.Status == model.SettlementStatusVoid {
			return errs.ErrConflict.WithMsg(fmt.Sprintf("应收单 %s 已作废，不能核销", m.DocNo))
		}
		if a.Amount.Cmp(money.Zero()) <= 0 {
			return errs.BadRequest("核销金额必须大于 0")
		}
		// 未结清额还要扣掉已开的贷项（T-52b）：退货冲减掉的那部分钱不能再收一次。
		credited, err := creditedFor(tx, m.ID)
		if err != nil {
			return err
		}
		// 超收判定逐位比较：金额与核销额都按末位精确存取，
		// 多核 0.0001 也是多核，放过去就成了"这张单收的钱比开的票多"。
		if remaining := m.Amount.Sub(m.SettledAmount).Sub(credited); a.Amount.Cmp(remaining) > 0 {
			return errs.BadRequest(fmt.Sprintf(
				"应收单 %s 只剩 %s %s 未结清，不能核销 %s",
				m.DocNo, m.CurrencyCode, remaining, a.Amount))
		}
		settled := m.SettledAmount.Add(a.Amount)
		return tx.Model(&model.Receivable{}).Where("id = ?", m.ID).Updates(map[string]any{
			"settled_amount":      settled,
			"settled_amount_base": m.SettledAmountBase.Add(a.AmountBase),
			"status":              model.SettlementStatusFor(m.Amount, settled),
		}).Error

	case model.SettlementTargetPayable:
		var m model.Payable
		err := tx.First(&m, a.TargetID).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errs.BadRequest(fmt.Sprintf("应付单 %d 不存在", a.TargetID))
		}
		if err != nil {
			return fmt.Errorf("查询应付单 id=%d: %w", a.TargetID, err)
		}
		if m.Status == model.SettlementStatusVoid {
			return errs.ErrConflict.WithMsg(fmt.Sprintf("应付单 %s 已作废，不能核销", m.DocNo))
		}
		if a.Amount.Cmp(money.Zero()) <= 0 {
			return errs.BadRequest("核销金额必须大于 0")
		}
		if remaining := m.Amount.Sub(m.SettledAmount); a.Amount.Cmp(remaining) > 0 {
			return errs.BadRequest(fmt.Sprintf(
				"应付单 %s 只剩 %s %s 未结清，不能核销 %s",
				m.DocNo, m.CurrencyCode, remaining, a.Amount))
		}
		settled := m.SettledAmount.Add(a.Amount)
		return tx.Model(&model.Payable{}).Where("id = ?", m.ID).Updates(map[string]any{
			"settled_amount":      settled,
			"settled_amount_base": m.SettledAmountBase.Add(a.AmountBase),
			"status":              model.SettlementStatusFor(m.Amount, settled),
		}).Error

	default:
		return errs.BadRequest("核销对象类型不合法：10=应收单，20=应付单")
	}
}

// reverseAllocation 是 applyAllocation 的逆操作，删单时用。
//
// 减完不能是负数：负数只可能来自数据本身已经不一致，
// 那时候先报错比把金额改成一个更离谱的值有用。
func reverseAllocation(tx *gorm.DB, a model.PaymentAllocation) error {
	switch a.TargetType {
	case model.SettlementTargetReceivable:
		var m model.Receivable
		if err := tx.First(&m, a.TargetID).Error; err != nil {
			return fmt.Errorf("回滚时查询应收单 id=%d: %w", a.TargetID, err)
		}
		settled := m.SettledAmount.Sub(a.Amount)
		if settled.IsNegative() {
			return fmt.Errorf("应收单 %s 的已核销金额不足以回滚", m.DocNo)
		}
		return tx.Model(&model.Receivable{}).Where("id = ?", m.ID).Updates(map[string]any{
			"settled_amount":      settled,
			"settled_amount_base": maxDecimal(m.SettledAmountBase.Sub(a.AmountBase), money.Zero()),
			"status":              model.SettlementStatusFor(m.Amount, settled),
		}).Error

	case model.SettlementTargetPayable:
		var m model.Payable
		if err := tx.First(&m, a.TargetID).Error; err != nil {
			return fmt.Errorf("回滚时查询应付单 id=%d: %w", a.TargetID, err)
		}
		settled := m.SettledAmount.Sub(a.Amount)
		if settled.IsNegative() {
			return fmt.Errorf("应付单 %s 的已核销金额不足以回滚", m.DocNo)
		}
		return tx.Model(&model.Payable{}).Where("id = ?", m.ID).Updates(map[string]any{
			"settled_amount":      settled,
			"settled_amount_base": maxDecimal(m.SettledAmountBase.Sub(a.AmountBase), money.Zero()),
			"status":              model.SettlementStatusFor(m.Amount, settled),
		}).Error

	default:
		return fmt.Errorf("未知的核销对象类型 %d", a.TargetType)
	}
}

func maxDecimal(a, b money.Decimal) money.Decimal {
	if a.Cmp(b) > 0 {
		return a
	}
	return b
}

// creditedFor 汇总某张应收单已被贷项冲减的金额（T-52b）。
//
// 在事务里直接查表而不是注入仓储：applyAllocation 是包内函数，
// 为一个辅助查询多一个构造参数，会让所有调用点跟着改。
func creditedFor(tx *gorm.DB, receivableID uint) (money.Decimal, error) {
	var rows []model.CreditNote
	if err := tx.Select("amount").Where("receivable_id = ?", receivableID).Find(&rows).Error; err != nil {
		return money.Zero(), fmt.Errorf("汇总应收单 %d 的贷项: %w", receivableID, err)
	}
	sum := money.Zero()
	for _, row := range rows {
		sum = sum.Add(row.Amount)
	}
	return sum, nil
}
