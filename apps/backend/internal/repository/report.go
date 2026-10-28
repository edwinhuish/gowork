package repository

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/errs"
)

// OutstandingSettlement 是一张还没结清的单据，账龄报表的原料。
//
// 只取报表需要的列：报表要的是"谁欠多少、什么时候到期"，
// 把整张应收单（含来源单号、备注、状态机）读进来只会让聚合多背一份内存。
type OutstandingSettlement struct {
	PartnerID         uint
	PartnerCode       string
	PartnerName       string
	CurrencyCode      string
	Amount            float64
	SettledAmount     float64
	AmountBase        float64
	SettledAmountBase float64
	// DueDate 只用到日期部分；分桶按整天算，带时分秒会让边界日期飘到隔壁桶。
	DueDate time.Time
}

// ReportRepository 是只读聚合的入口：账龄（T-22）与单票毛利（T-22）。
//
// 单独成一个仓储而不是往应收应付里加方法：报表横跨多张表（单据 + 主体 +
// 采购行 + 费用），而应收应付仓储的职责是"守各自单据的写入规则"。
// 混在一起，写路径的约束会被报表的读需求牵着走。
type ReportRepository interface {
	// ListOutstandingSettlements 列出未结清且未作废的应收或应付。
	//
	// 过滤条件里的 amount > settled_amount 用 0.0001 的余量：分批收付累加到
	// 最后一笔时可能差一个末位，那种单子已经结清，不该再出现在账龄里。
	ListOutstandingSettlements(
		ctx context.Context, target model.SettlementTarget,
	) ([]OutstandingSettlement, error)
}

type reportRepo struct {
	*Base[model.Receivable]
}

func NewReportRepository(db *gorm.DB) ReportRepository {
	return &reportRepo{Base: NewBase[model.Receivable](db)}
}

func (r *reportRepo) ListOutstandingSettlements(
	ctx context.Context, target model.SettlementTarget,
) ([]OutstandingSettlement, error) {
	table, err := settlementTable(target)
	if err != nil {
		return nil, err
	}

	rows := make([]OutstandingSettlement, 0, 64)
	// 用 Table 而不是 Model：两张表结构相同，写成 Model(&model.Receivable{}) +
	// 切表名反而绕。代价是软删除条件要自己写 —— 漏了就会把已删的单据算进账龄。
	err = r.DB().WithContext(ctx).
		Table(table+" AS s").
		Select(`s.partner_id,
			p.code AS partner_code,
			p.name AS partner_name,
			s.currency_code,
			s.amount,
			s.settled_amount,
			s.amount_base,
			s.settled_amount_base,
			s.due_date`).
		Joins("LEFT JOIN md_partner AS p ON p.id = s.partner_id AND p.deleted_at IS NULL").
		Where("s.deleted_at IS NULL").
		Where("s.status <> ?", model.SettlementStatusVoid).
		Where("s.amount - s.settled_amount > ?", 0.0001).
		Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("查询未结清单据(%s): %w", table, err)
	}
	return rows, nil
}

// settlementTable 把核销对象翻译成表名。非法取值在入口就拒掉：
// 静默回退到应收会让"付款方向的账龄"悄悄变成客户欠款。
func settlementTable(target model.SettlementTarget) (string, error) {
	switch target {
	case model.SettlementTargetReceivable:
		return "biz_receivable", nil
	case model.SettlementTargetPayable:
		return "biz_payable", nil
	default:
		return "", errs.BadRequest("核销对象类型不合法：10=应收单，20=应付单")
	}
}
