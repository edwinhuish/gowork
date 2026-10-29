package service

import (
	"context"
	"errors"
	"testing"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/errs"
	"github.com/edwinhuish/gowork/internal/repository"
)

func newPartnerAddressTestService(t *testing.T) (PartnerAddressService, PartnerService) {
	t.Helper()
	db := openTestDB(t,
		&model.Partner{}, &model.PartnerAddress{}, &model.Product{}, &model.Currency{}, &model.User{})
	partnerRepo := repository.NewPartnerRepository(db)
	partners := NewPartnerService(
		partnerRepo,
		repository.NewProductRepository(db),
		repository.NewCurrencyRepository(db),
		repository.NewUserRepository(db),
	)
	addresses := NewPartnerAddressService(repository.NewPartnerAddressRepository(db), partnerRepo)
	return addresses, partners
}

// TestPartnerAddressDefaults 是 T-49 第二步的验收点：
//
//	① 每个用途下第一条地址自动成为默认；
//	② 后设的默认会把前一条降级（同一事务）；
//	③ 不允许取消唯一的默认——那会让单证取址无据可依；
//	④ 删掉默认后，同用途剩下最早的一条接替；
//	⑤ 不同用途各自独立，互不影响。
func TestPartnerAddressDefaults(t *testing.T) {
	ctx := context.Background()
	svc, partners := newPartnerAddressTestService(t)

	role := int8(model.PartnerRoleCustomer)
	cus, err := partners.Create(ctx, dto.PartnerInput{Code: "CUS001", Name: "客户", Role: &role})
	if err != nil {
		t.Fatalf("新建主体失败: %v", err)
	}

	first, err := svc.Create(ctx, cus.ID, dto.PartnerAddressInput{
		Type: "invoice", Address: "Av. Amazonas 123", Country: "厄瓜多尔", City: "基多",
	})
	if err != nil {
		t.Fatalf("新建第一条地址失败: %v", err)
	}
	if !first.IsDefault {
		t.Fatal("同用途的第一条地址应自动成为默认，否则单证取不到它")
	}

	second, err := svc.Create(ctx, cus.ID, dto.PartnerAddressInput{
		Type: "invoice", Address: "Calle Sur 45", Country: "厄瓜多尔", IsDefault: true,
	})
	if err != nil {
		t.Fatalf("新建第二条地址失败: %v", err)
	}
	rows, err := svc.List(ctx, cus.ID, dto.PartnerAddressQuery{Type: "invoice"})
	if err != nil {
		t.Fatalf("查询地址失败: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("应有 2 条发票地址，得到 %d", len(rows))
	}
	if !rows[0].IsDefault || rows[0].ID != second.ID {
		t.Errorf("默认地址应排在最前且是后建的那条，得到 id=%d default=%v", rows[0].ID, rows[0].IsDefault)
	}
	if rows[1].IsDefault {
		t.Error("原默认地址应已被降级，同用途不能有两条默认")
	}

	// ③ 取消唯一默认必须被拒绝：留一个"有地址但没有默认"的用途，
	// 单证取址就只能靠猜。
	if _, err := svc.Update(ctx, cus.ID, second.ID, dto.PartnerAddressInput{
		Type: "invoice", Address: "Calle Sur 45",
	}); err == nil {
		t.Fatal("取消唯一默认地址应被拒绝")
	} else {
		var e *errs.Error
		if !errors.As(err, &e) || e.Code != errs.CodeInvalidParam {
			t.Fatalf("应为 400（参数错误），得到 %v", err)
		}
	}

	// ⑤ 另一用途的第一条地址同样自动成为默认，且不影响发票地址。
	delivery, err := svc.Create(ctx, cus.ID, dto.PartnerAddressInput{
		Type: "delivery", Address: "Zona Industrial 7",
	})
	if err != nil {
		t.Fatalf("新建收货地址失败: %v", err)
	}
	if !delivery.IsDefault {
		t.Error("新用途的第一条地址也应自动成为默认")
	}
	invoiceRows, err := svc.List(ctx, cus.ID, dto.PartnerAddressQuery{Type: "invoice"})
	if err != nil {
		t.Fatalf("查询发票地址失败: %v", err)
	}
	if len(invoiceRows) != 2 {
		t.Fatalf("发票地址不应被收货地址影响，得到 %d 条", len(invoiceRows))
	}

	// ④ 删掉默认的发票地址后，剩下的那条要接替。
	if err := svc.Delete(ctx, cus.ID, second.ID); err != nil {
		t.Fatalf("删除默认地址失败: %v", err)
	}
	invoiceRows, err = svc.List(ctx, cus.ID, dto.PartnerAddressQuery{Type: "invoice"})
	if err != nil {
		t.Fatalf("删除后查询失败: %v", err)
	}
	if len(invoiceRows) != 1 {
		t.Fatalf("删除后应剩 1 条，得到 %d", len(invoiceRows))
	}
	if !invoiceRows[0].IsDefault {
		t.Error("删掉默认后剩下的那条应接替为默认")
	}
}

// TestPartnerAddressScoping 覆盖两类「看起来能用错」的入参：
// 用途拼错，以及拿别家的地址 id 改自己主体下的地址。
func TestPartnerAddressScoping(t *testing.T) {
	ctx := context.Background()
	svc, partners := newPartnerAddressTestService(t)

	role := int8(model.PartnerRoleCustomer)
	owner, err := partners.Create(ctx, dto.PartnerInput{Code: "CUS001", Name: "客户甲", Role: &role})
	if err != nil {
		t.Fatalf("新建主体失败: %v", err)
	}
	other, err := partners.Create(ctx, dto.PartnerInput{Code: "CUS002", Name: "客户乙", Role: &role})
	if err != nil {
		t.Fatalf("新建主体失败: %v", err)
	}

	if _, err := svc.Create(ctx, owner.ID, dto.PartnerAddressInput{
		Type: "billing", Address: "某地",
	}); err == nil {
		t.Fatal("未定义的地址用途应被拒绝")
	}

	addr, err := svc.Create(ctx, owner.ID, dto.PartnerAddressInput{
		Type: "notify", Address: "甲的通知方",
	})
	if err != nil {
		t.Fatalf("新建地址失败: %v", err)
	}

	// 路径上的 partnerID 是 other，地址属于 owner：必须按不存在处理，
	// 否则任何人都能借着"别人的 id"改到别家的地址。
	if _, err := svc.Update(ctx, other.ID, addr.ID, dto.PartnerAddressInput{
		Type: "notify", Address: "被篡改",
	}); !errors.Is(err, errs.ErrNotFound) {
		t.Fatalf("跨主体改地址应返回 404，得到 %v", err)
	}
	if err := svc.Delete(ctx, other.ID, addr.ID); !errors.Is(err, errs.ErrNotFound) {
		t.Fatalf("跨主体删地址应返回 404，得到 %v", err)
	}

	// 主体不存在时也不该返回空列表：那是"这家客户没有地址"，
	// 与"这家客户不存在"要能分开。
	if _, err := svc.List(ctx, 9999, dto.PartnerAddressQuery{}); !errors.Is(err, errs.ErrNotFound) {
		t.Fatalf("主体不存在应返回 404，得到 %v", err)
	}
}
