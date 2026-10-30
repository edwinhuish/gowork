package service

import (
	"context"
	"testing"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/model"
	codenorm "github.com/edwinhuish/gowork/internal/pkg/code"
	"github.com/edwinhuish/gowork/internal/repository"
)

type productCodeFixture struct {
	products ProductService
	codes    ProductCodeService
	sources  repository.ProductSourceRepository
	// 仓储本身也留着：有的用例要绕过 service 造出"历史遗留"的数据形态，
	// 比如产品已软删、编号行却还在的孤儿。
	productRepo repository.ProductRepository
	codeRepo    repository.ProductCodeRepository
	productA    uint
	productB    uint
	supplierID  uint
}

func newProductCodeFixture(t *testing.T) productCodeFixture {
	t.Helper()
	ctx := context.Background()

	db := openTestDB(t,
		&model.Partner{}, &model.Product{}, &model.ProductCode{}, &model.ProductSource{})
	partnerRepo := repository.NewPartnerRepository(db)
	productRepo := repository.NewProductRepository(db)
	codeRepo := repository.NewProductCodeRepository(db)
	sourceRepo := repository.NewProductSourceRepository(db)

	supplier := &model.Partner{
		Code: "SUP001", Name: "永康五金厂", Role: model.PartnerRoleSupplier,
		EntityType: model.PartnerEntityCompany,
	}
	if err := partnerRepo.Create(ctx, supplier); err != nil {
		t.Fatalf("建供应商失败: %v", err)
	}

	a := &model.Product{
		SKUCode: "BH01-0001", NameCN: "不锈钢合页", Unit: "PCS",
	}
	b := &model.Product{
		SKUCode: "BH02-0001", NameCN: "抽屉滑轨", Unit: "SET",
	}
	for _, p := range []*model.Product{a, b} {
		if err := productRepo.Create(ctx, p); err != nil {
			t.Fatalf("建产品失败: %v", err)
		}
	}

	return productCodeFixture{
		products: NewProductService(
			productRepo, partnerRepo, sourceRepo, codeRepo, repository.NewTxManager(db)),
		codes:       NewProductCodeService(codeRepo, productRepo),
		sources:     sourceRepo,
		productRepo: productRepo,
		codeRepo:    codeRepo,
		productA:    a.ID,
		productB:    b.ID,
		supplierID:  supplier.ID,
	}
}

// search 用关键词搜一次产品列表，返回结果与总数（总数与列表必须是同一组条件）。
func (f productCodeFixture) search(
	t *testing.T, keyword string,
) *dto.PageResult[model.Product] {
	t.Helper()
	res, err := f.products.List(context.Background(), dto.ProductQuery{
		PageQuery: dto.PageQuery{Keyword: keyword},
	})
	if err != nil {
		t.Fatalf("搜索 %q 失败: %v", keyword, err)
	}
	return res
}

func TestProductCodeCreateRules(t *testing.T) {
	ctx := context.Background()
	f := newProductCodeFixture(t)

	if _, err := f.codes.Create(ctx, dto.ProductCodeInput{
		ProductID: f.productA, Code: "ABC-123", Source: model.ProductCodeSourceCustomer,
	}); err != nil {
		t.Fatalf("新增编号失败: %v", err)
	}

	// 同一个产品下换个写法再加一次：大小写与分隔符不区分，否则客户换个写法就录两遍。
	if _, err := f.codes.Create(ctx, dto.ProductCodeInput{
		ProductID: f.productA, Code: "abc 123",
	}); err == nil {
		t.Error("同一产品的重复编号应被拒")
	}

	// 不同产品可以共用：两家客户从同一个工厂拿货，工厂号相同是常态。
	if _, err := f.codes.Create(ctx, dto.ProductCodeInput{
		ProductID: f.productB, Code: "ABC123", Source: model.ProductCodeSourceFactory,
	}); err != nil {
		t.Errorf("不同产品共用同一编号应被允许: %v", err)
	}

	// 编号等于自己的 SKU 不必再录一遍：列表上「命中编号」与主标识会是同一个词。
	if _, err := f.codes.Create(ctx, dto.ProductCodeInput{
		ProductID: f.productA, Code: "bh01 0001",
	}); err == nil {
		t.Error("编号等于自己的 SKU 时应被拒")
	}
}

func TestProductListFindsByCode(t *testing.T) {
	ctx := context.Background()
	f := newProductCodeFixture(t)

	if _, err := f.codes.Create(ctx, dto.ProductCodeInput{
		ProductID: f.productA, Code: "KT-7788", Source: model.ProductCodeSourceSupplier,
	}); err != nil {
		t.Fatalf("新增编号失败: %v", err)
	}

	// 按编号的另一种写法搜：空格代替连字符，且小写。
	res := f.search(t, "kt 7788")
	if res.Total != 1 || len(res.List) != 1 {
		t.Fatalf("按编号搜索命中 %d 条（total=%d），期望只有 1 条", len(res.List), res.Total)
	}
	if res.List[0].ID != f.productA {
		t.Errorf("命中的产品 = %d，期望 %d", res.List[0].ID, f.productA)
	}
	// 回填原始写法：用户要知道是哪个号命中的。
	if got := res.List[0].MatchedCodes; len(got) != 1 || got[0] != "KT-7788" {
		t.Errorf("命中编号 = %v，期望 [KT-7788]", got)
	}

	// 关键词与编号无关时不该被编号表带出结果。
	if other := f.search(t, "ZZZ"); other.Total != 0 {
		t.Errorf("无关关键词命中 %d 条", other.Total)
	}
}

// 改 SKU 之后旧号仍要能搜到：客户手里的报价单、邮件用的还是旧号，
// 搜不到就等于这单在系统里对不上。
func TestProductSKUChangeArchivesOldCode(t *testing.T) {
	ctx := context.Background()
	f := newProductCodeFixture(t)

	if _, err := f.products.Update(ctx, f.productA, dto.ProductInput{
		SKUCode: "BH01-9001", NameCN: "不锈钢合页", Unit: "PCS",
	}); err != nil {
		t.Fatalf("改 SKU 失败: %v", err)
	}

	res := f.search(t, "BH01 0001")
	if res.Total != 1 || len(res.List) != 1 || res.List[0].ID != f.productA {
		t.Fatalf("旧 SKU 搜不到：命中 %d 条", res.Total)
	}
	if got := res.List[0].MatchedCodes; len(got) != 1 || got[0] != "BH01-0001" {
		t.Errorf("命中编号 = %v，期望 [BH01-0001]", got)
	}

	// 新 SKU 不得等于别的产品用过的编号：否则搜索结果里两条货看着一样。
	if _, err := f.codes.Create(ctx, dto.ProductCodeInput{
		ProductID: f.productB, Code: "BH01-9002",
	}); err != nil {
		t.Fatalf("给 B 加编号失败: %v", err)
	}
	if _, err := f.products.Update(ctx, f.productA, dto.ProductInput{
		SKUCode: "BH01-9002", NameCN: "不锈钢合页", Unit: "PCS",
	}); err == nil {
		t.Error("新 SKU 撞上其他产品用过的编号时应被拒")
	}

	// 改回自己的旧号是允许的：那是自己的码，不是别人的。
	if _, err := f.products.Update(ctx, f.productA, dto.ProductInput{
		SKUCode: "BH01-0001", NameCN: "不锈钢合页", Unit: "PCS",
	}); err != nil {
		t.Errorf("改回自己的旧 SKU 应被允许: %v", err)
	}
}

// 供应商货号挂在 md_product_source 上，不在编号表里：按它搜产品也要能命中，
// 否则「录在货源面板的货号」和「能搜到的货号」就是两套东西。
func TestProductListFindsBySupplierSKU(t *testing.T) {
	ctx := context.Background()
	f := newProductCodeFixture(t)

	src := &model.ProductSource{
		ProductID:       f.productA,
		PartnerID:       f.supplierID,
		SupplierSKU:     "A-100",
		SupplierSKUNorm: codenorm.Normalize("A-100"),
		CurrencyCode:    "CNY",
	}
	if err := f.sources.Create(ctx, src); err != nil {
		t.Fatalf("建货源失败: %v", err)
	}

	// 换个写法搜：小写 + 空格代替连字符。
	res := f.search(t, "a 100")
	if res.Total != 1 || len(res.List) != 1 || res.List[0].ID != f.productA {
		t.Fatalf("按供应商货号搜索命中 %d 条（total=%d），期望只有 A", len(res.List), res.Total)
	}
	if got := res.List[0].MatchedCodes; len(got) != 1 || got[0] != "A-100" {
		t.Errorf("命中编号 = %v，期望 [A-100]（原写法）", got)
	}

	// 软删的货源不再参与检索：那条子查询是原生 SQL，不经过 gorm 的软删条件，
	// 少了 deleted_at = 0，删掉的货号会一直把产品带出来。
	if err := f.sources.Delete(ctx, src.ID); err != nil {
		t.Fatalf("删除货源失败: %v", err)
	}
	if gone := f.search(t, "a 100"); gone.Total != 0 {
		t.Errorf("软删的货源仍被搜到：%d 条", gone.Total)
	}

	// 同一个号既录进编号表、又挂在货源上时只提示一次：提示里出现两遍，
	// 用户会以为这是两个不同的编号。
	if err := f.sources.Create(ctx, &model.ProductSource{
		ProductID:       f.productA,
		PartnerID:       f.supplierID,
		SupplierSKU:     "A-100",
		SupplierSKUNorm: codenorm.Normalize("A-100"),
		CurrencyCode:    "CNY",
	}); err != nil {
		t.Fatalf("重建货源失败: %v", err)
	}
	if _, err := f.codes.Create(ctx, dto.ProductCodeInput{
		ProductID: f.productA, Code: "A 100", Source: model.ProductCodeSourceSupplier,
	}); err != nil {
		t.Fatalf("新增编号失败: %v", err)
	}
	merged := f.search(t, "A100")
	if merged.Total != 1 || len(merged.List) != 1 {
		t.Fatalf("同一个号录了两处，命中 %d 条（total=%d），期望 1 条", len(merged.List), merged.Total)
	}
	if got := merged.List[0].MatchedCodes; len(got) != 1 {
		t.Errorf("命中编号 = %v，期望去重后只剩 1 条", got)
	}
}

// 产品删掉时它的编号必须一起清掉：孤儿编号没有入口能删，
// 却会让「新 SKU 不得等于任何产品用过的编号」永久拒绝那个号。
func TestProductDeleteReleasesItsCodes(t *testing.T) {
	ctx := context.Background()
	f := newProductCodeFixture(t)

	if _, err := f.codes.Create(ctx, dto.ProductCodeInput{
		ProductID: f.productA, Code: "KT-7788", Source: model.ProductCodeSourceCustomer,
	}); err != nil {
		t.Fatalf("新增编号失败: %v", err)
	}

	if err := f.products.Delete(ctx, f.productA); err != nil {
		t.Fatalf("删除产品失败: %v", err)
	}

	left, err := f.codeRepo.ListByProductID(ctx, f.productA)
	if err != nil {
		t.Fatalf("查编号失败: %v", err)
	}
	if len(left) != 0 {
		t.Fatalf("产品删除后仍留下 %d 条编号", len(left))
	}

	if _, err := f.products.Create(ctx, dto.ProductInput{
		SKUCode: "KT-7788", NameCN: "同号新产品", Unit: "PCS",
	}); err != nil {
		t.Errorf("用已删产品的旧编号做新 SKU 应被允许: %v", err)
	}
}

// 历史数据里可能已经躺着"产品已删、编号还在"的孤儿（清理只在删除那条路径上做），
// 孤儿不该再占号：它没有任何界面入口，占着号等于那个号凭空消失。
func TestSoftDeletedProductCodesDoNotReserveSKU(t *testing.T) {
	ctx := context.Background()
	f := newProductCodeFixture(t)

	if err := f.codeRepo.Create(ctx, &model.ProductCode{
		ProductID: f.productA,
		Code:      "ZZ-9",
		CodeNorm:  codenorm.Normalize("ZZ-9"),
		Source:    model.ProductCodeSourceOther,
	}); err != nil {
		t.Fatalf("造孤儿编号失败: %v", err)
	}
	// 绕过 service 直接软删产品，编号就留在库里了。
	if err := f.productRepo.Delete(ctx, f.productA); err != nil {
		t.Fatalf("软删产品失败: %v", err)
	}

	if _, err := f.products.Create(ctx, dto.ProductInput{
		SKUCode: "ZZ-9", NameCN: "新产品", Unit: "PCS",
	}); err != nil {
		t.Errorf("已删产品的孤儿编号仍占着号: %v", err)
	}
}

// 编号删除是硬删：软删会让「删掉再加回来」撞上唯一索引，而列表里又看不见那行。
func TestProductCodeDeleteThenRecreate(t *testing.T) {
	ctx := context.Background()
	f := newProductCodeFixture(t)

	m, err := f.codes.Create(ctx, dto.ProductCodeInput{ProductID: f.productA, Code: "OLD-1"})
	if err != nil {
		t.Fatalf("新增编号失败: %v", err)
	}
	if err := f.codes.Delete(ctx, m.ID); err != nil {
		t.Fatalf("删除编号失败: %v", err)
	}
	if _, err := f.codes.Create(ctx, dto.ProductCodeInput{
		ProductID: f.productA, Code: "old 1",
	}); err != nil {
		t.Errorf("删除后重新添加同一编号应被允许: %v", err)
	}
}
