package service

import (
	"context"
	"testing"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/repository"
)

type productCodeFixture struct {
	products ProductService
	codes    ProductCodeService
	productA uint
	productB uint
}

func newProductCodeFixture(t *testing.T) productCodeFixture {
	t.Helper()
	ctx := context.Background()

	db := openTestDB(t, &model.Partner{}, &model.Product{}, &model.ProductCode{})
	partnerRepo := repository.NewPartnerRepository(db)
	productRepo := repository.NewProductRepository(db)
	codeRepo := repository.NewProductCodeRepository(db)

	a := &model.Product{
		SKUCode: "BH01-0001", NameCN: "不锈钢合页", Unit: "PCS", Status: model.EnableStatusOn,
	}
	b := &model.Product{
		SKUCode: "BH02-0001", NameCN: "抽屉滑轨", Unit: "SET", Status: model.EnableStatusOn,
	}
	for _, p := range []*model.Product{a, b} {
		if err := productRepo.Create(ctx, p); err != nil {
			t.Fatalf("建产品失败: %v", err)
		}
	}

	return productCodeFixture{
		products: NewProductService(
			productRepo, partnerRepo, repository.NewProductSourceRepository(db), codeRepo),
		codes:    NewProductCodeService(codeRepo, productRepo),
		productA: a.ID,
		productB: b.ID,
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
