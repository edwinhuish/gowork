package service

import (
	"context"
	"testing"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/repository"
)

func newImportTestService(t *testing.T) PartnerService {
	t.Helper()
	db := openTestDB(t, &model.Partner{}, &model.Product{}, &model.Currency{}, &model.User{})
	return NewPartnerService(
		repository.NewPartnerRepository(db),
		repository.NewProductRepository(db),
		repository.NewCurrencyRepository(db),
		repository.NewUserRepository(db),
	)
}

func importRows() []dto.PartnerImportRow {
	return []dto.PartnerImportRow{
		{Line: 2, Code: "CUS001", Name: "Ferreteria El Sol", Role: "customer", Country: "Ecuador",
			Phone: "+593 99 123 4567", CreditDays: "30", CreditLimit: "5000"},
		{Line: 3, Code: "SUP001", Name: "宁波五金", Role: "供应商", Country: "China"},
		// 角色是中文 + 英文组合，导出文件里就是这个样子。
		{Line: 4, Code: "BOTH01", Name: "客供两用", Role: "customer+supplier"},
		{Line: 5, Code: "", Name: "没编码", Role: "customer"},
		{Line: 6, Code: "CUS001", Name: "文件内重复", Role: "customer"},
		{Line: 7, Code: "BAD01", Name: "角色写错", Role: "代理"},
	}
}

// dry_run 只出报告不落库：一份几百行的文件里错两三行是常态，
// 先让人看清"哪几行会被跳过"，改完再导。
func TestImportPartnersDryRun(t *testing.T) {
	svc := newImportTestService(t)
	ctx := context.Background()

	report, err := svc.ImportPartners(ctx, importRows(), true)
	if err != nil {
		t.Fatalf("导入失败: %v", err)
	}
	if report.Total != 6 {
		t.Errorf("总行数应为 6，得到 %d", report.Total)
	}
	if report.Created != 0 {
		t.Errorf("dry_run 不该落库，得到 created=%d", report.Created)
	}
	if len(report.Failures) != 3 {
		t.Fatalf("应有三行失败（空编码 / 文件内重复 / 角色非法），得到 %+v", report.Failures)
	}
	// 失败行要带行号：让人能回到文件里那一行去改。
	for _, f := range report.Failures {
		if f.Line == 0 || f.Reason == "" {
			t.Errorf("失败行缺少行号或原因: %+v", f)
		}
	}

	res, err := svc.List(ctx, dto.PartnerQuery{})
	if err != nil {
		t.Fatalf("查列表失败: %v", err)
	}
	if res.Total != 0 {
		t.Errorf("dry_run 之后库里不该有主体，得到 %d", res.Total)
	}
}

func TestImportPartnersCreates(t *testing.T) {
	svc := newImportTestService(t)
	ctx := context.Background()

	report, err := svc.ImportPartners(ctx, importRows(), false)
	if err != nil {
		t.Fatalf("导入失败: %v", err)
	}
	if report.Created != 3 || report.Skipped != 0 {
		t.Errorf("应导入 3 行，得到 created=%d skipped=%d", report.Created, report.Skipped)
	}

	list, err := svc.List(ctx, dto.PartnerQuery{})
	if err != nil {
		t.Fatalf("查列表失败: %v", err)
	}
	if list.Total != 3 {
		t.Fatalf("库里应有 3 个主体，得到 %d", list.Total)
	}
	for _, p := range list.List {
		if p.Code == "BOTH01" {
			// 组合角色取并集：导出时是 customer+supplier，导回来必须还是两个角色。
			if !p.Role.Has(model.PartnerRoleCustomer) || !p.Role.Has(model.PartnerRoleSupplier) {
				t.Errorf("组合角色没解析对: %d", p.Role)
			}
		}
	}

	// 再导一次同一份文件：编码已存在，全部失败而不是建出重复主体。
	again, err := svc.ImportPartners(ctx, importRows(), false)
	if err != nil {
		t.Fatalf("二次导入失败: %v", err)
	}
	if again.Created != 0 {
		t.Errorf("重复导入不该新建，得到 created=%d", again.Created)
	}
}

// 查重提示不阻断：外贸同名公司跨国家确实存在，拦住等于让人把询盘丢掉。
func TestImportPartnersDuplicateIsHintOnly(t *testing.T) {
	svc := newImportTestService(t)
	ctx := context.Background()

	first := []dto.PartnerImportRow{
		{Line: 2, Code: "CUS001", Name: "Ferreteria El Sol", Role: "customer",
			Country: "Ecuador", Email: "buyer@solar.com.ec"},
	}
	if _, err := svc.ImportPartners(ctx, first, false); err != nil {
		t.Fatalf("首次导入失败: %v", err)
	}

	second := []dto.PartnerImportRow{
		{Line: 2, Code: "CUS002", Name: "Ferreteria El Sol S.A.", Role: "customer",
			Country: "Ecuador", Email: "buyer@solar.com.ec"},
	}
	report, err := svc.ImportPartners(ctx, second, false)
	if err != nil {
		t.Fatalf("二次导入失败: %v", err)
	}
	if report.Created != 1 {
		t.Errorf("可疑重复不该阻断，得到 created=%d failures=%+v", report.Created, report.Failures)
	}
	if len(report.Duplicates) != 1 {
		t.Fatalf("应提示 1 条可疑重复，得到 %+v", report.Duplicates)
	}
	if len(report.Duplicates[0].Reasons) == 0 {
		t.Error("提示要说明像在哪里（名称+国家 / 邮箱同域 / 电话）")
	}
}
