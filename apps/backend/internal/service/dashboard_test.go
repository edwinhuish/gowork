package service

import (
	"context"
	"log/slog"
	"testing"

	"gorm.io/gorm"

	"github.com/edwinhuish/gowork/internal/config"
	"github.com/edwinhuish/gowork/internal/infra/cache"
	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/repository"
)

type dashboardFixture struct {
	svc DashboardService
	db  *gorm.DB
	c   cache.Cache
}

func newDashboardFixture(t *testing.T, withCache bool) dashboardFixture {
	t.Helper()
	db := openTestDB(t, &model.Product{}, &model.Partner{})
	var c cache.Cache
	if withCache {
		built, err := cache.New(config.Cache{Driver: config.CacheMemory},
			slog.New(slog.DiscardHandler))
		if err != nil {
			t.Fatalf("建缓存失败: %v", err)
		}
		c = built
	}
	return dashboardFixture{
		db: db,
		c:  c,
		svc: NewDashboardService(
			repository.NewProductRepository(db), repository.NewPartnerRepository(db), c),
	}
}

func (f dashboardFixture) addProduct(t *testing.T, sku string, status model.EnableStatus) {
	t.Helper()
	p := &model.Product{SKUCode: sku, NameCN: sku, Unit: "PCS"}
	if err := f.db.Create(p).Error; err != nil {
		t.Fatalf("建产品失败: %v", err)
	}
	// Status 带 `default:1`：零值会被 gorm 省略、由数据库填 1，
	// 于是"显式停用"被静默存成启用。建完单独写一次才可靠。
	if err := f.db.Model(&model.Product{}).Where("id = ?", p.ID).
		Update("status", status).Error; err != nil {
		t.Fatalf("改产品状态失败: %v", err)
	}
}

func (f dashboardFixture) addPartner(t *testing.T, code string, role model.PartnerRole) {
	t.Helper()
	p := &model.Partner{
		Code: code, Name: code, Role: role,
		EntityType: model.PartnerEntityCompany,
	}
	if err := f.db.Create(p).Error; err != nil {
		t.Fatalf("建主体失败: %v", err)
	}
}

// 「客供两用」的主体在客户与供应商两个指标里**各计一次**：
// 它确实同时是两种身份，只是共用一条记录、一套联系方式。
func TestDashboardSummaryCounts(t *testing.T) {
	f := newDashboardFixture(t, false)
	ctx := context.Background()

	f.addProduct(t, "P1", model.EnableStatusOn)
	f.addProduct(t, "P2", model.EnableStatusOn)
	f.addProduct(t, "P3", model.EnableStatusOff)
	f.addPartner(t, "C1", model.PartnerRoleCustomer)
	f.addPartner(t, "S1", model.PartnerRoleSupplier)
	f.addPartner(t, "B1", model.PartnerRoleCustomer|model.PartnerRoleSupplier)

	got, err := f.svc.Summary(ctx)
	if err != nil {
		t.Fatalf("取汇总失败: %v", err)
	}
	if got.ProductTotal != 3 || got.EnabledProducts != 2 {
		t.Errorf("产品 3 / 启用 2，得到 %d / %d", got.ProductTotal, got.EnabledProducts)
	}
	if got.CustomerTotal != 2 {
		t.Errorf("客户应有 2 家（含客供两用），得到 %d", got.CustomerTotal)
	}
	if got.SupplierTotal != 2 {
		t.Errorf("供应商应有 2 家（含客供两用），得到 %d", got.SupplierTotal)
	}
}

// 缓存生效的证据是"数据变了、这一屏还没变"——30 秒的旧值是设计，不是 bug。
func TestDashboardSummaryCaches(t *testing.T) {
	f := newDashboardFixture(t, true)
	ctx := context.Background()
	f.addProduct(t, "P1", model.EnableStatusOn)

	first, err := f.svc.Summary(ctx)
	if err != nil {
		t.Fatalf("取汇总失败: %v", err)
	}
	if first.ProductTotal != 1 {
		t.Fatalf("首次应为 1，得到 %d", first.ProductTotal)
	}

	f.addProduct(t, "P2", model.EnableStatusOn)
	second, err := f.svc.Summary(ctx)
	if err != nil {
		t.Fatalf("取汇总失败: %v", err)
	}
	if second.ProductTotal != 1 {
		t.Errorf("TTL 内应命中缓存（仍是 1），得到 %d", second.ProductTotal)
	}

	// 清掉缓存后应重新查库，看到新数据。
	if err := f.c.Del(ctx, "dashboard:summary"); err != nil {
		t.Fatalf("清缓存失败: %v", err)
	}
	third, err := f.svc.Summary(ctx)
	if err != nil {
		t.Fatalf("取汇总失败: %v", err)
	}
	if third.ProductTotal != 2 {
		t.Errorf("缓存清掉后应看到 2，得到 %d", third.ProductTotal)
	}
}

// 缓存里的内容坏了（老版本格式、被人手改过）不能让整页 500：
// 汇总数据随时能重算，宁可重算一次。
func TestDashboardSummaryFallsBackOnBrokenCache(t *testing.T) {
	f := newDashboardFixture(t, true)
	ctx := context.Background()
	f.addProduct(t, "P1", model.EnableStatusOn)

	if err := f.c.Set(ctx, "dashboard:summary", "{not json", 0); err != nil {
		t.Fatalf("写坏缓存失败: %v", err)
	}
	got, err := f.svc.Summary(ctx)
	if err != nil {
		t.Fatalf("缓存坏掉时应当回落查库，得到错误: %v", err)
	}
	if got.ProductTotal != 1 {
		t.Errorf("回落后应拿到真实数据（1），得到 %d", got.ProductTotal)
	}
}

// 没有缓存也不能崩：单测与一次性脚本里 cache 就是 nil。
func TestDashboardSummaryWithoutCache(t *testing.T) {
	f := newDashboardFixture(t, false)
	ctx := context.Background()
	f.addProduct(t, "P1", model.EnableStatusOn)
	f.addProduct(t, "P2", model.EnableStatusOn)

	if _, err := f.svc.Summary(ctx); err != nil {
		t.Fatalf("无缓存时取汇总失败: %v", err)
	}
	got, err := f.svc.Summary(ctx)
	if err != nil {
		t.Fatalf("无缓存时取汇总失败: %v", err)
	}
	// 没有缓存就该每次都是新的。
	if got.ProductTotal != 2 {
		t.Errorf("应为 2，得到 %d", got.ProductTotal)
	}
}
