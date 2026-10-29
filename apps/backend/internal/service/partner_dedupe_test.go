package service

import (
	"context"
	"testing"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/model"
)

// 查重跑在真实库上：分组要跨"这一页之外"的记录比对，而软删行是否参与
// 这件事只有真库能回答（SQLite 的 softDelete 过滤是 gorm scope，不是内存里的判断）。
func TestPartnerDuplicates(t *testing.T) {
	ctx := context.Background()
	svc := newPartnerTestService(t)

	role := int8(model.PartnerRoleCustomer)
	create := func(code, name, country, phone, email string) *model.Partner {
		t.Helper()
		m, err := svc.Create(ctx, dto.PartnerInput{
			Code: code, Name: name, Role: &role,
			Country: country, Phone: phone, Email: email,
		})
		if err != nil {
			t.Fatalf("新建主体 %s 失败: %v", code, err)
		}
		return m
	}

	sol := create("CUS001", "Ferreteria El Sol", "Ecuador", "+593 9 0000 0001", "carlos@elsol.com.ec")
	dup := create("CUS002", "FERRETERIA  EL-SOL", "Ecuador", "593900000001", "ventas@elsol.com.ec")
	// 同名不同国：外贸里是两笔生意，不该并。
	create("CUS003", "Ferreteria El Sol", "Peru", "", "")
	// 两条都用 gmail：不算重复。
	create("CUS004", "Otro Cliente", "Ecuador", "", "otro@gmail.com")
	create("CUS005", "Nadie Mas", "Ecuador", "", "nadie@gmail.com")

	t.Run("三条规则各出一组，同一对主体", func(t *testing.T) {
		groups, err := svc.Duplicates(ctx)
		if err != nil {
			t.Fatalf("查重失败: %v", err)
		}
		if len(groups) != 3 {
			t.Fatalf("应有 3 组（名称 / 邮箱 / 电话），实际 %d 组: %+v", len(groups), groups)
		}
		for _, g := range groups {
			if len(g.Partners) != 2 ||
				g.Partners[0].ID != sol.ID || g.Partners[1].ID != dup.ID {
				t.Errorf("规则 %s 的组应为 CUS001+CUS002，实际 %+v", g.Rule, g.Partners)
			}
			if g.Reason == "" {
				t.Errorf("规则 %s 的分组缺少命中原因", g.Rule)
			}
		}
	})

	t.Run("候选排除自身", func(t *testing.T) {
		hits, err := svc.DuplicateCandidates(ctx, dto.PartnerDuplicateQuery{
			Name: "Ferreteria El Sol", Country: "Ecuador", ExcludeID: sol.ID,
		})
		if err != nil {
			t.Fatalf("查候选失败: %v", err)
		}
		if len(hits) != 1 || hits[0].Partner.ID != dup.ID {
			t.Fatalf("应只剩 CUS002，实际 %+v", hits)
		}
	})

	t.Run("免费邮箱不算候选", func(t *testing.T) {
		hits, err := svc.DuplicateCandidates(ctx, dto.PartnerDuplicateQuery{
			Email: "carlos.perez@gmail.com",
		})
		if err != nil {
			t.Fatalf("查候选失败: %v", err)
		}
		if len(hits) != 0 {
			t.Errorf("gmail 上的两个客户不是同一家，不该命中任何主体，实际 %+v", hits)
		}
	})

	t.Run("四要素全空时返回空", func(t *testing.T) {
		hits, err := svc.DuplicateCandidates(ctx, dto.PartnerDuplicateQuery{})
		if err != nil {
			t.Fatalf("查候选失败: %v", err)
		}
		if len(hits) != 0 {
			t.Errorf("没有可比的输入时不该命中，实际 %+v", hits)
		}
	})

	t.Run("软删的行不参与查重", func(t *testing.T) {
		if err := svc.Delete(ctx, dup.ID); err != nil {
			t.Fatalf("删除 CUS002 失败: %v", err)
		}
		groups, err := svc.Duplicates(ctx)
		if err != nil {
			t.Fatalf("查重失败: %v", err)
		}
		if len(groups) != 0 {
			t.Errorf("重复的另一方删掉后不该再成组，实际 %+v", groups)
		}
	})
}
