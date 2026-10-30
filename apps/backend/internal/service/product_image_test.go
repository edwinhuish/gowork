package service

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"testing"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/errs"
	"github.com/edwinhuish/gowork/internal/repository"
)

type productImageFixture struct {
	images     ProductImageService
	imageRepo  repository.ProductImageRepository
	productA   uint
	productB   uint
	productSvc ProductService
}

func newProductImageFixture(t *testing.T) productImageFixture {
	t.Helper()
	ctx := context.Background()

	db := openTestDB(t,
		&model.Partner{}, &model.Product{},
		&model.ProductImage{}, &model.ProductImageContent{})
	partnerRepo := repository.NewPartnerRepository(db)
	productRepo := repository.NewProductRepository(db)
	sourceRepo := repository.NewProductSourceRepository(db)
	codeRepo := repository.NewProductCodeRepository(db)
	imageRepo := repository.NewProductImageRepository(db)
	txs := repository.NewTxManager(db)

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

	return productImageFixture{
		images:    NewProductImageService(imageRepo, productRepo, txs),
		imageRepo: imageRepo,
		productSvc: NewProductService(productRepo, partnerRepo, sourceRepo, codeRepo, txs,
			WithProductImages(imageRepo)),
		productA: a.ID,
		productB: b.ID,
	}
}

// hasCode 判业务错误码。errs 的哨兵变量是指针，WithMsg 返回副本，
// errors.Is 比到的是指针而不是码，测试里统一按 Code 判。
func hasCode(err error, code errs.Code) bool {
	var e *errs.Error
	return errors.As(err, &e) && e.Code == code
}

func testPNG(t *testing.T, w, h int, c color.Color) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(img, img.Bounds(), image.NewUniform(c), image.Point{}, draw.Src)
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("生成测试 PNG: %v", err)
	}
	return buf.Bytes()
}

func testJPEG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(img, img.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatalf("生成测试 JPEG: %v", err)
	}
	return buf.Bytes()
}

func uploadOne(
	t *testing.T, svc ProductImageService, productID uint, name string, content []byte,
) {
	t.Helper()
	if _, err := svc.Upload(context.Background(), productID,
		[]dto.ProductImageFile{{FileName: name, Content: content}}); err != nil {
		t.Fatalf("上传 %s 失败: %v", name, err)
	}
}

func TestProductImageUploadMarksFirstAsPrimary(t *testing.T) {
	f := newProductImageFixture(t)
	ctx := context.Background()

	list, err := f.images.Upload(ctx, f.productA, []dto.ProductImageFile{
		{FileName: "正面.png", Content: testPNG(t, 40, 30, color.RGBA{R: 200, A: 255})},
		{FileName: "反面.png", Content: testPNG(t, 40, 30, color.RGBA{B: 200, A: 255})},
	})
	if err != nil {
		t.Fatalf("上传失败: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("应有 2 张，实际 %d", len(list))
	}
	if !list[0].IsPrimary || list[1].IsPrimary {
		t.Fatalf("第一张应为主图，实际 %v / %v", list[0].IsPrimary, list[1].IsPrimary)
	}
	if list[0].SortOrder != 0 || list[1].SortOrder != 1 {
		t.Fatalf("顺序应为 0、1，实际 %d、%d", list[0].SortOrder, list[1].SortOrder)
	}
	// URL 带版本号：替换内容后路径要变，前端缓存才不会继续用旧图。
	if list[0].ThumbURL == "" || list[0].URL == "" {
		t.Fatalf("取图路径未回填: %+v", list[0])
	}
	if !bytes.Contains([]byte(list[0].ThumbURL), []byte("size=thumb")) {
		t.Fatalf("缩略图路径应带 size=thumb，实际 %s", list[0].ThumbURL)
	}

	// 追加不改变已有顺序：主图仍是第一张。
	uploadOne(t, f.images, f.productA, "包装.png", testPNG(t, 10, 10, color.White))
	list, err = f.images.List(ctx, f.productA)
	if err != nil {
		t.Fatalf("查询图片失败: %v", err)
	}
	if len(list) != 3 || !list[0].IsPrimary {
		t.Fatalf("追加后应仍是第一张主图，实际 %+v", list)
	}
}

func TestProductImageUploadRejectsBadFiles(t *testing.T) {
	f := newProductImageFixture(t)
	ctx := context.Background()

	cases := []struct {
		name    string
		file    string
		content []byte
	}{
		{name: "扩展名伪装", file: "假的.png", content: testJPEG(t, 8, 8)},
		{name: "不是图片", file: "说明.png", content: []byte("这不是图片")},
		{name: "不支持的格式", file: "图.gif", content: testPNG(t, 8, 8, color.White)},
		{name: "空文件", file: "空.png", content: nil},
		{name: "超过大小上限", file: "大.png", content: bytes.Repeat([]byte{0}, int(MaxProductImageSize)+1)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := f.images.Upload(ctx, f.productA,
				[]dto.ProductImageFile{{FileName: tc.file, Content: tc.content}})
			if err == nil {
				t.Fatal("期望被拒绝，实际通过了")
			}
			if !hasCode(err, errs.CodeInvalidParam) {
				t.Fatalf("期望参数错误，实际 %v", err)
			}
		})
	}

	// 整批要么全进、要么全不进：第二张不合格时第一张也不该落库。
	_, err := f.images.Upload(ctx, f.productA, []dto.ProductImageFile{
		{FileName: "好.png", Content: testPNG(t, 8, 8, color.White)},
		{FileName: "坏.png", Content: []byte("坏文件")},
	})
	if err == nil {
		t.Fatal("整批应被拒绝")
	}
	list, err := f.images.List(ctx, f.productA)
	if err != nil {
		t.Fatalf("查询图片失败: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("整批失败不该落库，实际 %d 张", len(list))
	}
}

func TestProductImageQuota(t *testing.T) {
	f := newProductImageFixture(t)
	ctx := context.Background()

	batch := func(n int) []dto.ProductImageFile {
		files := make([]dto.ProductImageFile, 0, n)
		for i := 0; i < n; i++ {
			files = append(files, dto.ProductImageFile{
				FileName: "图.png",
				Content:  testPNG(t, 8, 8, color.White),
			})
		}
		return files
	}

	for i := 0; i < MaxProductImages/MaxProductImagesPerUpload; i++ {
		if _, err := f.images.Upload(ctx, f.productA, batch(MaxProductImagesPerUpload)); err != nil {
			t.Fatalf("第 %d 批失败: %v", i+1, err)
		}
	}
	// 单次超过批量上限也要在解析前拦掉。
	if _, err := f.images.Upload(ctx, f.productA,
		batch(MaxProductImagesPerUpload+1)); err == nil {
		t.Fatal("单批超限应被拒绝")
	}
	// 配额满了之后再来一张：400，且提示里说得清还剩几张。
	_, err := f.images.Upload(ctx, f.productA, batch(1))
	if err == nil {
		t.Fatal("超过配额应被拒绝")
	}
	var e *errs.Error
	if !errors.As(err, &e) || e.Code != errs.CodeInvalidParam {
		t.Fatalf("期望参数错误，实际 %v", err)
	}
	if !bytes.Contains([]byte(e.Msg), []byte("最多")) {
		t.Fatalf("提示应说明上限，实际 %q", e.Msg)
	}

	list, err := f.images.List(ctx, f.productA)
	if err != nil {
		t.Fatalf("查询图片失败: %v", err)
	}
	if len(list) != MaxProductImages {
		t.Fatalf("应停在 %d 张，实际 %d", MaxProductImages, len(list))
	}
}

func TestProductImageReorderChangesPrimary(t *testing.T) {
	f := newProductImageFixture(t)
	ctx := context.Background()

	list, err := f.images.Upload(ctx, f.productA, []dto.ProductImageFile{
		{FileName: "1.png", Content: testPNG(t, 8, 8, color.White)},
		{FileName: "2.png", Content: testPNG(t, 8, 8, color.White)},
		{FileName: "3.png", Content: testPNG(t, 8, 8, color.White)},
	})
	if err != nil {
		t.Fatalf("上传失败: %v", err)
	}

	reversed := []uint{list[2].ID, list[1].ID, list[0].ID}
	got, err := f.images.Reorder(ctx, f.productA, reversed)
	if err != nil {
		t.Fatalf("调整顺序失败: %v", err)
	}
	for i, want := range reversed {
		if got[i].ID != want {
			t.Fatalf("第 %d 位应为 %d，实际 %d", i, want, got[i].ID)
		}
		if got[i].SortOrder != i {
			t.Fatalf("第 %d 位 sort_order 应为 %d，实际 %d", i, i, got[i].SortOrder)
		}
		if got[i].IsPrimary != (i == 0) {
			t.Fatalf("第 %d 位的 is_primary 不对: %v", i, got[i].IsPrimary)
		}
	}

	// 少一个 id：说明用户看到的不是最新列表，409 而不是猜着合并。
	if _, err := f.images.Reorder(ctx, f.productA, reversed[:2]); !hasCode(err, errs.CodeConflict) {
		t.Fatalf("集合不一致应返回 409，实际 %v", err)
	}
	// 重复 id 同样不是合法排列。
	dup := []uint{reversed[0], reversed[0], reversed[1]}
	if _, err := f.images.Reorder(ctx, f.productA, dup); !hasCode(err, errs.CodeConflict) {
		t.Fatalf("重复 id 应返回 409，实际 %v", err)
	}

	// 删掉主图，下一张自动接任——没有单独的「提升主图」请求。
	if err := f.images.Delete(ctx, f.productA, reversed[0]); err != nil {
		t.Fatalf("删除主图失败: %v", err)
	}
	after, err := f.images.List(ctx, f.productA)
	if err != nil {
		t.Fatalf("查询图片失败: %v", err)
	}
	if len(after) != 2 || after[0].ID != reversed[1] || !after[0].IsPrimary {
		t.Fatalf("删除后应由下一张接任主图，实际 %+v", after)
	}

	// 产品下的图片不能从另一个产品的路径操作。
	if err := f.images.Delete(ctx, f.productB, after[0].ID); !hasCode(err, errs.CodeNotFound) {
		t.Fatalf("跨产品操作应返回 404，实际 %v", err)
	}
}

func TestProductImageReplaceKeepsPosition(t *testing.T) {
	f := newProductImageFixture(t)
	ctx := context.Background()

	list, err := f.images.Upload(ctx, f.productA, []dto.ProductImageFile{
		{FileName: "主图.png", Content: testPNG(t, 40, 30, color.RGBA{R: 200, A: 255})},
		{FileName: "次图.png", Content: testPNG(t, 40, 30, color.RGBA{B: 200, A: 255})},
	})
	if err != nil {
		t.Fatalf("上传失败: %v", err)
	}
	oldURL := list[1].URL
	oldThumbURL := list[1].ThumbURL

	replaced, err := f.images.Replace(ctx, f.productA, list[1].ID,
		"新次图.jpg", testJPEG(t, 20, 10))
	if err != nil {
		t.Fatalf("替换失败: %v", err)
	}
	if replaced.ID != list[1].ID || replaced.SortOrder != 1 || replaced.IsPrimary {
		t.Fatalf("替换应保留位置与主图身份，实际 %+v", replaced)
	}
	if replaced.Width != 20 || replaced.Height != 10 {
		t.Fatalf("尺寸应更新为 20×10，实际 %d×%d", replaced.Width, replaced.Height)
	}
	if replaced.MimeType != "image/jpeg" || replaced.FileName != "新次图.jpg" {
		t.Fatalf("类型或文件名未更新: %+v", replaced)
	}
	if replaced.URL == oldURL || replaced.ThumbURL == oldThumbURL {
		t.Fatalf("替换后 URL 应带新的版本号，仍是 %s", replaced.URL)
	}

	// 替换主图不改变它是主图。
	primary, err := f.images.Replace(ctx, f.productA, list[0].ID,
		"新主图.png", testPNG(t, 12, 12, color.White))
	if err != nil {
		t.Fatalf("替换主图失败: %v", err)
	}
	if !primary.IsPrimary || primary.SortOrder != 0 {
		t.Fatalf("替换后仍应为主图，实际 %+v", primary)
	}
}

// 产品列表与详情带出主图：列表页的缩略图靠这个字段，前端不必为每行再请求一次。
func TestProductListCarriesPrimaryImage(t *testing.T) {
	f := newProductImageFixture(t)
	ctx := context.Background()

	list, err := f.images.Upload(ctx, f.productA, []dto.ProductImageFile{
		{FileName: "正面.png", Content: testPNG(t, 8, 8, color.White)},
		{FileName: "背面.png", Content: testPNG(t, 8, 8, color.White)},
	})
	if err != nil {
		t.Fatalf("上传失败: %v", err)
	}

	page, err := f.productSvc.List(ctx, dto.ProductQuery{})
	if err != nil {
		t.Fatalf("查询产品列表失败: %v", err)
	}
	var found *model.Product
	for i := range page.List {
		if page.List[i].ID == f.productA {
			found = &page.List[i]
		}
	}
	if found == nil {
		t.Fatal("列表里应有这个产品")
	}
	if found.PrimaryImage == nil || found.PrimaryImage.ID != list[0].ID ||
		!found.PrimaryImage.IsPrimary || found.PrimaryImage.ThumbURL == "" {
		t.Fatalf("列表应带第一张为主图，实际 %+v", found.PrimaryImage)
	}

	// 详情同口径。
	detail, err := f.productSvc.Get(ctx, f.productA)
	if err != nil {
		t.Fatalf("查询产品详情失败: %v", err)
	}
	if detail.PrimaryImage == nil || detail.PrimaryImage.ID != list[0].ID {
		t.Fatalf("详情应带主图，实际 %+v", detail.PrimaryImage)
	}

	// 换了主图之后，列表与详情必须跟着换——它们共用一个「顺序第一张」的判定。
	if _, err := f.images.Reorder(ctx, f.productA,
		[]uint{list[1].ID, list[0].ID}); err != nil {
		t.Fatalf("调整顺序失败: %v", err)
	}
	detail, err = f.productSvc.Get(ctx, f.productA)
	if err != nil {
		t.Fatalf("查询产品详情失败: %v", err)
	}
	if detail.PrimaryImage == nil || detail.PrimaryImage.ID != list[1].ID {
		t.Fatalf("主图应换成第二张，实际 %+v", detail.PrimaryImage)
	}

	// 没有图片的产品不该带空主图对象。
	empty, err := f.productSvc.Get(ctx, f.productB)
	if err != nil {
		t.Fatalf("查询无图产品失败: %v", err)
	}
	if empty.PrimaryImage != nil {
		t.Fatalf("无图产品不该有主图，实际 %+v", empty.PrimaryImage)
	}
}

func TestProductImageContentServesThumbAndOriginal(t *testing.T) {
	f := newProductImageFixture(t)
	ctx := context.Background()

	original := testPNG(t, 800, 600, color.RGBA{G: 200, A: 255})
	list, err := f.images.Upload(ctx, f.productA,
		[]dto.ProductImageFile{{FileName: "大图.png", Content: original}})
	if err != nil {
		t.Fatalf("上传失败: %v", err)
	}
	id := list[0].ID

	_, full, err := f.images.Content(ctx, f.productA, id, false)
	if err != nil {
		t.Fatalf("取原图失败: %v", err)
	}
	if !bytes.Equal(full, original) {
		t.Fatal("原图应与上传的字节一致")
	}

	_, thumb, err := f.images.Content(ctx, f.productA, id, true)
	if err != nil {
		t.Fatalf("取缩略图失败: %v", err)
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(thumb))
	if err != nil {
		t.Fatalf("缩略图应可解码: %v", err)
	}
	if format != "jpeg" {
		t.Fatalf("缩略图统一转 JPEG，实际 %s", format)
	}
	if cfg.Width != productImageThumbSide || cfg.Height != 300 {
		t.Fatalf("缩略图应等比缩到 400×300，实际 %d×%d", cfg.Width, cfg.Height)
	}

	// 跨产品取图是 404，不是「读到了但不该读」。
	if _, _, err := f.images.Content(ctx, f.productB, id, true); !hasCode(err, errs.CodeNotFound) {
		t.Fatalf("跨产品取图应返回 404，实际 %v", err)
	}
}
