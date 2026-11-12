package service

import (
	"context"
	"testing"

	"gowork_server/internal/dto"
	"gowork_server/internal/model"
	"gowork_server/internal/repository"
)

// TestPartnerContacts 是「主体带多个联系人」的验收点。
//
// 四条各防一件事：
//   - 列表把主体下的**全部**联系人带出来，而不是只带主联系人那一个；
//   - 主联系人排第一（前端头像组只显示前三个，主联系人不能被录入顺序挤掉）；
//   - 没有联系人的主体给空数组而不是 null，前端形状固定；
//   - 详情与列表口径一致，否则会出现"列表有头像、详情没人"。
func TestPartnerContacts(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t,
		&model.Partner{}, &model.Product{}, &model.Currency{}, &model.User{}, &model.Contact{})
	contacts := repository.NewContactRepository(db)
	svc := NewPartnerService(
		repository.NewPartnerRepository(db),
		repository.NewProductRepository(db),
		repository.NewCurrencyRepository(db),
		repository.NewUserRepository(db),
		WithPartnerContacts(contacts),
	)

	roleOf := func(r model.PartnerRole) *int8 {
		v := int8(r)
		return &v
	}
	create := func(code, name string) *model.Partner {
		t.Helper()
		m, err := svc.Create(ctx, dto.PartnerInput{
			Code: code, Name: name, Role: roleOf(model.PartnerRoleCustomer),
		})
		if err != nil {
			t.Fatalf("新建主体 %s 失败: %v", code, err)
		}
		return m
	}
	addContact := func(partnerID uint, name string, primary bool) {
		t.Helper()
		if err := contacts.Create(ctx, &model.Contact{
			PartnerID: partnerID, Name: name, IsPrimary: primary,
		}); err != nil {
			t.Fatalf("新建联系人 %s 失败: %v", name, err)
		}
	}

	multi := create("CUS101", "三个人对接的客户")
	create("CUS102", "还没录联系人的客户")

	// 先建非主联系人、再建主联系人：主联系人 id 更大，只能靠 is_primary 排到前面。
	addContact(multi.ID, "Maria", false)
	addContact(multi.ID, "Carlos", true)
	addContact(multi.ID, "Ana", false)

	res, err := svc.List(ctx, dto.PartnerQuery{})
	if err != nil {
		t.Fatalf("查询列表失败: %v", err)
	}
	byCode := make(map[string]*model.Partner, len(res.List))
	for i := range res.List {
		byCode[res.List[i].Code] = &res.List[i]
	}

	got := byCode["CUS101"]
	if got == nil {
		t.Fatalf("列表里没有 CUS101")
	}
	if len(got.Contacts) != 3 {
		t.Fatalf("应带出 3 个联系人，实际 %d 个", len(got.Contacts))
	}
	if got.Contacts[0].Name != "Carlos" || !got.Contacts[0].IsPrimary {
		t.Errorf("主联系人应排第一，实际首位是 %q（is_primary=%v）",
			got.Contacts[0].Name, got.Contacts[0].IsPrimary)
	}
	if got.Contacts[1].Name != "Maria" || got.Contacts[2].Name != "Ana" {
		t.Errorf("其余联系人应按录入顺序（Maria、Ana），实际 %q、%q",
			got.Contacts[1].Name, got.Contacts[2].Name)
	}

	if rows := byCode["CUS102"].Contacts; rows == nil || len(rows) != 0 {
		t.Errorf("没有联系人的主体应返回空数组而不是 null，实际 %#v", rows)
	}

	// 详情与列表同一口径。
	detail, err := svc.Get(ctx, multi.ID)
	if err != nil {
		t.Fatalf("查询详情失败: %v", err)
	}
	if len(detail.Contacts) != len(got.Contacts) || detail.Contacts[0].Name != got.Contacts[0].Name {
		t.Errorf("详情的联系人应与列表一致，列表 %d 个、详情 %d 个",
			len(got.Contacts), len(detail.Contacts))
	}

	// 没装配联系人仓储时不能崩：内部调用拿不到这个 option，回填必须静默跳过。
	bareDB := openTestDB(t, &model.Partner{}, &model.Product{}, &model.Currency{}, &model.User{})
	bare := NewPartnerService(
		repository.NewPartnerRepository(bareDB),
		repository.NewProductRepository(bareDB),
		repository.NewCurrencyRepository(bareDB),
		repository.NewUserRepository(bareDB),
	)
	if _, err := bare.List(ctx, dto.PartnerQuery{}); err != nil {
		t.Fatalf("未装配联系人仓储时列表不应报错: %v", err)
	}
}
