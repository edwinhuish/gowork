package service

import (
	"context"
	"reflect"
	"testing"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/repository"
)

func newPartnerTestService(t *testing.T) PartnerService {
	t.Helper()
	db := openTestDB(t, &model.Partner{}, &model.Product{}, &model.Currency{})
	return NewPartnerService(
		repository.NewPartnerRepository(db),
		repository.NewProductRepository(db),
		repository.NewCurrencyRepository(db),
	)
}

// TestPartnerRoleScoping 是 T-17 的验收点：货代进了同一张 md_partner 之后，
// 三个角色入口必须各看各的，尤其是"客户兼货代"要同时出现在客户与货代列表里。
func TestPartnerRoleScoping(t *testing.T) {
	ctx := context.Background()
	svc := newPartnerTestService(t)

	roleOf := func(r model.PartnerRole) *int8 {
		v := int8(r)
		return &v
	}
	create := func(code, name string, r model.PartnerRole) {
		t.Helper()
		_, err := svc.Create(ctx, dto.PartnerInput{Code: code, Name: name, Role: roleOf(r)})
		if err != nil {
			t.Fatalf("新建主体 %s 失败: %v", code, err)
		}
	}

	create("CUS001", "纯客户", model.PartnerRoleCustomer)
	create("SUP001", "纯供应商", model.PartnerRoleSupplier)
	create("FWD001", "纯货代", model.PartnerRoleForwarder)
	create("FWD002", "客户兼货代", model.PartnerRoleCustomer|model.PartnerRoleForwarder)

	cases := []struct {
		name string
		role model.PartnerRole
		// 列表默认按 id DESC，后建的在前。
		want []string
	}{
		{"客户", model.PartnerRoleCustomer, []string{"FWD002", "CUS001"}},
		{"供应商", model.PartnerRoleSupplier, []string{"SUP001"}},
		{"货代", model.PartnerRoleForwarder, []string{"FWD002", "FWD001"}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			res, err := svc.List(ctx, dto.PartnerQuery{Role: roleOf(c.role)})
			if err != nil {
				t.Fatalf("查询 %s 列表失败: %v", c.name, err)
			}
			got := make([]string, 0, len(res.List))
			for _, m := range res.List {
				got = append(got, m.Code)
			}
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("%s 列表应为 %v，得到 %v", c.name, c.want, got)
			}
		})
	}
}

func TestPartnerRoleValidation(t *testing.T) {
	ctx := context.Background()
	svc := newPartnerTestService(t)

	// 新建必须有角色：没有角色的主体不会出现在任何候选列表里，等于一条查不到的脏数据。
	if _, err := svc.Create(ctx, dto.PartnerInput{Code: "X001", Name: "无角色"}); err == nil {
		t.Error("不给 role 时应拒绝创建")
	}

	// 8 是未定义的位，不是"多选了一个"——位掩码只认已登记的位。
	invalid := int8(8)
	if _, err := svc.Create(ctx, dto.PartnerInput{Code: "X002", Name: "非法角色", Role: &invalid}); err == nil {
		t.Error("role=8 时应拒绝创建")
	}
}
