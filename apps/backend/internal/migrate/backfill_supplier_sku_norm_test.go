package migrate

import (
	"log/slog"
	"testing"
	"time"

	"github.com/edwinhuish/gowork/internal/infra/database"
	"github.com/edwinhuish/gowork/internal/model"
)

func TestBackfillSupplierSKUNorm(t *testing.T) {
	db := newTestDB(t)
	log := slog.New(slog.DiscardHandler)

	if err := database.AutoMigrate(db, &model.ProductSource{}); err != nil {
		t.Fatalf("建货源表: %v", err)
	}

	// 新列由 AutoMigrate 加出来时是空的：这三行就是要被回填的"存量"。
	rows := []*model.ProductSource{
		{ProductID: 1, PartnerID: 1, SupplierSKU: "A-100", CurrencyCode: "CNY", Status: model.EnableStatusOn},
		{ProductID: 1, PartnerID: 2, SupplierSKU: "", CurrencyCode: "CNY", Status: model.EnableStatusOn},
		{
			ProductID: 1, PartnerID: 3, SupplierSKU: "B-200", CurrencyCode: "CNY",
			Status: model.EnableStatusOn,
			Base:   model.Base{DeletedAt: model.DeletedAt(time.Now().UnixMilli())},
		},
	}
	for _, r := range rows {
		if err := db.Create(r).Error; err != nil {
			t.Fatalf("写入货源: %v", err)
		}
	}

	if err := BackfillSupplierSKUNorm(db, log); err != nil {
		t.Fatalf("回填失败: %v", err)
	}

	load := func(id uint) *model.ProductSource {
		t.Helper()
		var m model.ProductSource
		if err := db.Unscoped().First(&m, id).Error; err != nil {
			t.Fatalf("读取货源 %d: %v", id, err)
		}
		return &m
	}

	if got := load(rows[0].ID).SupplierSKUNorm; got != "A100" {
		t.Errorf("回填后的归一化值 = %q，期望 A100", got)
	}
	// 空货号不回填：归一化后仍是空串，"处理过"与"没处理"没有区别。
	if got := load(rows[1].ID).SupplierSKUNorm; got != "" {
		t.Errorf("空货号的归一化值 = %q，期望留空", got)
	}
	// 软删的货源不参与检索，不必回填。
	if got := load(rows[2].ID).SupplierSKUNorm; got != "" {
		t.Errorf("软删货源的归一化值 = %q，期望不处理", got)
	}

	// 幂等：记账之后不再重跑，手工改过的值不会被覆盖。
	if err := db.Model(&model.ProductSource{}).Where("id = ?", rows[0].ID).
		Update("supplier_sku_norm", "MANUAL").Error; err != nil {
		t.Fatalf("改值: %v", err)
	}
	if err := BackfillSupplierSKUNorm(db, log); err != nil {
		t.Fatalf("二次回填失败: %v", err)
	}
	if got := load(rows[0].ID).SupplierSKUNorm; got != "MANUAL" {
		t.Errorf("二次回填覆盖了已有值：%q", got)
	}
}
