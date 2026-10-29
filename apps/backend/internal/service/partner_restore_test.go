package service

import (
	"context"
	"testing"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/errs"
)

func partnerRoleInput(r model.PartnerRole) *int8 {
	v := int8(r)
	return &v
}

func partnerSearch(t *testing.T, svc PartnerService, keyword string, includeDeleted bool) int64 {
	t.Helper()
	res, err := svc.List(context.Background(), dto.PartnerQuery{
		PageQuery:      dto.PageQuery{Page: 1, PageSize: 10, Keyword: keyword},
		IncludeDeleted: includeDeleted,
	})
	if err != nil {
		t.Fatalf("查往来单位失败: %v", err)
	}
	return res.Total
}

func TestPartnerRestore(t *testing.T) {
	ctx := context.Background()
	svc := newPartnerTestService(t)

	m, err := svc.Create(ctx, dto.PartnerInput{
		Code: "CUS900", Name: "恢复测试客户", Role: partnerRoleInput(model.PartnerRoleCustomer),
	})
	if err != nil {
		t.Fatalf("新建主体: %v", err)
	}

	if err := svc.Delete(ctx, m.ID); err != nil {
		t.Fatalf("删除主体: %v", err)
	}
	// 默认列表看不到，打开「含已删除」才看得见——这是恢复入口的唯一发现路径。
	if n := partnerSearch(t, svc, "CUS900", false); n != 0 {
		t.Fatalf("删除后默认列表仍有 %d 条", n)
	}
	if n := partnerSearch(t, svc, "CUS900", true); n != 1 {
		t.Fatalf("含已删除应有 1 条，得到 %d", n)
	}

	r, err := svc.Restore(ctx, m.ID)
	if err != nil {
		t.Fatalf("恢复失败: %v", err)
	}
	if r.ID != m.ID || r.DeletedAt != 0 {
		t.Fatalf("恢复结果异常: id=%d deleted_at=%d", r.ID, r.DeletedAt)
	}
	if n := partnerSearch(t, svc, "CUS900", false); n != 1 {
		t.Errorf("恢复后默认列表应有 1 条，得到 %d", n)
	}
}

// 对没删过的记录调用恢复是误操作，要明确拒绝。
func TestPartnerRestoreRejectsAliveRecord(t *testing.T) {
	ctx := context.Background()
	svc := newPartnerTestService(t)

	m, err := svc.Create(ctx, dto.PartnerInput{
		Code: "CUS901", Name: "没删过的", Role: partnerRoleInput(model.PartnerRoleCustomer),
	})
	if err != nil {
		t.Fatalf("新建主体: %v", err)
	}

	if _, err := svc.Restore(ctx, m.ID); err == nil {
		t.Fatal("未删除的主体不应被恢复")
	} else if e, ok := errs.From(err); !ok || e.Code != errs.CodeInvalidParam {
		t.Fatalf("期望 40001，实际 %v", err)
	}
}

// 编码已被别的记录占用时，恢复要给出能读懂的提示；释放后可以恢复。
func TestPartnerRestoreRejectsOccupiedCode(t *testing.T) {
	ctx := context.Background()
	svc := newPartnerTestService(t)

	old, err := svc.Create(ctx, dto.PartnerInput{
		Code: "CUS902", Name: "旧记录", Role: partnerRoleInput(model.PartnerRoleCustomer),
	})
	if err != nil {
		t.Fatalf("新建主体: %v", err)
	}
	if err := svc.Delete(ctx, old.ID); err != nil {
		t.Fatalf("删除主体: %v", err)
	}
	reborn, err := svc.Create(ctx, dto.PartnerInput{
		Code: "CUS902", Name: "同号重建", Role: partnerRoleInput(model.PartnerRoleCustomer),
	})
	if err != nil {
		t.Fatalf("同编码重建失败: %v", err)
	}

	if _, err := svc.Restore(ctx, old.ID); err == nil {
		t.Fatal("编码被占用时应拒绝恢复")
	} else if e, ok := errs.From(err); !ok || e.Code != errs.CodeConflict {
		t.Fatalf("期望 40901，实际 %v", err)
	}

	if err := svc.Delete(ctx, reborn.ID); err != nil {
		t.Fatalf("删除重建的那条: %v", err)
	}
	if _, err := svc.Restore(ctx, old.ID); err != nil {
		t.Errorf("释放编码后应能恢复: %v", err)
	}
}
