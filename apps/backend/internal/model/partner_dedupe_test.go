package model

import "testing"

// 归一化是查重的地基：同一个客户在名片、邮件签名、报关资料上的写法各不相同，
// 归一不干净就会"明明同一家却查不出来"，归一过头就是把两家并成一家。
func TestPartnerDedupeKeys(t *testing.T) {
	nameCases := []struct {
		name, country, want string
	}{
		{"Ferreteria El Sol", "Ecuador", "ferreteriaelsol|ecuador"},
		{"FERRETERIA  EL-SOL", " ecuador ", "ferreteriaelsol|ecuador"},
		{"Ferreteria El Sol.", "Ecuador", "ferreteriaelsol|ecuador"},
		// 中文不折叠、不剥公司后缀：两个后缀不同的主体可能是两家。
		{"永康市华信五金厂", "China", "永康市华信五金厂|china"},
		{"Ningbo Huaxiang Co., Ltd", "", "ningbohuaxiangcoltd|"},
		// 单字符名称与空名称不参与：拿来分组只会把无关主体串在一起。
		{"A", "China", ""},
		{"   ", "China", ""},
	}
	for _, c := range nameCases {
		if got := PartnerNameKey(c.name, c.country); got != c.want {
			t.Errorf("PartnerNameKey(%q, %q) = %q，期望 %q", c.name, c.country, got, c.want)
		}
	}

	phoneCases := []struct{ phone, want string }{
		{"+593 9 0000 0001", "900000001"},
		{"593900000001", "900000001"},
		{"0900000001", "900000001"},
		// 11 位的手机号只留尾 9 位，与带 +86 的写法对得上。
		{"13800000001", "800000001"},
		{"", ""},
		// 少于 7 位不参与：分机号与区号片段比出来的"重复"没有意义。
		{"123456", ""},
	}
	for _, c := range phoneCases {
		if got := PartnerPhoneKey(c.phone); got != c.want {
			t.Errorf("PartnerPhoneKey(%q) = %q，期望 %q", c.phone, got, c.want)
		}
	}

	emailCases := []struct{ email, want string }{
		{"carlos@ElSol.com.ec", "elsol.com.ec"},
		{"Carlos@www.elsol.com.ec", "elsol.com.ec"},
		// 免费邮箱人人都能注册，两个客户都用 gmail 不是同一家。
		{"someone@gmail.com", ""},
		{"someone@qq.com", ""},
		{"没有邮箱", ""},
		{"broken@", ""},
	}
	for _, c := range emailCases {
		if got := PartnerEmailDomainKey(c.email); got != c.want {
			t.Errorf("PartnerEmailDomainKey(%q) = %q，期望 %q", c.email, got, c.want)
		}
	}
}

// 分组是"清点全库还剩多少重复"，三条规则各分一遍，同一条只能落进一类里。
func TestGroupPartnerDuplicates(t *testing.T) {
	all := []Partner{
		{Base: Base{ID: 1}, Name: "Ferreteria El Sol", Country: "Ecuador", Phone: "+593 9 0000 0001", Email: "carlos@elsol.com.ec"},
		{Base: Base{ID: 2}, Name: "FERRETERIA EL-SOL", Country: "Ecuador", Phone: "593900000001", Email: "ventas@elsol.com.ec"},
		// 同名不同国：外贸里是两笔生意，不能并。
		{Base: Base{ID: 3}, Name: "Ferreteria El Sol", Country: "Peru"},
		// 两条都用 gmail：不算重复。
		{Base: Base{ID: 4}, Name: "Otro Cliente", Country: "Ecuador", Email: "otro@gmail.com"},
		{Base: Base{ID: 5}, Name: "Nadie Mas", Country: "Ecuador", Email: "nadie@gmail.com"},
	}

	groups := GroupPartnerDuplicates(all)
	if len(groups) != 3 {
		t.Fatalf("应有 3 组（名称 / 邮箱 / 电话），实际 %d 组: %+v", len(groups), groups)
	}

	wantRules := []string{PartnerDupNameCountry, PartnerDupEmailDomain, PartnerDupPhone}
	for i, want := range wantRules {
		got := groups[i]
		if got.Rule != want {
			t.Errorf("第 %d 组的规则 = %q，期望 %q", i, got.Rule, want)
		}
		if len(got.Partners) != 2 || got.Partners[0].ID != 1 || got.Partners[1].ID != 2 {
			t.Errorf("第 %d 组应为 1+2，实际 %+v", i, got.Partners)
		}
		if got.Reason == "" {
			t.Errorf("第 %d 组没有命中原因", i)
		}
	}
}

// 候选用于录入时提示：一条主体只出现一次，命中几条规则就把原因并在一起。
func TestFindPartnerDuplicates(t *testing.T) {
	all := []Partner{
		{Base: Base{ID: 1}, Name: "Ferreteria El Sol", Country: "Ecuador", Phone: "+593 9 0000 0001", Email: "carlos@elsol.com.ec"},
		{Base: Base{ID: 2}, Name: "FERRETERIA EL-SOL", Country: "Ecuador", Phone: "593900000001", Email: "ventas@elsol.com.ec"},
	}

	t.Run("三条规则全中时只出一条候选", func(t *testing.T) {
		hits := FindPartnerDuplicates(all, PartnerProbe{
			Name: "Ferreteria ElSol", Country: "ecuador",
			Phone: "0900000001", Email: "info@elsol.com.ec",
		})
		if len(hits) != 2 {
			t.Fatalf("应命中两条，实际 %d", len(hits))
		}
		if len(hits[0].Reasons) != 3 {
			t.Errorf("两条主体都该命中三条规则，实际原因 %v", hits[0].Reasons)
		}
	})

	t.Run("编辑时排除自身", func(t *testing.T) {
		hits := FindPartnerDuplicates(all, PartnerProbe{
			Name: "Ferreteria El Sol", Country: "Ecuador", ExcludeID: 1,
		})
		if len(hits) != 1 || hits[0].Partner.ID != 2 {
			t.Fatalf("应只剩 id=2，实际 %+v", hits)
		}
	})

	t.Run("四要素都不可比时直接返回空", func(t *testing.T) {
		hits := FindPartnerDuplicates(all, PartnerProbe{Name: "A", Email: "x@gmail.com", Phone: "123"})
		if len(hits) != 0 {
			t.Errorf("不可比的输入不该命中任何主体，实际 %+v", hits)
		}
	})
}
