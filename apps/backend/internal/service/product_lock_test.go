package service

import (
	"context"
	"testing"
	"time"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/pkg/errs"
)

// 产品走的是 Base.UpdateFields 这条通用路径（报价单走各自的事务），
// 两条路径必须共用同一套判定——否则"哪条记录受保护"取决于它当初怎么写更新。
func TestProductOptimisticLock(t *testing.T) {
	ctx := context.Background()
	f := newProductCodeFixture(t)

	current, err := f.products.Get(ctx, f.productB)
	if err != nil {
		t.Fatalf("读产品: %v", err)
	}
	stale := current.UpdatedAt

	in := dto.ProductInput{
		SKUCode:   current.SKUCode,
		NameCN:    "改过的品名",
		Unit:      current.Unit,
		UpdatedAt: &stale,
	}
	if _, err := f.products.Update(ctx, current.ID, in); err != nil {
		t.Fatalf("带当前版本更新应成功: %v", err)
	}

	// gorm 的 updated_at 取系统时间，两次落在同一毫秒时条件仍会命中，
	// 那样这条用例什么也没测到。隔开一点，测语义不测时钟精度。
	time.Sleep(2 * time.Millisecond)

	if _, err := f.products.Update(ctx, current.ID, in); err == nil {
		t.Fatal("拿过期版本更新应当被拒绝")
	} else if e, ok := errs.From(err); !ok || e.Code != errs.CodeConflict {
		t.Fatalf("期望 40901，实际 %v", err)
	}

	// 不带版本照旧能改：脚本与迁移不该被这条规则挡住。
	if _, err := f.products.Update(ctx, current.ID, dto.ProductInput{
		SKUCode: current.SKUCode, NameCN: "不带版本", Unit: current.Unit,
	}); err != nil {
		t.Fatalf("不带版本的更新应当成功: %v", err)
	}
}
