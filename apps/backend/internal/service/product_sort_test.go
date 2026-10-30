package service

import (
	"context"
	"testing"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/money"
	"github.com/edwinhuish/gowork/internal/repository"
)

// 列表排序只认白名单字段，其余一律退回默认顺序（id DESC），
// 这里把两条都钉住：字段对了要真的按它排，字段错了要安静地退回去而不是 500。
func TestProductListSort(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t, &model.Partner{}, &model.Product{}, &model.ProductCode{}, &model.ProductSource{})
	partnerRepo := repository.NewPartnerRepository(db)
	productRepo := repository.NewProductRepository(db)
	codeRepo := repository.NewProductCodeRepository(db)
	sourceRepo := repository.NewProductSourceRepository(db)
	svc := NewProductService(
		productRepo, partnerRepo, sourceRepo, codeRepo, repository.NewTxManager(db))

	// 售价刻意与编码顺序相反，才分得清"按售价排"和"按 id 排"。
	rows := []*model.Product{
		{SKUCode: "AA-001", NameCN: "甲", Unit: "PCS", SalePrice: money.MustParse("30")},
		{SKUCode: "BB-001", NameCN: "乙", Unit: "PCS", SalePrice: money.MustParse("10")},
		{SKUCode: "CC-001", NameCN: "丙", Unit: "PCS", SalePrice: money.MustParse("20")},
	}
	ids := make([]uint, 0, len(rows))
	for _, p := range rows {
		if err := productRepo.Create(ctx, p); err != nil {
			t.Fatalf("建产品失败: %v", err)
		}
		ids = append(ids, p.ID)
	}

	cases := []struct {
		name   string
		field  string
		order  string
		expect []uint
	}{
		{"售价升序", "sale_price", "asc", []uint{ids[1], ids[2], ids[0]}},
		{"售价降序", "sale_price", "desc", []uint{ids[0], ids[2], ids[1]}},
		{"编码升序", "sku_code", "asc", []uint{ids[0], ids[1], ids[2]}},
		{"不认识的字段退回默认顺序", "drop table md_product", "asc", []uint{ids[2], ids[1], ids[0]}},
		{"不传字段即默认顺序", "", "", []uint{ids[2], ids[1], ids[0]}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := svc.List(ctx, dto.ProductQuery{
				PageQuery: dto.PageQuery{Page: 1, PageSize: 20},
				SortField: c.field, SortOrder: c.order,
			})
			if err != nil {
				t.Fatalf("查询失败: %v", err)
			}
			if len(got.List) != len(c.expect) {
				t.Fatalf("返回 %d 条，期望 %d 条", len(got.List), len(c.expect))
			}
			for i, want := range c.expect {
				if got.List[i].ID != want {
					t.Fatalf("第 %d 条 id = %d，期望 %d（顺序：%v）", i, got.List[i].ID, want, c.expect)
				}
			}
		})
	}
}
