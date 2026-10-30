package service

import (
	"context"
	"testing"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/errs"
	"github.com/edwinhuish/gowork/internal/repository"
)

// 仓库此前连删除入口都没有，更没有恢复：录错一个只能放着让它占着下拉。
// 这次补齐删除 / 含已删除查询 / 恢复，并用默认仓的规则把边界钉住。
func TestWarehouseLifecycle(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t, &model.Warehouse{})
	svc := NewWarehouseService(repository.NewWarehouseRepository(db))

	mainWh, err := svc.Create(ctx, dto.WarehouseInput{Code: "MAIN", Name: "主仓", IsDefault: true})
	if err != nil {
		t.Fatalf("建主仓失败: %v", err)
	}
	spare, err := svc.Create(ctx, dto.WarehouseInput{Code: "SPARE", Name: "备用仓"})
	if err != nil {
		t.Fatalf("建备用仓失败: %v", err)
	}

	t.Run("默认仓不能删", func(t *testing.T) {
		err := svc.Delete(ctx, mainWh.ID)
		if e, ok := errs.From(err); !ok || e.Code != errs.CodeConflict {
			t.Fatalf("期望 40901，实际 %v", err)
		}
	})

	t.Run("删掉之后只在含已删除里出现", func(t *testing.T) {
		if err := svc.Delete(ctx, spare.ID); err != nil {
			t.Fatalf("删除备用仓失败: %v", err)
		}
		rows, err := svc.List(ctx, false)
		if err != nil {
			t.Fatalf("列表失败: %v", err)
		}
		if len(rows) != 1 || rows[0].ID != mainWh.ID {
			t.Fatalf("默认列表只该剩主仓，得到 %+v", rows)
		}
		withDeleted, err := svc.List(ctx, true)
		if err != nil {
			t.Fatalf("含已删除列表失败: %v", err)
		}
		if len(withDeleted) != 2 {
			t.Fatalf("含已删除应有两条，得到 %d 条", len(withDeleted))
		}
	})

	t.Run("恢复未删的仓库要报错", func(t *testing.T) {
		if _, err := svc.Restore(ctx, mainWh.ID); err == nil {
			t.Error("恢复未被删除的仓库应报错")
		}
	})

	t.Run("删掉的名额定回来时不冲突", func(t *testing.T) {
		if _, err := svc.Restore(ctx, spare.ID); err != nil {
			t.Fatalf("恢复备用仓失败: %v", err)
		}
		rows, err := svc.List(ctx, false)
		if err != nil {
			t.Fatalf("列表失败: %v", err)
		}
		if len(rows) != 2 {
			t.Errorf("恢复后应有两条仓库，得到 %d 条", len(rows))
		}
	})

	t.Run("编码被占时拒绝恢复", func(t *testing.T) {
		if err := svc.Delete(ctx, spare.ID); err != nil {
			t.Fatalf("删除备用仓失败: %v", err)
		}
		if _, err := svc.Create(ctx, dto.WarehouseInput{Code: "SPARE", Name: "新备用仓"}); err != nil {
			t.Fatalf("重录同编码仓库失败: %v", err)
		}
		if _, err := svc.Restore(ctx, spare.ID); err == nil {
			t.Error("编码被占时应拒绝恢复")
		}
	})
}

// 默认仓被取消之后必须自动有人顶上：没有默认仓时新流水落 warehouse_id = 0，
// 而那是一个查不出任何仓库的 id。
func TestWarehouseDemoteKeepsExactlyOneDefault(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t, &model.Warehouse{})
	svc := NewWarehouseService(repository.NewWarehouseRepository(db))

	first, err := svc.Create(ctx, dto.WarehouseInput{Code: "A", Name: "甲仓", IsDefault: true})
	if err != nil {
		t.Fatalf("建甲仓失败: %v", err)
	}
	second, err := svc.Create(ctx, dto.WarehouseInput{Code: "B", Name: "乙仓"})
	if err != nil {
		t.Fatalf("建乙仓失败: %v", err)
	}
	if err := svc.Update(ctx, first.ID, dto.WarehouseInput{
		Code: first.Code, Name: "甲仓", IsDefault: false,
	}); err != nil {
		t.Fatalf("取消默认仓失败: %v", err)
	}

	rows, err := svc.List(ctx, false)
	if err != nil {
		t.Fatalf("列表失败: %v", err)
	}
	defaults := 0
	for _, w := range rows {
		if w.IsDefault {
			defaults++
			if w.ID != second.ID {
				t.Errorf("应由乙仓顶上默认仓，实际是 id=%d", w.ID)
			}
		}
	}
	if defaults != 1 {
		t.Errorf("只能有一个默认仓，得到 %d 个", defaults)
	}
}
