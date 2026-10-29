package service

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/money"
	"github.com/edwinhuish/gowork/internal/repository"
)

// TestOwnerScopeFiltersDocuments 验证单据范围真的从客户归属继承：
// 归属只存主体上，单据靠 partner_id 关联过去，所以这里直接打仓储。
//
// 六类单据（报价 / 订单 / 合同 / 应收 / 应付 / 收付款）共用同一个 OwnerScope，
// 用订单验一次就够——SQL 片段是同一段，各自只是列名不同，而列名都是 partner_id。
func TestOwnerScopeFiltersDocuments(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t, &model.Partner{}, &model.SalesOrder{})
	partnerRepo := repository.NewPartnerRepository(db)
	orderRepo := repository.NewSalesOrderRepository(db)

	addPartner := func(code string, ownerID uint) uint {
		p := &model.Partner{
			Code: code, Name: code, Role: model.PartnerRoleCustomer,
			EntityType: model.PartnerEntityCompany, Status: model.EnableStatusOn,
			OwnerID: ownerID,
		}
		if err := db.Create(p).Error; err != nil {
			t.Fatalf("建主体失败: %v", err)
		}
		return p.ID
	}
	alice := addPartner("CUS001", 1)
	bob := addPartner("CUS002", 2)
	public := addPartner("CUS003", 0)

	for i, partnerID := range []uint{alice, bob, public} {
		order := &model.SalesOrder{
			DocNo: fmt.Sprintf("SO-%03d", i), PartnerID: partnerID,
			TradeTerm: model.TradeTermFOB, Status: model.DocumentStatusConfirmed,
			OrderDate:    model.DateOnly(time.Date(2026, 10, 1, 0, 0, 0, 0, time.Local)),
			CurrencyCode: "USD", ExchangeRate: money.MustParse("7.1"),
			AmountTotal: money.FromInt(100), AmountBaseTotal: money.FromInt(710),
		}
		if err := db.Create(order).Error; err != nil {
			t.Fatalf("建订单失败: %v", err)
		}
	}

	count := func(f repository.SalesOrderFilter) int64 {
		_, total, err := orderRepo.List(ctx, f, repository.Pagination{Page: 1, PageSize: 50})
		if err != nil {
			t.Fatalf("查订单失败: %v", err)
		}
		return total
	}

	if got := count(repository.SalesOrderFilter{}); got != 3 {
		t.Fatalf("不限范围时应看到 3 张，得到 %d", got)
	}

	aliceID := uint(1)
	scoped := count(repository.SalesOrderFilter{OwnerID: &aliceID})
	if scoped != 2 {
		t.Errorf("业务员应看到自己的 + 未分配的共 2 张，得到 %d", scoped)
	}

	// 未分配客户的单据对任何业务员都可见——这正是"刚上线的存量数据"那一堆，
	// 藏起来会让人以为数据丢了。
	bobID := uint(2)
	if got := count(repository.SalesOrderFilter{OwnerID: &bobID}); got != 2 {
		t.Errorf("另一个业务员也应看到 2 张，得到 %d", got)
	}

	// 主体被软删之后，它的单据仍应算在范围内（客户删了不等于单子不属于你）。
	if err := partnerRepo.Delete(ctx, bob); err != nil {
		t.Fatalf("软删主体失败: %v", err)
	}
	if got := count(repository.SalesOrderFilter{OwnerID: &bobID}); got != 2 {
		t.Errorf("主体被软删后范围不该变，得到 %d", got)
	}
}
