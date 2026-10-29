package service

import (
	"context"
	"testing"
	"time"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/money"
	"github.com/edwinhuish/gowork/internal/repository"
)

// TestReceivableStatement 是 T-53 的验收点：期初 + 本期发生 − 本期收款 = 期末，
// 且三类发生额（应收 / 贷项 / 收款）都进得去。
//
// 对账单最容易错的地方是**边界**：同一笔账既不能算进期初又算进本期，
// 也不能两头都不算。这里把 9 月与 10 月各放一笔，正是为了卡这条边界。
func TestReceivableStatement(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t,
		&model.Partner{}, &model.Receivable{}, &model.Payment{},
		&model.PaymentAllocation{}, &model.CreditNote{})

	partner := &model.Partner{
		Code: "C001", Name: "Ferreteria El Sol", Role: model.PartnerRoleCustomer,
		EntityType: model.PartnerEntityCompany, Status: model.EnableStatusOn,
	}
	if err := db.Create(partner).Error; err != nil {
		t.Fatalf("建主体失败: %v", err)
	}

	// 9 月：应收 1000，收款 400 → 期初应为 600。
	sep := time.Date(2026, 9, 1, 10, 0, 0, 0, time.Local)
	sepAR := &model.Receivable{Settlement: model.Settlement{
		PartnerID: partner.ID, DocNo: "RC-202609-0001",
		Status: model.SettlementStatusOpen, CurrencyCode: "USD",
		ExchangeRate: money.MustParse("7.1"), Amount: money.MustParse("1000"),
		AmountBase: money.MustParse("7100"), DueDate: model.DateOnly(sep),
	}}
	sepAR.CreatedAt = sep
	if err := db.Create(sepAR).Error; err != nil {
		t.Fatalf("建 9 月应收失败: %v", err)
	}
	sepPay := &model.Payment{
		PartnerID: partner.ID, DocNo: "PM-202609-0001",
		Direction: model.PaymentDirectionIn, CurrencyCode: "USD",
		ExchangeRate: money.MustParse("7.1"), Amount: money.MustParse("400"),
		AmountBase: money.MustParse("2840"), PayDate: model.DateOnly(sep.AddDate(0, 0, 9)),
		Method: "TT",
	}
	if err := db.Create(sepPay).Error; err != nil {
		t.Fatalf("建 9 月收款失败: %v", err)
	}

	// 10 月：应收 600、贷项 100（冲 9 月那张）。
	oct := time.Date(2026, 10, 5, 10, 0, 0, 0, time.Local)
	octAR := &model.Receivable{Settlement: model.Settlement{
		PartnerID: partner.ID, DocNo: "RC-202610-0001",
		Status: model.SettlementStatusOpen, CurrencyCode: "USD",
		ExchangeRate: money.MustParse("7.2"), Amount: money.MustParse("600"),
		AmountBase: money.MustParse("4320"), DueDate: model.DateOnly(oct),
	}}
	octAR.CreatedAt = oct
	if err := db.Create(octAR).Error; err != nil {
		t.Fatalf("建 10 月应收失败: %v", err)
	}
	note := &model.CreditNote{
		DocNo: "CN-202610-0001", ReceivableID: sepAR.ID,
		Amount: money.MustParse("100"), CurrencyCode: "USD",
		ExchangeRate: money.MustParse("7.1"), AmountBase: money.MustParse("710"),
		IssuedDate: model.DateOnly(oct.AddDate(0, 0, 3)),
	}
	if err := db.Create(note).Error; err != nil {
		t.Fatalf("建贷项失败: %v", err)
	}

	svc := NewStatementService(
		repository.NewStatementRepository(db), repository.NewPartnerRepository(db))

	got, err := svc.Statement(ctx, dto.StatementQuery{
		PartnerID: partner.ID, Currency: "USD", From: "2026-10-01", To: "2026-10-31",
	})
	if err != nil {
		t.Fatalf("出对账单失败: %v", err)
	}

	// 期初 = 9 月的 1000 − 400。
	if got.Opening.Cmp(money.MustParse("600")) != 0 {
		t.Errorf("期初应为 600，得到 %s", got.Opening)
	}
	// 本期发生 600、本期冲减 100（贷项算在"减"这一侧）。
	if got.Debit.Cmp(money.MustParse("600")) != 0 {
		t.Errorf("本期发生应为 600，得到 %s", got.Debit)
	}
	if got.Credit.Cmp(money.MustParse("100")) != 0 {
		t.Errorf("本期冲减应为 100，得到 %s", got.Credit)
	}
	// 期末 = 600 + 600 − 100。
	if got.Closing.Cmp(money.MustParse("1100")) != 0 {
		t.Errorf("期末应为 1100，得到 %s", got.Closing)
	}

	// 明细只含本期两笔：9 月的已经折进期初，再列出来就是重复。
	if len(got.Lines) != 2 {
		t.Fatalf("本期明细应有 2 条，得到 %d", len(got.Lines))
	}
	if got.Lines[0].DocDate > got.Lines[1].DocDate {
		t.Errorf("明细应按日期升序，得到 %s 在 %s 之前", got.Lines[0].DocDate, got.Lines[1].DocDate)
	}

	// 不传区间：期初为 0，四笔（两张应收、一笔收款、一笔贷项）都在明细里，期末同上。
	all, err := svc.Statement(ctx, dto.StatementQuery{PartnerID: partner.ID, Currency: "USD"})
	if err != nil {
		t.Fatalf("全区间对账单失败: %v", err)
	}
	if !all.Opening.IsZero() {
		t.Errorf("不传起始日时期初应为 0，得到 %s", all.Opening)
	}
	if len(all.Lines) != 4 {
		t.Errorf("全区间应有 4 条明细，得到 %d", len(all.Lines))
	}
	if all.Closing.Cmp(got.Closing) != 0 {
		t.Errorf("全区间期末应与 10 月期末一致：%s vs %s", all.Closing, got.Closing)
	}
}
