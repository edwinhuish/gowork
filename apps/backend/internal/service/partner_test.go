package service

import (
	"context"
	"reflect"
	"testing"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/repository"
)

// TestPartnerProfileFields 是 T-49a 的验收点：税号 / 客户分组 / 区域 / 分级
// 能写能读，分级白名单拦住非法值，筛选能区分「未分级」与「不筛」。
func TestPartnerProfileFields(t *testing.T) {
	ctx := context.Background()
	svc := newPartnerTestService(t)

	roleOf := func(r model.PartnerRole) *int8 {
		v := int8(r)
		return &v
	}
	str := func(s string) *string { return &s }
	create := func(in dto.PartnerInput) *model.Partner {
		t.Helper()
		m, err := svc.Create(ctx, in)
		if err != nil {
			t.Fatalf("新建主体 %s 失败: %v", in.Code, err)
		}
		return m
	}

	a := create(dto.PartnerInput{
		Code: "CUS001", Name: "核心客户", Role: roleOf(model.PartnerRoleCustomer),
		TaxID: str(" RUC-001 "), CustomerGroup: str("南美批发商"), Region: str("南美一区"),
		Grade: str("a"),
	})
	if a.Grade != model.PartnerGradeA {
		t.Errorf("分级应统一转大写，得到 %q", a.Grade)
	}
	if a.TaxID != "RUC-001" {
		t.Errorf("税号应去掉首尾空格，得到 %q", a.TaxID)
	}

	// 白名单：D 不是合法分级，必须当场拒绝，而不是存进去等报表出错。
	if _, err := svc.Create(ctx, dto.PartnerInput{
		Code: "CUS002", Name: "非法分级", Role: roleOf(model.PartnerRoleCustomer), Grade: str("D"),
	}); err == nil {
		t.Fatal("非法分级应被拒绝")
	}

	create(dto.PartnerInput{Code: "CUS003", Name: "未分级", Role: roleOf(model.PartnerRoleCustomer)})

	collect := func(q dto.PartnerQuery) []string {
		t.Helper()
		res, err := svc.List(ctx, q)
		if err != nil {
			t.Fatalf("查询失败: %v", err)
		}
		codes := make([]string, 0, len(res.List))
		for _, m := range res.List {
			codes = append(codes, m.Code)
		}
		return codes
	}

	if got := collect(dto.PartnerQuery{Grade: "a"}); !reflect.DeepEqual(got, []string{"CUS001"}) {
		t.Errorf("按 A 筛选应为 [CUS001]，得到 %v", got)
	}
	// 空串 = 不筛，none = 未分级。两者混在一起，「哪些客户还没分级」就永远答不出来。
	if got := collect(dto.PartnerQuery{Grade: "none"}); !reflect.DeepEqual(got, []string{"CUS003"}) {
		t.Errorf("按未分级筛选应为 [CUS003]，得到 %v", got)
	}
	if got := collect(dto.PartnerQuery{}); len(got) != 2 {
		t.Errorf("不传分级应返回全部 2 条，得到 %v", got)
	}
	if got := collect(dto.PartnerQuery{CustomerGroup: "南美批发商", Region: "南美一区"}); !reflect.DeepEqual(got, []string{"CUS001"}) {
		t.Errorf("按分组与区域筛选应为 [CUS001]，得到 %v", got)
	}

	// 编辑只提交分级时，其它档案要素不能被清空（nil = 这次不改）。
	// 供应商页面没有客户分组与分级，这条断言就是那道防线。
	if _, err := svc.Update(ctx, a.ID, dto.PartnerInput{
		Code: a.Code, Name: a.Name, Role: roleOf(model.PartnerRoleCustomer), Grade: str("B"),
	}); err != nil {
		t.Fatalf("编辑失败: %v", err)
	}
	got, err := svc.Get(ctx, a.ID)
	if err != nil {
		t.Fatalf("回查失败: %v", err)
	}
	if got.Grade != model.PartnerGradeB {
		t.Errorf("分级应为 B，得到 %q", got.Grade)
	}
	if got.TaxID != "RUC-001" || got.CustomerGroup != "南美批发商" || got.Region != "南美一区" {
		t.Errorf("未提交的档案要素被清空了：税号=%q 分组=%q 区域=%q",
			got.TaxID, got.CustomerGroup, got.Region)
	}
}

func newPartnerTestService(t *testing.T) PartnerService {
	t.Helper()
	db := openTestDB(t, &model.Partner{}, &model.Product{}, &model.Currency{}, &model.User{})
	return NewPartnerService(
		repository.NewPartnerRepository(db),
		repository.NewProductRepository(db),
		repository.NewCurrencyRepository(db),
		repository.NewUserRepository(db),
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
