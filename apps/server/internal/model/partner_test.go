package model

import (
	"reflect"
	"testing"
)

func TestRoleValuesContaining(t *testing.T) {
	cases := []struct {
		name   string
		target PartnerRole
		want   []PartnerRole
	}{
		// 三种角色共 7 种非空组合，每种角色各出现在其中 4 种里。
		{"客户", PartnerRoleCustomer, []PartnerRole{1, 3, 5, 7}},
		{"供应商", PartnerRoleSupplier, []PartnerRole{2, 3, 6, 7}},
		{"货代", PartnerRoleForwarder, []PartnerRole{4, 5, 6, 7}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := RoleValuesContaining(c.target)
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("包含 %s 的取值应为 %v，得到 %v", c.target, c.want, got)
			}
		})
	}
}

func TestPartnerRoleString(t *testing.T) {
	cases := []struct {
		role PartnerRole
		want string
	}{
		{PartnerRoleCustomer, "customer"},
		{PartnerRoleSupplier, "supplier"},
		{PartnerRoleForwarder, "forwarder"},
		{PartnerRoleCustomer | PartnerRoleSupplier, "customer+supplier"},
		{PartnerRoleCustomer | PartnerRoleForwarder, "customer+forwarder"},
		{AllPartnerRoles, "customer+supplier+forwarder"},
		// 零值合法不了，含未定义位说明有人写脏了一位，两者都退化成数字。
		{0, "role(0)"},
		{PartnerRole(8), "role(8)"},
	}

	for _, c := range cases {
		if got := c.role.String(); got != c.want {
			t.Errorf("role %d 的展示名应为 %q，得到 %q", int8(c.role), c.want, got)
		}
	}
}

func TestPartnerRoleIsValid(t *testing.T) {
	valid := []PartnerRole{
		PartnerRoleCustomer,
		PartnerRoleSupplier,
		PartnerRoleForwarder,
		PartnerRoleCustomer | PartnerRoleForwarder,
		AllPartnerRoles,
	}
	for _, r := range valid {
		if !r.IsValid() {
			t.Errorf("role %d 应合法", int8(r))
		}
	}

	invalid := []PartnerRole{0, PartnerRole(8), PartnerRole(PartnerRoleForwarder | 8)}
	for _, r := range invalid {
		if r.IsValid() {
			t.Errorf("role %d 应被判为非法", int8(r))
		}
	}
}
