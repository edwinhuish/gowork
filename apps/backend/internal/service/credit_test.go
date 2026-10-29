package service

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/errs"
	"github.com/edwinhuish/gowork/internal/pkg/money"
	"github.com/edwinhuish/gowork/internal/repository"
)

type creditFixture struct {
	credits    CreditService
	orders     SalesOrderService
	db         *gorm.DB
	customerID uint
	now        time.Time
	// log 收集信用放行的 warn 日志：留痕是验收项之一，不能只看"返回成功"。
	log *bytes.Buffer
}

// newCreditFixture 造一个客户（额度可调）与一套能确认订单的环境。
//
// 订单直接写库，不走 Create：这里要看的是"确认这一步拦不拦"，
// 而下单路径（单号、币种、明细校验）在 T-10 已经各自测过。
func newCreditFixture(t *testing.T, now time.Time, limit float64) creditFixture {
	t.Helper()
	ctx := context.Background()

	// 出货两张表要一起建：订单详情会带上"已出数量"，而那是从分配行算的。
	db := openTestDB(t,
		&model.Partner{}, &model.SalesOrder{}, &model.SalesOrderItem{},
		&model.Shipment{}, &model.ShipmentAllocation{},
		&model.Receivable{}, &model.Payable{})

	partnerRepo := repository.NewPartnerRepository(db)
	customer := &model.Partner{
		Code: "C001", Name: "Ferreteria El Sol", Role: model.PartnerRoleCustomer,
		EntityType: model.PartnerEntityCompany, Status: model.EnableStatusOn,
		CreditLimit: money.FromFloat(limit), CreditDays: 30,
	}
	if err := partnerRepo.Create(ctx, customer); err != nil {
		t.Fatalf("建客户失败: %v", err)
	}

	buf := &bytes.Buffer{}
	clock := func() time.Time { return now }
	credits := NewCreditService(
		repository.NewCreditRepository(db), partnerRepo,
		WithCreditLogger(slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{
			Level: slog.LevelWarn,
		}))))

	orders := NewSalesOrderService(
		repository.NewSalesOrderRepository(db), partnerRepo, repository.NewProductRepository(db),
		repository.NewShipmentRepository(db), nil, nil, credits,
		WithSalesOrderClock(clock))

	return creditFixture{
		credits:    credits,
		orders:     orders,
		db:         db,
		customerID: customer.ID,
		now:        now,
		log:        buf,
	}
}

// addOrder 写一张订单。金额与状态直接给定，便于精确摆出"差一点超额"的边界。
func (f creditFixture) addOrder(
	t *testing.T, docNo string, status model.DocumentStatus, amountBase float64,
) uint {
	t.Helper()
	m := &model.SalesOrder{
		DocNo: docNo, PartnerID: f.customerID, TradeTerm: model.TradeTermFOB,
		Status: status, OrderDate: model.DateOnly(f.now),
		CurrencyCode: "CNY", ExchangeRate: money.FromInt(1),
		AmountTotal: money.FromFloat(amountBase), AmountBaseTotal: money.FromFloat(amountBase),
	}
	if err := f.db.Create(m).Error; err != nil {
		t.Fatalf("建订单 %s 失败: %v", docNo, err)
	}
	return m.ID
}

// addReceivable 写一张应收（挂某张订单），用来验证敞口里"已开票未收款"那一块。
func (f creditFixture) addReceivable(
	t *testing.T, docNo string, sourceID uint, amountBase, settledBase float64,
	status model.SettlementStatus,
) {
	t.Helper()
	m := &model.Receivable{
		Settlement: model.Settlement{
			DocNo: docNo, PartnerID: f.customerID,
			SourceType: model.SettlementSourceSalesOrder, SourceID: sourceID, SourceDocNo: "SO-TEST",
			CurrencyCode: "CNY", ExchangeRate: money.FromInt(1),
			Amount: money.FromFloat(amountBase), AmountBase: money.FromFloat(amountBase),
			SettledAmount:     money.FromFloat(settledBase),
			SettledAmountBase: money.FromFloat(settledBase),
			DueDate: model.DateOnly(f.now), Status: status,
		},
	}
	if err := f.db.Create(m).Error; err != nil {
		t.Fatalf("建应收 %s 失败: %v", docNo, err)
	}
}

func actorCtx(role string) context.Context {
	return WithActor(context.Background(), &Principal{
		UserID: 1, Username: "tester", Role: role,
	})
}

func (f creditFixture) confirm(ctx context.Context, id uint, force bool) (*model.SalesOrder, error) {
	return f.orders.UpdateStatus(ctx, id, dto.SalesOrderStatusInput{
		Status: int8(model.DocumentStatusConfirmed), ForceCredit: force,
	})
}

func (f creditFixture) statusOf(t *testing.T, id uint) model.DocumentStatus {
	t.Helper()
	var m model.SalesOrder
	if err := f.db.First(&m, id).Error; err != nil {
		t.Fatalf("读订单失败: %v", err)
	}
	return m.Status
}

// TestCreditSummaryCountsOpenReceivableAndUnbilledOrder 守敞口口径：
// 已开票未收的钱 + 已确认还没开票的货，两块都要算，且不重复算。
func TestCreditSummaryCountsOpenReceivableAndUnbilledOrder(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 10, 28, 9, 0, 0, 0, time.Local)
	f := newCreditFixture(t, now, 10000)

	// 订单 1 已开票：5000 里收了 2000，剩 3000 进敞口；订单本身不再算"未开票"。
	billed := f.addOrder(t, "SO-CR-1", model.DocumentStatusConfirmed, 5000)
	f.addReceivable(t, "RC-CR-1", billed, 5000, 2000, model.SettlementStatusPartial)
	// 订单 2 已确认未开票：2000 进敞口。
	f.addOrder(t, "SO-CR-2", model.DocumentStatusConfirmed, 2000)
	// 草稿不占额度；作废的应收也不算。
	f.addOrder(t, "SO-CR-3", model.DocumentStatusDraft, 9999)
	f.addReceivable(t, "RC-CR-VOID", 999, 4000, 0, model.SettlementStatusVoid)

	sum, err := f.credits.Summary(ctx, f.customerID)
	if err != nil {
		t.Fatalf("查额度失败: %v", err)
	}
	wantDecimal(t, "未结清应收", sum.OpenReceivable, "3000")
	wantDecimal(t, "未开票订单", sum.UnbilledOrder, "2000")
	wantDecimal(t, "敞口", sum.Exposure, "5000")
	wantDecimal(t, "可用额度", sum.Available, "5000")
	if sum.Unlimited {
		t.Error("额度 10000 不该标成不限")
	}
}

// TestCreditSummarizeUnlimited：额度 0 表示不限，可用额度给 0 而不是负的"已超"。
func TestCreditSummaryUnlimited(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 10, 28, 9, 0, 0, 0, time.Local)
	f := newCreditFixture(t, now, 0)

	f.addOrder(t, "SO-CR-1", model.DocumentStatusConfirmed, 50000)

	sum, err := f.credits.Summary(ctx, f.customerID)
	if err != nil {
		t.Fatalf("查额度失败: %v", err)
	}
	if !sum.Unlimited {
		t.Error("额度 0 应标成不限")
	}
	if !sum.Available.IsZero() {
		t.Errorf("不限额度时可用额度应为 0，得到 %s", sum.Available)
	}
	wantDecimal(t, "敞口仍照实给出", sum.Exposure, "50000")
}

// TestCreditConfirmBlocksOverLimit 是 T-39 的验收点：超额确认返回 409，
// 订单留在草稿——拦不住的那一半等于没拦。
func TestCreditConfirmBlocksOverLimit(t *testing.T) {
	now := time.Date(2026, 10, 28, 9, 0, 0, 0, time.Local)
	f := newCreditFixture(t, now, 10000)

	f.addOrder(t, "SO-OLD", model.DocumentStatusConfirmed, 8000)
	draft := f.addOrder(t, "SO-NEW", model.DocumentStatusDraft, 3000)

	_, err := f.confirm(actorCtx(model.RoleStaff), draft, false)
	if err == nil {
		t.Fatal("超额确认应被拒绝")
	}
	e, ok := errs.From(err)
	if !ok || e.Code != errs.CodeConflict {
		t.Errorf("超额应返回 409（%d），得到 %v", errs.CodeConflict, err)
	}
	if !strings.Contains(err.Error(), "超过客户信用额度") {
		t.Errorf("错误文案应说明超额，得到 %q", err.Error())
	}
	if got := f.statusOf(t, draft); got != model.DocumentStatusDraft {
		t.Errorf("被拒后订单应留在草稿，得到 %s", got.Label())
	}
}

// TestCreditConfirmAtLimit：刚好用满是允许的，不能把 >= 写成 >。
func TestCreditConfirmAllowsAtLimit(t *testing.T) {
	now := time.Date(2026, 10, 28, 9, 0, 0, 0, time.Local)
	f := newCreditFixture(t, now, 10000)

	f.addOrder(t, "SO-OLD", model.DocumentStatusConfirmed, 8000)
	draft := f.addOrder(t, "SO-NEW", model.DocumentStatusDraft, 2000)

	if _, err := f.confirm(actorCtx(model.RoleStaff), draft, false); err != nil {
		t.Fatalf("刚好用满额度应放行，得到 %v", err)
	}
	if got := f.statusOf(t, draft); got != model.DocumentStatusConfirmed {
		t.Errorf("应当已确认，得到 %s", got.Label())
	}
}

func TestCreditUnlimitedConfirmAlwaysPasses(t *testing.T) {
	now := time.Date(2026, 10, 28, 9, 0, 0, 0, time.Local)
	f := newCreditFixture(t, now, 0)

	draft := f.addOrder(t, "SO-BIG", model.DocumentStatusDraft, 1000000)
	if _, err := f.confirm(actorCtx(model.RoleStaff), draft, false); err != nil {
		t.Fatalf("不限额度时确认应通过，得到 %v", err)
	}
}

// TestCreditForceOverrideOnlyAdmin：强制放行只对 admin 开放，
// 员工传了 force 也只能拿到 403；admin 放行要落一条 warn 日志。
func TestCreditForceOverrideOnlyAdmin(t *testing.T) {
	now := time.Date(2026, 10, 28, 9, 0, 0, 0, time.Local)
	f := newCreditFixture(t, now, 1000)

	draft := f.addOrder(t, "SO-NEW", model.DocumentStatusDraft, 5000)

	_, err := f.confirm(actorCtx(model.RoleStaff), draft, true)
	if err == nil {
		t.Fatal("非 admin 强制放行应被拒绝")
	}
	if e, ok := errs.From(err); !ok || e.Code != errs.CodeForbidden {
		t.Errorf("非 admin 放行应返回 403（%d），得到 %v", errs.CodeForbidden, err)
	}
	if got := f.statusOf(t, draft); got != model.DocumentStatusDraft {
		t.Errorf("被拒后订单应留在草稿，得到 %s", got.Label())
	}

	if _, err := f.confirm(actorCtx(model.RoleAdmin), draft, true); err != nil {
		t.Fatalf("admin 强制放行应通过，得到 %v", err)
	}
	if got := f.statusOf(t, draft); got != model.DocumentStatusConfirmed {
		t.Errorf("放行后应已确认，得到 %s", got.Label())
	}

	logged := f.log.String()
	for _, want := range []string{"信用额度强制放行", "SO-NEW", "tester"} {
		if !strings.Contains(logged, want) {
			t.Errorf("放行日志应包含 %q，实际日志: %s", want, logged)
		}
	}
}

// TestCreditConfirmWithoutActor：没有身份（内部调用）时不允许放行——
// 缺身份不等于管理员。
func TestCreditConfirmWithoutActor(t *testing.T) {
	now := time.Date(2026, 10, 28, 9, 0, 0, 0, time.Local)
	f := newCreditFixture(t, now, 1000)

	draft := f.addOrder(t, "SO-NEW", model.DocumentStatusDraft, 5000)
	if _, err := f.confirm(context.Background(), draft, true); err == nil {
		t.Fatal("没有登录身份时无条件放行应被拒绝")
	} else if e, ok := errs.From(err); !ok || e.Code != errs.CodeForbidden {
		t.Errorf("应返回 403，得到 %v", err)
	}
}
