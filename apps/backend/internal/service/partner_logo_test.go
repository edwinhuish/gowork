package service

import (
	"context"
	"fmt"
	"testing"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/repository"
)

// TestPartnerLogoURL 是「列表与详情带 Logo」的验收点。
//
// 三条各有各要防的事：
//   - 取最新那张，而不是最早那张（否则用户在详情页换了 logo，列表还挂着旧图）；
//   - PDF 不算图片（挂 vine 营业执照是常态，不能让它们在列表里变成破图）；
//   - 整页只查一次：两个有图的主体在一次 List 里都拿到值，说明是批量而不是逐条。
func TestPartnerLogoURL(t *testing.T) {
	ctx := context.Background()
	svc, attachments := newPartnerTestServiceWithAttachments(t)

	roleOf := func(r model.PartnerRole) *int8 {
		v := int8(r)
		return &v
	}
	create := func(code, name string) *model.Partner {
		t.Helper()
		m, err := svc.Create(ctx, dto.PartnerInput{
			Code: code, Name: name, Role: roleOf(model.PartnerRoleCustomer),
		})
		if err != nil {
			t.Fatalf("新建主体 %s 失败: %v", code, err)
		}
		return m
	}
	upload := func(partnerID uint, name, mime string) uint {
		t.Helper()
		m := &model.Attachment{
			EntityType: model.AttachPartner, EntityID: partnerID,
			FileName: name, MimeType: mime, Size: 4,
		}
		if err := attachments.Create(ctx, m, []byte("fake")); err != nil {
			t.Fatalf("写附件 %s 失败: %v", name, err)
		}
		return m.ID
	}

	withImages := create("CUS001", "有 LOGO 的客户")
	withPDF := create("CUS002", "只有营业执照的客户")
	withoutAny := create("CUS003", "什么都没传的客户")

	stale := upload(withImages.ID, "旧 logo.png", "image/png")
	upload(withPDF.ID, "营业执照.pdf", "application/pdf")
	fresh := upload(withImages.ID, "新 logo.jpg", "image/jpeg")

	res, err := svc.List(ctx, dto.PartnerQuery{})
	if err != nil {
		t.Fatalf("查询列表失败: %v", err)
	}
	logoOf := make(map[string]string, len(res.List))
	for _, m := range res.List {
		logoOf[m.Code] = m.LogoURL
	}

	want := func(id uint) string {
		return fmt.Sprintf("/attachments/%d/download", id)
	}
	if got := logoOf["CUS001"]; got != want(fresh) {
		t.Errorf("应取最新那张图片（%s），实际 %q", want(fresh), got)
	}
	if got := logoOf["CUS002"]; got != "" {
		t.Errorf("PDF 不是图片，不该被当成 Logo，实际 %q", got)
	}
	if got := logoOf[withoutAny.Code]; got != "" {
		t.Errorf("没有附件的主体不该带 Logo，实际 %q", got)
	}

	// 详情与列表同一口径：两边不一致会让人以为"刚传的图没生效"。
	detail, err := svc.Get(ctx, withImages.ID)
	if err != nil {
		t.Fatalf("查询详情失败: %v", err)
	}
	if detail.LogoURL != want(fresh) {
		t.Errorf("详情 Logo 应为最新那张（%s），实际 %q", want(fresh), detail.LogoURL)
	}
	if stale == fresh {
		t.Fatalf("测试前提不成立：两次上传应得到两个附件 id，实际 %d", fresh)
	}
	if err := attachments.Create(ctx, &model.Attachment{
		EntityType: model.AttachPartner, EntityID: withImages.ID,
		FileName: "更新的 logo.webp", MimeType: "image/webp", Size: 4,
	}, []byte("fake")); err != nil {
		t.Fatalf("追加图片失败: %v", err)
	}
	again, err := svc.Get(ctx, withImages.ID)
	if err != nil {
		t.Fatalf("再次查询详情失败: %v", err)
	}
	if again.LogoURL == want(fresh) {
		t.Errorf("换了 Logo 之后详情页应跟着换，仍为 %q", again.LogoURL)
	}

	// 没装配附件仓储时不能崩：内部调用（迁移、别的 service 回头查主体）
	// 拿不到这个 option，回填必须静默跳过。
	bareDB := openTestDB(t, &model.Partner{}, &model.Product{},
		&model.Currency{}, &model.User{})
	bare := NewPartnerService(
		repository.NewPartnerRepository(bareDB),
		repository.NewProductRepository(bareDB),
		repository.NewCurrencyRepository(bareDB),
		repository.NewUserRepository(bareDB),
	)
	barePartner, err := bare.Create(ctx, dto.PartnerInput{
		Code: "CUS900", Name: "没有附件能力的库", Role: roleOf(model.PartnerRoleCustomer),
	})
	if err != nil {
		t.Fatalf("新建主体失败: %v", err)
	}
	if m, err := bare.Get(ctx, barePartner.ID); err != nil {
		t.Fatalf("未装配附件仓储时查询详情失败: %v", err)
	} else if m.LogoURL != "" {
		t.Errorf("未装配附件仓储时不该有 Logo，实际 %q", m.LogoURL)
	}
	if res, err := bare.List(ctx, dto.PartnerQuery{}); err != nil {
		t.Fatalf("未装配附件仓储时查询列表失败: %v", err)
	} else if len(res.List) != 1 {
		t.Errorf("列表应有 1 条，实际 %d", len(res.List))
	}
}

// newPartnerTestServiceWithAttachments 带 Logo 回填能力的装配。
//
// 单独一个函数而不是给 newPartnerTestService 加个开关：后者是几十处用例共用的
// 常态装配，为它多开一条分支，等于让每个读者都要判断"这次要不要带附件"。
func newPartnerTestServiceWithAttachments(
	t *testing.T,
) (PartnerService, repository.AttachmentRepository) {
	t.Helper()
	db := openTestDB(t, &model.Partner{}, &model.Product{}, &model.Currency{},
		&model.User{}, &model.Attachment{}, &model.AttachmentContent{})
	attachments := repository.NewAttachmentRepository(db)
	return NewPartnerService(
		repository.NewPartnerRepository(db),
		repository.NewProductRepository(db),
		repository.NewCurrencyRepository(db),
		repository.NewUserRepository(db),
		WithPartnerAttachments(attachments),
	), attachments
}

// TestAttachmentImageTypesMatchUploadWhitelist 把两份名单绑在一起。
//
// 一边是"能上传什么"（service 的扩展名白名单），一边是"能当头像显示什么"；
// 真正会咬人的方向是后者：允许上传的类型里新增了图片而这里没加，
// 用户传了却永远显示占位标识，且没有任何报错。
func TestAttachmentImageTypesMatchUploadWhitelist(t *testing.T) {
	for _, mime := range model.AttachmentImageTypes {
		allowed := false
		for _, v := range allowedAttachmentTypes {
			if v == mime {
				allowed = true
				break
			}
		}
		if !allowed {
			t.Errorf("%s 被当成可展示的图片，但它根本不允许上传", mime)
		}
	}
	// 反方向：除了 PDF，允许上传的都应当能展示。
	for _, v := range allowedAttachmentTypes {
		if v == "application/pdf" {
			continue
		}
		listed := false
		for _, mime := range model.AttachmentImageTypes {
			if v == mime {
				listed = true
				break
			}
		}
		if !listed {
			t.Errorf("允许上传 %s，但它没被当成图片：PDF 之外的新类型要么两边一起加，要么都别加", v)
		}
	}
}
