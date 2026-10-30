package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	_ "image/png"
	"log/slog"
	"math"
	"net/http"
	"path/filepath"
	"strings"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/errs"
	"github.com/edwinhuish/gowork/internal/repository"
)

const (
	// MaxProductImageSize 是单张图片的上限，5MB。
	//
	// 手机直出的照片常在 2–4MB，再大就该先压；而内容存库、列表还要取缩略图，
	// 放宽到 10MB 只会让备份与首次加载一起变慢。
	MaxProductImageSize int64 = 5 << 20

	// MaxProductImages 是一个产品最多保留的图片数。
	// 十二张足够覆盖正反面、细节与包装；不设上限会让「主图」以外的顺序失去意义。
	MaxProductImages = 12

	// MaxProductImagesPerUpload 是单次请求最多收几张。
	// 4 × 5MB 低于 router.bodyLimitBytes（32MB），批量上传时前端自动分批。
	MaxProductImagesPerUpload = 4

	productImageThumbSide = 400
	// maxProductImageDimension / Pixels 挡解压炸弹：一张 2 万像素见方的 PNG
	// 文件可能只有几百 KB，解码后却要几 GB 内存。DecodeConfig 只读文件头，成本极低。
	maxProductImageDimension = 8000
	maxProductImagePixels    = 24 << 20
)

// allowedProductImageTypes 是唯一接受的图片类型：扩展名 → 期望的嗅探结果。
//
// 不含 SVG：它可内嵌脚本，本系统把图片直接内联展示，等于开一个存储型 XSS 的入口。
// 不含 GIF / AVIF：业务上没有需要，而 AVIF 没有标准库解码器。
var allowedProductImageTypes = map[string]string{
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".png":  "image/png",
	".webp": "image/webp",
}

// ProductImageService 管一个产品下的图片：顺序、主图与取图。
//
// 主图没有独立开关：**列表里的第一张就是主图**。「设为主图」在前端等价于
// 把那张移到第一位，服务端只认顺序，不存在「标记与位置打架」的状态。
type ProductImageService interface {
	List(ctx context.Context, productID uint) ([]model.ProductImage, error)
	Upload(ctx context.Context, productID uint, files []dto.ProductImageFile) ([]model.ProductImage, error)
	Replace(ctx context.Context, productID, imageID uint, fileName string, content []byte) (*model.ProductImage, error)
	Delete(ctx context.Context, productID, imageID uint) error
	// Reorder 接收完整排列；集合与库中不一致返回 409。
	Reorder(ctx context.Context, productID uint, ids []uint) ([]model.ProductImage, error)
	// Content 返回元数据与字节。thumb=true 取缩略图。
	Content(ctx context.Context, productID, imageID uint, thumb bool) (*model.ProductImage, []byte, error)
}

type productImageService struct {
	repo repository.ProductImageRepository
	// products 只用来确认产品存在：图片是产品的子表，挂到不存在的产品上比拒绝更难查。
	products repository.ProductRepository
	txs      repository.TxManager
	log      *slog.Logger
}

type ProductImageOption func(*productImageService)

func WithProductImageLogger(l *slog.Logger) ProductImageOption {
	return func(s *productImageService) { s.log = l }
}

func NewProductImageService(
	repo repository.ProductImageRepository,
	products repository.ProductRepository,
	txs repository.TxManager,
	opts ...ProductImageOption,
) ProductImageService {
	s := &productImageService{repo: repo, products: products, txs: txs, log: slog.Default()}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *productImageService) List(
	ctx context.Context, productID uint,
) ([]model.ProductImage, error) {
	return s.list(ctx, productID)
}

func (s *productImageService) Upload(
	ctx context.Context, productID uint, files []dto.ProductImageFile,
) ([]model.ProductImage, error) {
	if len(files) == 0 {
		return nil, errs.BadRequest("请选择要上传的图片")
	}
	if len(files) > MaxProductImagesPerUpload {
		return nil, errs.BadRequest(fmt.Sprintf(
			"单次最多上传 %d 张，请分批提交", MaxProductImagesPerUpload))
	}
	if err := s.ensureProduct(ctx, productID); err != nil {
		return nil, err
	}

	// 先按现有数量粗判一次：已经满了就不必解码 4 张大图再告诉用户放不下。
	existing, err := s.repo.CountByProduct(ctx, productID)
	if err != nil {
		return nil, err
	}
	if int(existing)+len(files) > MaxProductImages {
		return nil, errs.BadRequest(fmt.Sprintf(
			"每个产品最多 %d 张图片，当前已有 %d 张，本批最多还能传 %d 张",
			MaxProductImages, existing, maxInt(MaxProductImages-int(existing), 0)))
	}

	// 全部校验并生成缩略图后再落库：中途一张不合格就整批失败，
	// 而不是让用户对着「传进去三张、不知道哪张失败了」猜。
	prepared := make([]preparedProductImage, 0, len(files))
	for _, f := range files {
		p, err := prepareProductImage(f.FileName, f.Content)
		if err != nil {
			return nil, err
		}
		prepared = append(prepared, *p)
	}

	// 内部调用（迁移、脚本）没有身份，上传人记 0，与附件表的约定一致。
	uploadedBy := uint(0)
	if actor, ok := ActorFrom(ctx); ok {
		uploadedBy = actor.UserID
	}

	err = s.txs.Transaction(ctx, func(ctx context.Context) error {
		// 事务内重新数一次：两个请求同时看到「还剩 1 张」时，只有先提交的那个能进。
		n, err := s.repo.CountByProduct(ctx, productID)
		if err != nil {
			return err
		}
		if int(n)+len(prepared) > MaxProductImages {
			return errs.ErrConflict.WithMsg(fmt.Sprintf(
				"每个产品最多 %d 张图片，当前已有 %d 张", MaxProductImages, n))
		}

		// 追加到末尾：已有图片的顺序不因这次上传而变，第一张仍是主图。
		start := 0
		if list, err := s.repo.ListByProduct(ctx, productID); err != nil {
			return err
		} else if len(list) > 0 {
			start = list[len(list)-1].SortOrder + 1
		}

		for i := range prepared {
			n := prepared[i]
			m := &model.ProductImage{
				ProductID:  productID,
				SortOrder:  start + i,
				FileName:   n.fileName,
				MimeType:   n.mimeType,
				Size:       n.size,
				Width:      n.width,
				Height:     n.height,
				SHA256:     n.sha256,
				UploadedBy: uploadedBy,
			}
			if err := s.repo.Create(ctx, m, n.content, n.thumb); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	s.log.Info("上传产品图片", "product_id", productID, "count", len(prepared))
	return s.list(ctx, productID)
}

func (s *productImageService) Replace(
	ctx context.Context, productID, imageID uint, fileName string, content []byte,
) (*model.ProductImage, error) {
	if _, err := s.ownedImage(ctx, productID, imageID); err != nil {
		return nil, err
	}
	prepared, err := prepareProductImage(fileName, content)
	if err != nil {
		return nil, err
	}

	// 不动 sort_order：替换是换一个外观，不是换一个位置；原本是主图就还是主图。
	fields := map[string]any{
		"file_name": prepared.fileName,
		"mime_type": prepared.mimeType,
		"size":      prepared.size,
		"width":     prepared.width,
		"height":    prepared.height,
		"sha256":    prepared.sha256,
	}
	if err := s.repo.Replace(ctx, imageID, fields, prepared.content, prepared.thumb); err != nil {
		return nil, err
	}
	s.log.Info("替换产品图片", "product_id", productID, "image_id", imageID)

	list, err := s.list(ctx, productID)
	if err != nil {
		return nil, err
	}
	for i := range list {
		if list[i].ID == imageID {
			return &list[i], nil
		}
	}
	return nil, errs.ErrNotFound
}

func (s *productImageService) Delete(
	ctx context.Context, productID, imageID uint,
) error {
	if _, err := s.ownedImage(ctx, productID, imageID); err != nil {
		return err
	}
	if err := s.txs.Transaction(ctx, func(ctx context.Context) error {
		return s.repo.Delete(ctx, imageID)
	}); err != nil {
		return err
	}
	s.log.Info("删除产品图片", "product_id", productID, "image_id", imageID)
	return nil
}

func (s *productImageService) Reorder(
	ctx context.Context, productID uint, ids []uint,
) ([]model.ProductImage, error) {
	if err := s.ensureProduct(ctx, productID); err != nil {
		return nil, err
	}
	current, err := s.repo.ListByProduct(ctx, productID)
	if err != nil {
		return nil, err
	}

	// 提交的必须是当前集合的一个排列：少了说明别人删了图，多了说明别人加了图，
	// 两种都意味着用户看到的不是最新一版，直接让他刷新比猜着合并更安全。
	if len(ids) != len(current) {
		return nil, errs.ErrConflict.WithMsg("图片列表已被他人改动，请刷新后重试")
	}
	seen := make(map[uint]struct{}, len(ids))
	for _, id := range ids {
		if _, dup := seen[id]; dup {
			return nil, errs.ErrConflict.WithMsg("图片列表已被他人改动，请刷新后重试")
		}
		seen[id] = struct{}{}
	}
	for _, img := range current {
		if _, ok := seen[img.ID]; !ok {
			return nil, errs.ErrConflict.WithMsg("图片列表已被他人改动，请刷新后重试")
		}
	}

	if err := s.txs.Transaction(ctx, func(ctx context.Context) error {
		return s.repo.Reorder(ctx, productID, ids)
	}); err != nil {
		return nil, err
	}
	return s.list(ctx, productID)
}

func (s *productImageService) Content(
	ctx context.Context, productID, imageID uint, thumb bool,
) (*model.ProductImage, []byte, error) {
	img, err := s.ownedImage(ctx, productID, imageID)
	if err != nil {
		return nil, nil, err
	}
	data, err := s.repo.FindContent(ctx, imageID, thumb)
	if err != nil {
		return nil, nil, err
	}
	return img, data, nil
}

func (s *productImageService) list(
	ctx context.Context, productID uint,
) ([]model.ProductImage, error) {
	list, err := s.repo.ListByProduct(ctx, productID)
	if err != nil {
		return nil, err
	}
	for i := range list {
		decorateProductImage(&list[i], i == 0)
	}
	return list, nil
}

func (s *productImageService) ensureProduct(ctx context.Context, productID uint) error {
	_, err := s.products.FindByID(ctx, productID)
	if errors.Is(err, errs.ErrNotFound) {
		return errs.ErrNotFound.WithMsg("产品不存在")
	}
	return err
}

// ownedImage 取图并确认它确实挂在这个产品下。
//
// 路径里同时带 product_id 与 image_id，不校验的话任何登录用户都能用
// 「自己的产品路径 + 别人的图片 id」读到别家的图。
func (s *productImageService) ownedImage(
	ctx context.Context, productID, imageID uint,
) (*model.ProductImage, error) {
	img, err := s.repo.FindByID(ctx, imageID)
	if err != nil {
		return nil, err
	}
	if img.ProductID != productID {
		return nil, errs.ErrNotFound.WithMsg("图片不存在")
	}
	return img, nil
}

// decorateProductImage 回填响应字段：是否主图、取图路径与缩略图路径。
//
// URL 带上 updated_at：替换内容后路径随之变化，前端按路径做的 blob 缓存
// 与浏览器的强缓存都不会继续拿旧图。
func decorateProductImage(m *model.ProductImage, primary bool) {
	m.IsPrimary = primary
	m.URL = fmt.Sprintf("/products/%d/images/%d/content?v=%d",
		m.ProductID, m.ID, m.UpdatedAt.UnixMilli())
	m.ThumbURL = m.URL + "&size=thumb"
}

// preparedProductImage 是校验并生成缩略图之后的图片，可以直接落库。
type preparedProductImage struct {
	fileName string
	mimeType string
	size     int64
	width    int
	height   int
	sha256   string
	content  []byte
	thumb    []byte
}

// prepareProductImage 做全部校验并生成缩略图。
//
// 扩展名与内容必须对得上：只看扩展名，改个名字就能把任意文件塞进来；
// 只看内容，一个 .png 里放 JPEG 会让下游按错误类型处理。
func prepareProductImage(rawName string, content []byte) (*preparedProductImage, error) {
	name, err := cleanFileName(rawName)
	if err != nil {
		return nil, err
	}
	if len(content) == 0 {
		return nil, errs.BadRequest("图片是空的")
	}
	if int64(len(content)) > MaxProductImageSize {
		return nil, errs.BadRequest(fmt.Sprintf(
			"%s 超过单张 %d MB 的上限", name, MaxProductImageSize>>20))
	}

	ext := strings.ToLower(filepath.Ext(name))
	expected, ok := allowedProductImageTypes[ext]
	if !ok {
		return nil, errs.BadRequest("只支持 JPG、PNG、WebP，收到的是 " + orUnknownExt(ext))
	}
	if detected := http.DetectContentType(content); detected != expected {
		return nil, errs.BadRequest(fmt.Sprintf(
			"%s 的内容不是 %s（识别为 %s）", name, expected, detected))
	}

	// DecodeConfig 只读文件头：既拿到宽高，也顺带确认「它真的能被解码」。
	cfg, _, err := image.DecodeConfig(bytes.NewReader(content))
	if err != nil {
		return nil, errs.BadRequest(name + " 不是有效的图片文件")
	}
	if cfg.Width <= 0 || cfg.Height <= 0 ||
		cfg.Width > maxProductImageDimension || cfg.Height > maxProductImageDimension {
		return nil, errs.BadRequest(fmt.Sprintf(
			"图片尺寸需在 1–%d 像素之间", maxProductImageDimension))
	}
	if cfg.Width*cfg.Height > maxProductImagePixels {
		return nil, errs.BadRequest(fmt.Sprintf(
			"图片像素过多（%d×%d），请先缩小到 %d 万像素以内",
			cfg.Width, cfg.Height, maxProductImagePixels/10000))
	}

	src, _, err := image.Decode(bytes.NewReader(content))
	if err != nil {
		return nil, errs.BadRequest(name + " 不是有效的图片文件")
	}
	thumb, err := makeProductImageThumb(src)
	if err != nil {
		return nil, err
	}

	sum := sha256.Sum256(content)
	return &preparedProductImage{
		fileName: name,
		mimeType: expected,
		size:     int64(len(content)),
		width:    cfg.Width,
		height:   cfg.Height,
		sha256:   hex.EncodeToString(sum[:]),
		content:  content,
		thumb:    thumb,
	}, nil
}

// makeProductImageThumb 生成最长边 400px 的 JPEG 缩略图。
//
// 统一转 JPEG 而不是保持原格式：WebP 只有解码器没有标准库编码器，
// 保留多种格式只会让「列表该按什么类型响应」多一种分支。
// 透明底先铺白：JPEG 没有 alpha，直接画会让透明区域变黑。
func makeProductImageThumb(src image.Image) ([]byte, error) {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w == 0 || h == 0 {
		return nil, errs.BadRequest("图片尺寸为 0")
	}
	scale := math.Min(1, float64(productImageThumbSide)/float64(maxInt(w, h)))
	tw := maxInt(int(math.Round(float64(w)*scale)), 1)
	th := maxInt(int(math.Round(float64(h)*scale)), 1)

	dst := image.NewRGBA(image.Rect(0, 0, tw, th))
	draw.Draw(dst, dst.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, b, draw.Over, nil)

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: 85}); err != nil {
		return nil, fmt.Errorf("生成缩略图: %w", err)
	}
	return buf.Bytes(), nil
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
