package service

import (
	"context"

	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/repository"
)

// fillSettlementAllocations 给应收 / 应付详情补上"核销这张单的钱是哪几笔"。
//
// 只给详情用：列表页每行都去查一次核销表就是 N+1，而列表上本来也不显示它。
//
// 单号与 id 分开传：Settlement 是两张表共有的那段列，不自带主键（主键在
// Receivable / Payable 上），所以这里要调用方把 id 递进来。
//
// 仓储为 nil 时静默跳过——那表示装配没给（老测试只关心单据本身），
// 不是业务缺失；查询**出错**则要把错抛上去：空数组的含义是"还没收到钱"，
// 拿它冒充"查不到"会让一张已收款的单看起来像没收过。
func fillSettlementAllocations(
	ctx context.Context,
	repo repository.PaymentRepository,
	target model.SettlementTarget,
	targetID uint,
	s *model.Settlement,
) error {
	if repo == nil || s == nil || targetID == 0 {
		return nil
	}
	rows, err := repo.ListAllocationsByTarget(ctx, target, targetID)
	if err != nil {
		return err
	}
	s.Allocations = rows
	return nil
}
