package service

import (
	"context"
	"log/slog"
	"testing"

	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/repository"
)

// TestPDFAddressValues 覆盖 T-49 第三步的核心判断：单证按用途取地址，
// 取不到该用途时回退主体上那一列——存量主体没有多地址，不能因此印成空白。
//
// 断言 values 而不是解析 PDF：渲染结果里"这行到底印了没有"取决于版式，
// 而这一层要证明的是"取到的是哪一条"。
func TestPDFAddressValues(t *testing.T) {
	ctx := context.Background()
	repo := repository.NewPartnerAddressRepository(openTestDB(t, &model.PartnerAddress{}))
	svc := &pdfService{addresses: repo, log: slog.Default()}

	p := &model.Partner{
		Base:        model.Base{ID: 1},
		Name:        "ACME IMPORTS",
		Address:     "旧的主地址",
		Country:     "美国",
		ContactName: "Tom",
		Phone:       "555-0100",
	}

	// ① 还没配多地址：回退主表那一列，打印结果与改动前一致。
	values := map[string]string{}
	svc.partnerValues(ctx, values, p, model.PartnerAddressInvoice)
	if values["partner_address"] != "旧的主地址" || values["partner_country"] != "美国" {
		t.Fatalf("没有多地址时应回退主表地址，得到 %+v", values)
	}
	if values["partner_contact"] != "Tom 555-0100" {
		t.Errorf("回退时应使用主体的联系人与电话，得到 %q", values["partner_contact"])
	}

	// ② 配了发票地址：买方区块取它，不再用主表那一列。
	if err := repo.Create(ctx, &model.PartnerAddress{
		PartnerID: 1, Type: model.PartnerAddressInvoice,
		Country: "厄瓜多尔", Province: "Pichincha", City: "基多",
		Address: "Av. Amazonas 123", Contact: "Ana", Phone: "099-1",
	}); err != nil {
		t.Fatalf("新增发票地址失败: %v", err)
	}
	values = map[string]string{}
	svc.partnerValues(ctx, values, p, model.PartnerAddressInvoice)
	if want := "Pichincha, 基多, Av. Amazonas 123"; values["partner_address"] != want {
		t.Errorf("发票地址应为 %q，得到 %q", want, values["partner_address"])
	}
	if values["partner_country"] != "厄瓜多尔" {
		t.Errorf("国家应取自地址而不是主体，得到 %q", values["partner_country"])
	}

	// ③ 装箱单要的是收货地址：没配 delivery 时仍然回退，不能借用 invoice 那条。
	values = map[string]string{}
	svc.partnerValues(ctx, values, p, model.PartnerAddressDelivery)
	if values["partner_address"] != "旧的主地址" {
		t.Errorf("没有收货地址时应回退主表那一列，得到 %q", values["partner_address"])
	}

	// ④ 收货人 / 通知方区块：配了才印，没配时四个键全部为空（模板整块消失）。
	values = map[string]string{}
	svc.partyBlock(ctx, values, p, model.PartnerAddressConsignee, "consignee", "CONSIGNEE")
	if len(values) != 0 {
		t.Errorf("没有收货人地址时不应填充任何字段，得到 %+v", values)
	}

	if err := repo.Create(ctx, &model.PartnerAddress{
		PartnerID: 1, Type: model.PartnerAddressConsignee,
		Country: "厄瓜多尔", City: "瓜亚基尔", Address: "Puerto Marítimo 9",
	}); err != nil {
		t.Fatalf("新增收货人地址失败: %v", err)
	}
	values = map[string]string{}
	svc.partyBlock(ctx, values, p, model.PartnerAddressConsignee, "consignee", "CONSIGNEE")
	if values["consignee_label"] != "CONSIGNEE" || values["consignee_name"] != "ACME IMPORTS" {
		t.Errorf("收货人区块未填充: %+v", values)
	}
	if values["consignee_address"] != "瓜亚基尔, Puerto Marítimo 9" {
		t.Errorf("收货人地址拼接不正确: %q", values["consignee_address"])
	}

	// ⑤ addresses 为 nil（测试或未注入）时一律回退，不能 panic。
	bare := &pdfService{log: slog.Default()}
	values = map[string]string{}
	bare.partnerValues(ctx, values, p, model.PartnerAddressInvoice)
	if values["partner_address"] != "旧的主地址" {
		t.Errorf("未注入地址仓储时应回退，得到 %q", values["partner_address"])
	}
}
