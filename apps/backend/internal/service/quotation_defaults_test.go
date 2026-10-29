package service

import (
	"context"
	"testing"
	"time"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/money"
)

// TestQuotationInheritsPartnerDefaults 覆盖 T-49 的最后一步：
// 主体的贸易术语与默认币种终于有人读了——报价不填这两项时取客户默认，
// 显式传值仍然优先，客户也没有默认时才报错。
//
// 这条链路的实际意义：老客户一贯 FOB + 美元，业务员每张报价都要重填一遍，
// 而这个字段早就存在客户档案里。填了不生效，比没有这个字段更误导人。
func TestQuotationInheritsPartnerDefaults(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 10, 29, 10, 0, 0, 0, time.Local)
	fx := newQuotationFixture(t, &now)

	str := func(s string) *string { return &s }
	if _, err := fx.partners.Update(ctx, fx.customerID, dto.PartnerInput{
		Code: "C001", Name: "Ferreteria El Sol",
		TradeTerm: str("FOB"), CurrencyCode: str("USD"),
	}); err != nil {
		t.Fatalf("设置客户默认值失败: %v", err)
	}

	items := []dto.QuotationItemInput{
		{ProductID: fx.productID, Qty: qtyOf(100), UnitPrice: money.MustParse("10")},
	}

	// ① 两项都不填：都取客户默认。
	got, err := fx.svc.Create(ctx, dto.QuotationInput{
		PartnerID:  fx.customerID,
		ValidUntil: "2026-11-30",
		Items:      items,
	})
	if err != nil {
		t.Fatalf("不填贸易术语与币种应能取默认值，实际失败: %v", err)
	}
	if got.Quotation.TradeTerm != model.TradeTerm("FOB") {
		t.Errorf("贸易术语应取客户默认 FOB，得到 %q", got.Quotation.TradeTerm)
	}
	if got.Quotation.CurrencyCode != "USD" {
		t.Errorf("币种应取客户默认 USD，得到 %q", got.Quotation.CurrencyCode)
	}

	// ② 显式传值优先于默认：CIF + CNY 照填。
	got, err = fx.svc.Create(ctx, dto.QuotationInput{
		PartnerID:    fx.customerID,
		TradeTerm:    "CIF",
		CurrencyCode: "CNY",
		ValidUntil:   "2026-11-30",
		Items:        items,
	})
	if err != nil {
		t.Fatalf("显式传值应成功: %v", err)
	}
	if got.Quotation.TradeTerm != model.TradeTerm("CIF") || got.Quotation.CurrencyCode != "CNY" {
		t.Errorf("显式传值应优先，得到 %q / %q",
			got.Quotation.TradeTerm, got.Quotation.CurrencyCode)
	}

	// ③ 客户没有默认、单上也没填：必须报错，而不是把空术语存进去。
	// 空术语印在报价单上，客户会来问这一项到底含不含运费。
	roleValue := int8(model.PartnerRoleCustomer)
	plain, err := fx.partners.Create(ctx, dto.PartnerInput{
		Code: "C002", Name: "没有默认值的新客户", Role: &roleValue,
	})
	if err != nil {
		t.Fatalf("新建客户失败: %v", err)
	}
	if _, err := fx.svc.Create(ctx, dto.QuotationInput{
		PartnerID:  plain.ID,
		ValidUntil: "2026-11-30",
		Items:      items,
	}); err == nil {
		t.Fatal("客户没有默认贸易术语时，不填应当报错")
	}
}
