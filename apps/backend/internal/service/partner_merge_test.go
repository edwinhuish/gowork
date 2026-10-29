package service

import (
	"context"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/errs"
	"github.com/edwinhuish/gowork/internal/pkg/money"
	"github.com/edwinhuish/gowork/internal/repository"
)

type partnerMergeFixture struct {
	svc PartnerService
	db  *gorm.DB
}

func newPartnerMergeFixture(t *testing.T) partnerMergeFixture {
	t.Helper()
	// 建表清单必须覆盖 partnerRefTablesForTest：少一张表，合并就会在那张表上失败。
	db := openTestDB(t,
		&model.Partner{}, &model.Product{}, &model.Currency{},
		&model.Contact{}, &model.Activity{}, &model.Lead{}, &model.Opportunity{},
		&model.ProductSource{}, &model.Attachment{},
		&model.Quotation{}, &model.SalesOrder{}, &model.PurchaseOrder{}, &model.Contract{},
		&model.Receivable{}, &model.Payable{}, &model.Payment{},
		&model.ShipmentCharge{}, &model.StockDocument{}, &model.User{},
	)
	return partnerMergeFixture{
		svc: NewPartnerService(
			repository.NewPartnerRepository(db),
			repository.NewProductRepository(db),
			repository.NewCurrencyRepository(db),
			repository.NewUserRepository(db),
		),
		db: db,
	}
}

// partnerRefTablesForTest 是"该被合并带走的引用"的独立清单。
//
// 故意不引用 repository 里的那份：那份漏了哪张表，引用这份就一起漏，测试也就白写了。
// 新增指向 md_partner 的外键时，两处都要补。
func partnerRefTablesForTest() []struct{ table, column string } {
	return []struct{ table, column string }{
		{"crm_partner_contact", "partner_id"},
		{"crm_activity", "partner_id"},
		{"crm_lead", "partner_id"},
		{"crm_opportunity", "partner_id"},
		{"biz_quotation", "partner_id"},
		{"biz_sales_order", "partner_id"},
		{"biz_purchase_order", "supplier_id"},
		{"biz_contract", "partner_id"},
		{"biz_receivable", "partner_id"},
		{"biz_payable", "partner_id"},
		{"biz_payment", "partner_id"},
		{"biz_shipment_charge", "partner_id"},
		{"inv_stock_document", "partner_id"},
		{"md_product", "supplier_id"},
		{"md_product_source", "partner_id"},
	}
}

func TestPartnerMerge(t *testing.T) {
	ctx := context.Background()
	f := newPartnerMergeFixture(t)

	customerRole := int8(model.PartnerRoleCustomer)
	supplierRole := int8(model.PartnerRoleSupplier)

	// 主记录：客户，国家已填、电话空着。被合并方：供应商，有电话与邮箱。
	survivor, err := f.svc.Create(ctx, dto.PartnerInput{
		Code: "CUS001", Name: "Ferreteria El Sol", Role: &customerRole, Country: "Ecuador",
	})
	if err != nil {
		t.Fatalf("新建主记录失败: %v", err)
	}
	source, err := f.svc.Create(ctx, dto.PartnerInput{
		Code: "SUP001", Name: "Ferreteria El Sol S.A.", Role: &supplierRole,
		Phone: "13800000009", Email: "ventas@elsol.com.ec",
	})
	if err != nil {
		t.Fatalf("新建被合并方失败: %v", err)
	}

	// 直接往库里造引用：各表的建单路径在别的用例里已经测过，
	// 这里要验的只是"合并有没有漏表"。
	contact := &model.Contact{
		PartnerID: source.ID, Name: "Carlos",
		IsPrimary: true, Status: model.EnableStatusOn,
	}
	mustCreate(t, f.db, contact)
	activity := &model.Activity{
		PartnerID: source.ID, Type: model.ActivityTypeCall, Subject: "第一次询盘",
		OccurredAt: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC),
	}
	mustCreate(t, f.db, activity)

	product := &model.Product{SKUCode: "BH01-0001-SS", NameCN: "304 不锈钢合页", Unit: "PCS"}
	mustCreate(t, f.db, product)
	src := &model.ProductSource{
		ProductID: product.ID, PartnerID: source.ID, SupplierSKU: "HY-4",
		CurrencyCode: "CNY", PurchasePrice: money.MustParse("6.8"),
		PurchasePriceBase: money.MustParse("6.8"), ExchangeRate: money.MustParse("1"),
		Status: model.EnableStatusOn,
	}
	mustCreate(t, f.db, src)

	attachment := &model.Attachment{
		EntityType: model.AttachPartner, EntityID: source.ID,
		FileName: "营业执照.pdf", MimeType: "application/pdf", Size: 1024,
	}
	mustCreate(t, f.db, attachment)

	res, err := f.svc.Merge(ctx, survivor.ID, dto.PartnerMergeInput{SourceID: source.ID})
	if err != nil {
		t.Fatalf("合并失败: %v", err)
	}

	t.Run("角色取并集，空字段被补齐，自有字段不被覆盖", func(t *testing.T) {
		if got := res.Partner.Role; got != model.PartnerRoleCustomer|model.PartnerRoleSupplier {
			t.Errorf("合并后角色 = %s，期望 customer+supplier", got)
		}
		if res.Partner.Phone != "13800000009" {
			t.Errorf("主记录电话应为被合并方的值，实际 %q", res.Partner.Phone)
		}
		if res.Partner.Country != "Ecuador" {
			t.Errorf("主记录自己填过的国家不该被覆盖，实际 %q", res.Partner.Country)
		}
		if res.Partner.Name != "Ferreteria El Sol" {
			t.Errorf("名称应保留主记录的，实际 %q", res.Partner.Name)
		}
	})

	t.Run("引用整体迁到主记录", func(t *testing.T) {
		var got model.Contact
		f.db.First(&got, contact.ID)
		if got.PartnerID != survivor.ID {
			t.Errorf("联系人仍指向 %d，期望 %d", got.PartnerID, survivor.ID)
		}
		var act model.Activity
		f.db.First(&act, activity.ID)
		if act.PartnerID != survivor.ID {
			t.Errorf("跟进记录仍指向 %d，期望 %d", act.PartnerID, survivor.ID)
		}
		var ps model.ProductSource
		f.db.First(&ps, src.ID)
		if ps.PartnerID != survivor.ID {
			t.Errorf("货源仍指向 %d，期望 %d", ps.PartnerID, survivor.ID)
		}
		var att model.Attachment
		f.db.First(&att, attachment.ID)
		if att.EntityID != survivor.ID {
			t.Errorf("附件仍挂在 %d，期望 %d", att.EntityID, survivor.ID)
		}
		if res.Moved["crm_partner_contact"] != 1 || res.Moved["sys_attachment"] != 1 {
			t.Errorf("迁移条数不对: %+v", res.Moved)
		}
	})

	t.Run("全库不再有行指向被合并方", func(t *testing.T) {
		for _, ref := range partnerRefTablesForTest() {
			var n int64
			if err := f.db.Table(ref.table).Where(ref.column+" = ?", source.ID).Count(&n).Error; err != nil {
				t.Fatalf("统计 %s: %v", ref.table, err)
			}
			if n != 0 {
				t.Errorf("%s 还有 %d 行指向被合并方", ref.table, n)
			}
		}
		var n int64
		f.db.Table("sys_attachment").
			Where("entity_type = ? AND entity_id = ?", model.AttachPartner, source.ID).Count(&n)
		if n != 0 {
			t.Errorf("sys_attachment 还有 %d 行指向被合并方", n)
		}
	})

	t.Run("被合并方软删而不是消失", func(t *testing.T) {
		_, err := f.svc.Get(ctx, source.ID)
		wantCode(t, err, errs.CodeNotFound, "已软删的主体查不到")
		restored, err := f.svc.Restore(ctx, source.ID)
		if err != nil {
			t.Fatalf("恢复失败: %v", err)
		}
		if restored.ID != source.ID {
			t.Errorf("恢复出来的是 id=%d，期望 %d", restored.ID, source.ID)
		}
	})
}

// 合并之后一个主体不能有两个主联系人：发货通知该发给谁，谁也说不清。
func TestPartnerMergeDemotesPrimaryContact(t *testing.T) {
	ctx := context.Background()
	f := newPartnerMergeFixture(t)

	role := int8(model.PartnerRoleCustomer)
	survivor, err := f.svc.Create(ctx, dto.PartnerInput{Code: "CUS001", Name: "甲方", Role: &role})
	if err != nil {
		t.Fatalf("新建失败: %v", err)
	}
	source, err := f.svc.Create(ctx, dto.PartnerInput{Code: "CUS002", Name: "甲方（重复录入）", Role: &role})
	if err != nil {
		t.Fatalf("新建失败: %v", err)
	}

	keep := &model.Contact{PartnerID: survivor.ID, Name: "老板", IsPrimary: true, Status: model.EnableStatusOn}
	mustCreate(t, f.db, keep)
	demoted := &model.Contact{PartnerID: source.ID, Name: "采购", IsPrimary: true, Status: model.EnableStatusOn}
	mustCreate(t, f.db, demoted)

	if _, err := f.svc.Merge(ctx, survivor.ID, dto.PartnerMergeInput{SourceID: source.ID}); err != nil {
		t.Fatalf("合并失败: %v", err)
	}

	var primaries []model.Contact
	f.db.Where("partner_id = ? AND is_primary = ? AND deleted_at = 0", survivor.ID, true).Find(&primaries)
	if len(primaries) != 1 || primaries[0].ID != keep.ID {
		t.Fatalf("应只剩主记录自己那个主联系人，实际 %+v", primaries)
	}
	// 降级不等于删掉：被合并方的主联系人仍是这家的联系人，只是不再是"主"。
	var got model.Contact
	f.db.First(&got, demoted.ID)
	if got.PartnerID != survivor.ID || got.IsPrimary {
		t.Errorf("被合并方的主联系人应保留并降级，实际 partner_id=%d is_primary=%v",
			got.PartnerID, got.IsPrimary)
	}
}

func TestPartnerMergeRejects(t *testing.T) {
	ctx := context.Background()
	f := newPartnerMergeFixture(t)

	role := int8(model.PartnerRoleCustomer)
	a, err := f.svc.Create(ctx, dto.PartnerInput{Code: "CUS001", Name: "甲方", Role: &role})
	if err != nil {
		t.Fatalf("新建失败: %v", err)
	}
	b, err := f.svc.Create(ctx, dto.PartnerInput{Code: "CUS002", Name: "乙方", Role: &role})
	if err != nil {
		t.Fatalf("新建失败: %v", err)
	}

	t.Run("不能合并到自己", func(t *testing.T) {
		_, err := f.svc.Merge(ctx, a.ID, dto.PartnerMergeInput{SourceID: a.ID})
		wantCode(t, err, errs.CodeInvalidParam, "自合并")
	})

	t.Run("被合并方不存在", func(t *testing.T) {
		_, err := f.svc.Merge(ctx, a.ID, dto.PartnerMergeInput{SourceID: 9999})
		wantCode(t, err, errs.CodeInvalidParam, "被合并方不存在")
	})

	t.Run("主记录被改过时拒绝", func(t *testing.T) {
		stale := a.UpdatedAt
		// 换一次版本：模拟"打开合并弹窗之后，别人先在编辑页保存了一次"。
		_, err := f.svc.Update(ctx, a.ID, dto.PartnerInput{Code: "CUS001", Name: "甲方"})
		if err != nil {
			t.Fatalf("改主记录失败: %v", err)
		}
		_, err = f.svc.Merge(ctx, a.ID, dto.PartnerMergeInput{SourceID: b.ID, UpdatedAt: &stale})
		wantCode(t, err, errs.CodeConflict, "主记录被改过")
	})
}

// 同一产品同一货号在两家都有：合并后就是重复录入，而货源是报价的成本依据，
// 系统不能替人决定丢掉哪一条。
func TestPartnerMergeRejectsProductSourceConflict(t *testing.T) {
	ctx := context.Background()
	f := newPartnerMergeFixture(t)

	role := int8(model.PartnerRoleSupplier)
	a, err := f.svc.Create(ctx, dto.PartnerInput{Code: "SUP001", Name: "华信五金", Role: &role})
	if err != nil {
		t.Fatalf("新建失败: %v", err)
	}
	b, err := f.svc.Create(ctx, dto.PartnerInput{Code: "SUP002", Name: "华信五金（重复）", Role: &role})
	if err != nil {
		t.Fatalf("新建失败: %v", err)
	}

	product := &model.Product{SKUCode: "BH01-0001-SS", NameCN: "304 不锈钢合页", Unit: "PCS"}
	mustCreate(t, f.db, product)
	for _, partnerID := range []uint{a.ID, b.ID} {
		mustCreate(t, f.db, &model.ProductSource{
			ProductID: product.ID, PartnerID: partnerID, SupplierSKU: "HY-4",
			CurrencyCode: "CNY", PurchasePrice: money.MustParse("6.8"),
			PurchasePriceBase: money.MustParse("6.8"), ExchangeRate: money.MustParse("1"),
			Status: model.EnableStatusOn,
		})
	}

	_, err = f.svc.Merge(ctx, a.ID, dto.PartnerMergeInput{SourceID: b.ID})
	wantCode(t, err, errs.CodeConflict, "货源冲突")
	// 拦下来之后两条主体都还在：合并失败不该留下半边状态。
	if _, err := f.svc.Get(ctx, b.ID); err != nil {
		t.Fatalf("被合并方不该被删: %v", err)
	}
}

func mustCreate(t *testing.T, db *gorm.DB, m any) {
	t.Helper()
	if err := db.Create(m).Error; err != nil {
		t.Fatalf("造数据失败: %v", err)
	}
}
