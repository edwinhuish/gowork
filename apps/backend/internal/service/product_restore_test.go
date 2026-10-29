package service

import (
	"context"
	"errors"
	"testing"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/pkg/errs"
)

// 删除是软删，恢复就该有入口：把标记写回 0，行重新出现在普通查询里。
func TestProductRestore(t *testing.T) {
	ctx := context.Background()
	f := newProductCodeFixture(t)

	if err := f.products.Delete(ctx, f.productA); err != nil {
		t.Fatalf("删除产品: %v", err)
	}
	if _, err := f.products.Get(ctx, f.productA); !errors.Is(err, errs.ErrNotFound) {
		t.Fatalf("删除后普通查询应返回 ErrNotFound，得到 %v", err)
	}

	m, err := f.products.Restore(ctx, f.productA)
	if err != nil {
		t.Fatalf("恢复失败: %v", err)
	}
	if m.ID != f.productA {
		t.Fatalf("恢复后 id = %d，期望 %d", m.ID, f.productA)
	}
	if m.DeletedAt != 0 {
		t.Errorf("恢复后删除标记 = %d，期望 0", m.DeletedAt)
	}
	if _, err := f.products.Get(ctx, f.productA); err != nil {
		t.Errorf("恢复后应能被普通查询读到: %v", err)
	}
}

// 对没删过的记录调用恢复是误操作，要明确拒绝，而不是静默当成成功。
func TestProductRestoreRejectsAliveRecord(t *testing.T) {
	ctx := context.Background()
	f := newProductCodeFixture(t)

	_, err := f.products.Restore(ctx, f.productA)
	if err == nil {
		t.Fatal("未删除的产品不应被恢复")
	}
	if e, ok := errs.From(err); !ok || e.Code != errs.CodeInvalidParam {
		t.Fatalf("期望 40001，实际 %v", err)
	}
}

// 删掉之后同 SKU 已经被别的产品占用时，恢复会撞唯一索引。
// 这里要在进入数据库之前就给出能读懂的提示：谁占了、该怎么办。
func TestProductRestoreRejectsOccupiedSKU(t *testing.T) {
	ctx := context.Background()
	f := newProductCodeFixture(t)

	if err := f.products.Delete(ctx, f.productA); err != nil {
		t.Fatalf("删除产品: %v", err)
	}
	// 删掉之后同一个 SKU 可以立刻重建——这正是「未删才唯一」的效果，
	// 但也意味着旧记录的恢复会与它冲突。
	reborn, err := f.products.Create(ctx, dto.ProductInput{
		SKUCode: "BH01-0001", NameCN: "同号新产品", Unit: "PCS",
	})
	if err != nil {
		t.Fatalf("同 SKU 重建失败: %v", err)
	}

	_, err = f.products.Restore(ctx, f.productA)
	if err == nil {
		t.Fatal("SKU 已被占用时应拒绝恢复")
	}
	if e, ok := errs.From(err); !ok || e.Code != errs.CodeConflict {
		t.Fatalf("期望 409，实际 %v", err)
	}

	// 删掉占用的那条之后就能恢复了。
	if err := f.products.Delete(ctx, reborn.ID); err != nil {
		t.Fatalf("删除重建的那条: %v", err)
	}
	if _, err := f.products.Restore(ctx, f.productA); err != nil {
		t.Errorf("释放 SKU 后应能恢复: %v", err)
	}
}
