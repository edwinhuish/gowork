package service

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/errs"
	"github.com/edwinhuish/gowork/internal/pkg/money"
	"github.com/edwinhuish/gowork/internal/repository"
)

type sourceFixture struct {
	sources  ProductSourceService
	products ProductService
	// IDs 由 fixture 建好，测试里直接用。
	productID  uint
	product2ID uint
	supplierID uint
	customerID uint
}

func newSourceFixture(t *testing.T, now time.Time) sourceFixture {
	t.Helper()
	ctx := context.Background()

	db := openTestDB(t,
		&model.Currency{}, &model.ExchangeRate{}, &model.Partner{},
		&model.Product{}, &model.ProductSource{})

	currencyRepo := repository.NewCurrencyRepository(db)
	rateRepo := repository.NewExchangeRateRepository(db)
	currencySvc := NewCurrencyService(currencyRepo, rateRepo)
	clock := func() time.Time { return now }

	if _, err := currencySvc.Create(ctx, dto.CurrencyInput{
		Code: "CNY", Name: "人民币", IsBase: boolPtr(true),
	}); err != nil {
		t.Fatalf("建本币失败: %v", err)
	}
	if _, err := currencySvc.Create(ctx, dto.CurrencyInput{Code: "USD", Name: "美元"}); err != nil {
		t.Fatalf("建外币失败: %v", err)
	}
	if _, err := NewExchangeRateService(rateRepo, currencyRepo).Create(ctx,
		dto.ExchangeRateInput{
			CurrencyCode:  "USD",
			Rate:          7.1,
			EffectiveDate: now.Format(rateDateLayout),
		}); err != nil {
		t.Fatalf("登记汇率失败: %v", err)
	}

	partnerRepo := repository.NewPartnerRepository(db)
	supplier := &model.Partner{
		Code: "S001", Name: "永康五金厂", Role: model.PartnerRoleSupplier,
		EntityType: model.PartnerEntityCompany, Status: model.EnableStatusOn,
	}
	customer := &model.Partner{
		Code: "C001", Name: "Ferreteria El Sol", Role: model.PartnerRoleCustomer,
		EntityType: model.PartnerEntityCompany, Status: model.EnableStatusOn,
	}
	for _, p := range []*model.Partner{supplier, customer} {
		if err := partnerRepo.Create(ctx, p); err != nil {
			t.Fatalf("建主体失败: %v", err)
		}
	}

	productRepo := repository.NewProductRepository(db)
	p1 := &model.Product{
		SKUCode: "BH01-0001", NameCN: "不锈钢合页", Unit: "PCS", Status: model.EnableStatusOn,
	}
	p2 := &model.Product{
		SKUCode: "BH01-0002", NameCN: "合页（彩盒装）", Unit: "PCS", Status: model.EnableStatusOn,
	}
	for _, p := range []*model.Product{p1, p2} {
		if err := productRepo.Create(ctx, p); err != nil {
			t.Fatalf("建产品失败: %v", err)
		}
	}

	sourceRepo := repository.NewProductSourceRepository(db)

	return sourceFixture{
		sources: NewProductSourceService(
			sourceRepo, productRepo, partnerRepo, currencySvc,
			WithProductSourceClock(clock),
		),
		products: NewProductService(
			productRepo, partnerRepo, sourceRepo, repository.NewProductCodeRepository(db),
		),
		productID:  p1.ID,
		product2ID: p2.ID,
		supplierID: supplier.ID,
		customerID: customer.ID,
	}
}

func TestProductSourceRules(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 10, 28, 10, 0, 0, 0, time.Local)
	fx := newSourceFixture(t, now)

	input := dto.ProductSourceInput{
		ProductID:     fx.productID,
		PartnerID:     fx.supplierID,
		SupplierSKU:   "  A-100  ",
		PurchasePrice: money.MustParse("1.2"),
		CurrencyCode:  "usd",
		MOQ:           1000,
		DeliveryDays:  25,
	}

	var created *model.ProductSource
	t.Run("按当日汇率折本币", func(t *testing.T) {
		m, err := fx.sources.Create(ctx, input)
		if err != nil {
			t.Fatalf("新增货源失败: %v", err)
		}
		created = m
		if m.CurrencyCode != "USD" {
			t.Errorf("currency_code = %q，期望归一化为 USD", m.CurrencyCode)
		}
		if m.SupplierSKU != "A-100" {
			t.Errorf("supplier_sku = %q，期望去掉首尾空格", m.SupplierSKU)
		}
		if m.PurchasePriceBase.Cmp(money.MustParse("8.52")) != 0 {
			t.Errorf("purchase_price_base = %s，期望 8.52", m.PurchasePriceBase)
		}
		if m.ExchangeRate.Cmp(money.MustParse("7.1")) != 0 {
			t.Errorf("exchange_rate = %s，期望 7.1", m.ExchangeRate)
		}
	})

	t.Run("重复货号被拒", func(t *testing.T) {
		_, err := fx.sources.Create(ctx, input)
		if e, ok := errs.From(err); !ok || e.Code != errs.CodeConflict {
			t.Fatalf("期望 40901，实际 %v", err)
		}
	})

	t.Run("同一供应商的空货号也不能重复", func(t *testing.T) {
		noSKU := input
		noSKU.SupplierSKU = ""
		noSKU.PurchasePrice = money.MustParse("1.1")
		if _, err := fx.sources.Create(ctx, noSKU); err != nil {
			t.Fatalf("第一条空货号应被允许: %v", err)
		}
		if _, err := fx.sources.Create(ctx, noSKU); err == nil {
			t.Fatal("第二条空货号应被拒")
		} else if e, ok := errs.From(err); !ok || e.Code != errs.CodeConflict {
			t.Fatalf("期望 40901，实际 %v", err)
		}
	})

	t.Run("另一个产品可以用同一货号", func(t *testing.T) {
		// 我方一个 SKU 拆成散装与彩盒两套编码、都从同一家同一个货号进货，
		// 是真实做法；按全局货号查重会把这种拆分判成重复。
		same := input
		same.ProductID = fx.product2ID
		if _, err := fx.sources.Create(ctx, same); err != nil {
			t.Fatalf("跨产品复用货号应被允许: %v", err)
		}
	})

	t.Run("非供应商角色被拒", func(t *testing.T) {
		bad := input
		bad.PartnerID = fx.customerID
		bad.SupplierSKU = "A-200"
		_, err := fx.sources.Create(ctx, bad)
		if e, ok := errs.From(err); !ok || e.Code != errs.CodeInvalidParam {
			t.Fatalf("期望 40001，实际 %v", err)
		}
	})

	t.Run("货源不能换供应商", func(t *testing.T) {
		moved := input
		moved.PartnerID = fx.customerID
		_, err := fx.sources.Update(ctx, created.ID, moved)
		if e, ok := errs.From(err); !ok || e.Code != errs.CodeInvalidParam {
			t.Fatalf("期望 40001，实际 %v", err)
		}
	})

	t.Run("停用状态不会被默认值覆盖", func(t *testing.T) {
		// md_product_source.status 刻意没有 default tag：写了之后显式传 0
		// 会被 gorm 省略、由数据库默认值填成 1，停用的货源又重新出现在比价列表里。
		off := int8(model.EnableStatusOff)
		m, err := fx.sources.Create(ctx, dto.ProductSourceInput{
			ProductID: fx.productID, PartnerID: fx.supplierID,
			SupplierSKU: "A-300", PurchasePrice: money.MustParse("2"), Status: &off,
		})
		if err != nil {
			t.Fatalf("新增停用货源失败: %v", err)
		}
		stored, err := fx.sources.Get(ctx, m.ID)
		if err != nil {
			t.Fatalf("查货源失败: %v", err)
		}
		if stored.Status != model.EnableStatusOff {
			t.Errorf("status = %d，期望停用（0）", stored.Status)
		}
	})

	t.Run("有货源的产品不能删", func(t *testing.T) {
		err := fx.products.Delete(ctx, fx.productID)
		if e, ok := errs.From(err); !ok || e.Code != errs.CodeConflict {
			t.Fatalf("期望 40901，实际 %v", err)
		}
	})
}

func approx(a, b float64) bool { return math.Abs(a-b) < 1e-9 }
