package bootstrap

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	stddraw "image/draw"
	"image/png"
	"math"

	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"

	"github.com/edwinhuish/gowork/internal/dto"
)

// 演示用的产品图是**画出来的示意图**，不是照片。
//
// 选画不选"随便找几张图塞进来"，是因为演示数据要能重建：仓库里没有素材目录，
// 而把真图提交进仓库既占体积又有版权问题。画出来的图有个额外好处 ——
// 每张都带 SKU，卡片视图里一眼能对上是哪件货，比一张对不上号的库存照更有用。
//
// 只在空库灌种子时跑一次，16 张图的开销可以忽略。

// demoProductImageSide 是出图边长。与上传服务生成的 400px 缩略图配合：
// 卡片视图显示的是缩略图，点开预览才是这张原图。
const demoProductImageSide = 800

// categoryColors 是各类别的底色（上浅下深）。
//
// 同类别共用一个色系，是为了让卡片视图一眼按类别分组：五金偏蓝钢、
// 锁具偏炭灰、紧固件偏铜黄…… 换类别比换文字标签更快。
var categoryColors = map[string][2]color.RGBA{
	"BH": {{R: 0x9C, G: 0xBE, B: 0xE2, A: 0xFF}, {R: 0x3F, G: 0x63, B: 0x94, A: 0xFF}},
	"LK": {{R: 0xB8, G: 0xB8, B: 0xC2, A: 0xFF}, {R: 0x44, G: 0x46, B: 0x52, A: 0xFF}},
	"FS": {{R: 0xEC, G: 0xC9, B: 0x8A, A: 0xFF}, {R: 0xA9, G: 0x77, B: 0x33, A: 0xFF}},
	"HT": {{R: 0xF2, G: 0xBB, B: 0x8C, A: 0xFF}, {R: 0xA8, G: 0x5C, B: 0x1E, A: 0xFF}},
	"PT": {{R: 0xA6, G: 0xC2, B: 0xD2, A: 0xFF}, {R: 0x36, G: 0x5F, B: 0x78, A: 0xFF}},
	"PL": {{R: 0x9F, G: 0xCF, B: 0xC7, A: 0xFF}, {R: 0x37, G: 0x74, B: 0x6C, A: 0xFF}},
	"MS": {{R: 0xC5, G: 0xB2, B: 0xDD, A: 0xFF}, {R: 0x5F, G: 0x4A, B: 0x86, A: 0xFF}},
	"PK": {{R: 0xE0, G: 0xCD, B: 0xB2, A: 0xFF}, {R: 0x8C, G: 0x6F, B: 0x4A, A: 0xFF}},
}

var (
	demoImageFallback = [2]color.RGBA{
		{R: 0xC9, G: 0xCF, B: 0xD8, A: 0xFF},
		{R: 0x5B, G: 0x64, B: 0x74, A: 0xFF},
	}
	// 线稿色统一用深石板灰：底色换来换去，图形本身保持同一个"形状语言"。
	demoGlyphInk = color.RGBA{R: 0x22, G: 0x28, B: 0x33, A: 0xFF}
)

// seedProductImages 给每个演示产品生成一张主图。
//
// 走 ProductImageService.Upload 而不是直接往表里插：缩略图、宽高、SHA256
// 都由服务在上传路径里算好，自己插会漏掉缩略图，列表页就只能拿到原图。
func (a *App) seedProductImages(ctx context.Context, d seedDeps, master *demoMaster) error {
	for sku, product := range master.products {
		content, err := demoProductImage(sku, product.Category)
		if err != nil {
			return fmt.Errorf("生成演示产品图（%s）: %w", sku, err)
		}
		if _, err := d.ProductImages.Upload(ctx, product.ID, []dto.ProductImageFile{{
			FileName: sku + ".png",
			Content:  content,
		}}); err != nil {
			return fmt.Errorf("上传演示产品图（%s）: %w", sku, err)
		}
	}
	return nil
}

// demoProductImage 画一张 800×800 的示意图并编码成 PNG。
//
// 先在 2 倍画布上画再一次性降采样：这套图形全是硬边（圆、多边形、线段），
// 不降采样的话缩略图上边缘会有明显锯齿，卡片视图里很扎眼。
func demoProductImage(sku, category string) ([]byte, error) {
	const super = 2
	side := demoProductImageSide * super
	canvas := image.NewRGBA(image.Rect(0, 0, side, side))

	pair, ok := categoryColors[category]
	if !ok {
		pair = demoImageFallback
	}
	top, bottom := pair[0], pair[1]
	verticalGradient(canvas, top, bottom)

	// 一块半透明白板托住图形：直接画在渐变上，深色底会吃掉线稿的对比度。
	cx, cy := side/2, int(float64(side)*0.45)
	panel := int(float64(side) * 0.30)
	fillRoundRect(canvas, cx-panel, cy-panel, cx+panel, cy+panel, panel/6, color.RGBA{R: 255, G: 255, B: 255, A: 46})

	drawCategoryGlyph(canvas, category, cx, cy, int(float64(side)*0.20))
	drawSkuText(canvas, sku, side)

	out := image.NewRGBA(image.Rect(0, 0, demoProductImageSide, demoProductImageSide))
	xdraw.CatmullRom.Scale(out, out.Bounds(), canvas, canvas.Bounds(), stddraw.Over, nil)

	var buf bytes.Buffer
	if err := png.Encode(&buf, out); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// drawCategoryGlyph 按类别画一组线稿。缺省画菱形，保证未知类别也有图。
func drawCategoryGlyph(dst *image.RGBA, category string, cx, cy, s int) {
	t := s * 2 / 5 // 线宽
	switch category {
	case "BH": // 建筑五金：两片合页板 + 三颗螺丝孔
		fillRoundRect(dst, cx-s/2-t, cy-s, cx-s/2+t, cy+s, t/2, demoGlyphInk)
		fillRoundRect(dst, cx+s/2-t, cy-s, cx+s/2+t, cy+s, t/2, demoGlyphInk)
		// 孔要用浅色挖出来：跟板同色画上去等于没画，远看就是两块黑板。
		for i := -1; i <= 1; i++ {
			fillCircle(dst, cx-s/2, cy+i*s/2, t*2/5, color.RGBA{R: 255, G: 255, B: 255, A: 225})
			fillCircle(dst, cx+s/2, cy+i*s/2, t*2/5, color.RGBA{R: 255, G: 255, B: 255, A: 225})
		}
	case "LK": // 锁具：拱形锁环 + 锁体 + 锁孔
		// 锁环只画上半圈，再往下补两小段接到锁体上：
		// 画整圈会跟锁体叠成一个"8"，远看像数字不像锁。
		bodyTop := cy - s/10
		fillArc(dst, cx, bodyTop, s/2, t*4/5, math.Pi, 2*math.Pi, demoGlyphInk)
		fillSegment(dst, float64(cx-s/2), float64(bodyTop-t*2/5), float64(cx-s/2), float64(bodyTop+t*3/5), t*4/5, demoGlyphInk)
		fillSegment(dst, float64(cx+s/2), float64(bodyTop-t*2/5), float64(cx+s/2), float64(bodyTop+t*3/5), t*4/5, demoGlyphInk)
		fillRoundRect(dst, cx-s*5/8, bodyTop, cx+s*5/8, cy+s*4/5, t*6/5, demoGlyphInk)
		fillCircle(dst, cx, bodyTop+s/2, t*3/5, color.RGBA{R: 255, G: 255, B: 255, A: 225})
	case "FS": // 紧固件：六角头 + 螺纹
		fillPolygon(dst, hexagon(cx, cy-s/3, s*3/4), demoGlyphInk)
		fillRect(dst, cx-t/2, cy, cx+t/2, cy+s, demoGlyphInk)
		for i := 0; i < 3; i++ {
			y := cy + s/3 + i*t*3/5
			fillSegment(dst, float64(cx-t), float64(y), float64(cx+t), float64(y), t/4, demoGlyphInk)
		}
	case "HT": // 手动工具：扳手 —— 柄要停在环外，画进环里就把开口填死了
		fillSegment(dst, float64(cx-s*3/5), float64(cy+s*4/5), float64(cx+s/4), float64(cy-s/4), t*3/2, demoGlyphInk)
		fillRing(dst, cx+s/2, cy-s/2, s*3/5, s*3/5-t, demoGlyphInk)
	case "PT": // 电动工具：机身 + 钻头 + 手柄
		fillRoundRect(dst, cx-s, cy-s/2, cx+s/2, cy+s/4, t, demoGlyphInk)
		fillSegment(dst, float64(cx+s/2), float64(cy-s/8), float64(cx+s*4/5), float64(cy-s/8), t, demoGlyphInk)
		fillSegment(dst, float64(cx-s*3/4), float64(cy+s/4), float64(cx-s/2), float64(cy+s*3/4), t*3/2, demoGlyphInk)
	case "PL": // 水暖：出水弯管 + 阀体
		fillArc(dst, cx, cy, s*3/4, t, math.Pi, math.Pi*3/2, demoGlyphInk)
		fillSegment(dst, float64(cx), float64(cy-s*3/4), float64(cx), float64(cy+s/2), t, demoGlyphInk)
		fillRoundRect(dst, cx-t*3/2, cy+s/2-t/2, cx+t*3/2, cy+s/2+t/2, t/2, demoGlyphInk)
	case "MS": // 测量工具：尺身 + 刻度
		fillRoundRect(dst, cx-s, cy-t, cx+s, cy+t, t/3, demoGlyphInk)
		for i := -2; i <= 2; i++ {
			x := cx + i*s/3
			h := t * 2 / 3
			if i%2 == 0 {
				h = t
			}
			fillRect(dst, x-t/8, cy-t, x+t/8, cy-t+h, color.RGBA{R: 255, G: 255, B: 255, A: 220})
		}
	case "PK": // 包装：纸箱轮廓 + 折线
		fillPolygon(dst, []image.Point{
			{cx - s, cy - s/3}, {cx, cy - s*3/4}, {cx + s, cy - s/3},
			{cx + s, cy + s*3/4}, {cx - s, cy + s*3/4},
		}, demoGlyphInk)
		fillSegment(dst, float64(cx), float64(cy-s*3/4), float64(cx), float64(cy+s*3/4), t/2,
			color.RGBA{R: 255, G: 255, B: 255, A: 190})
		fillSegment(dst, float64(cx-2), float64(cy-s/3), float64(cx-2), float64(cy+s*3/4), t/2,
			color.RGBA{R: 255, G: 255, B: 255, A: 190})
	default:
		fillPolygon(dst, []image.Point{
			{cx, cy - s}, {cx + s, cy}, {cx, cy + s}, {cx - s, cy},
		}, demoGlyphInk)
		fillRing(dst, cx, cy, s*3/5, s*3/5-t, color.RGBA{R: 255, G: 255, B: 255, A: 200})
	}
}

// drawSkuText 把 SKU 用 4 倍像素字画在底部。
//
// 只用 ASCII：basicfont 里没有中文字形，画中文会变成一排空白。
// SKU 本来就是 ASCII，而且它比中文品名更适合当图上的标识 —— 能直接拿去搜。
func drawSkuText(dst *image.RGBA, sku string, side int) {
	const scale = 4
	face := basicfont.Face7x13
	width := face.Advance * len(sku)
	height := face.Height

	stamp := image.NewRGBA(image.Rect(0, 0, width+2, height+2))
	drawer := font.Drawer{
		Dst:  stamp,
		Src:  image.NewUniform(color.RGBA{R: 255, G: 255, B: 255, A: 235}),
		Face: face,
		Dot:  fixed.P(1, face.Ascent+1),
	}
	drawer.DrawString(sku)

	x0 := side/2 - width*scale/2
	y0 := int(float64(side)*0.84) - height*scale/2
	for y := 0; y < stamp.Bounds().Dy(); y++ {
		for x := 0; x < stamp.Bounds().Dx(); x++ {
			if _, _, _, a := stamp.At(x, y).RGBA(); a == 0 {
				continue
			}
			fillRect(dst, x0+x*scale, y0+y*scale, x0+(x+1)*scale-1, y0+(y+1)*scale-1,
				color.RGBA{R: 255, G: 255, B: 255, A: 235})
		}
	}
}

// ---- 绘图原语 ----

func verticalGradient(dst *image.RGBA, top, bottom color.RGBA) {
	b := dst.Bounds()
	h := float64(b.Dy() - 1)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		t := float64(y-b.Min.Y) / h
		c := color.RGBA{
			R: uint8(float64(top.R)*(1-t) + float64(bottom.R)*t),
			G: uint8(float64(top.G)*(1-t) + float64(bottom.G)*t),
			B: uint8(float64(top.B)*(1-t) + float64(bottom.B)*t),
			A: 0xFF,
		}
		for x := b.Min.X; x < b.Max.X; x++ {
			dst.SetRGBA(x, y, c)
		}
	}
}

// blendPixel 按 alpha 把颜色叠到已有点上；越界直接丢弃。
func blendPixel(dst *image.RGBA, x, y int, c color.RGBA) {
	if !(image.Point{X: x, Y: y}).In(dst.Bounds()) {
		return
	}
	if c.A == 0 {
		return
	}
	i := dst.PixOffset(x, y)
	if c.A == 0xFF {
		dst.Pix[i] = c.R
		dst.Pix[i+1] = c.G
		dst.Pix[i+2] = c.B
		dst.Pix[i+3] = 0xFF
		return
	}
	a := float64(c.A) / 255
	dst.Pix[i] = uint8(float64(c.R)*a + float64(dst.Pix[i])*(1-a))
	dst.Pix[i+1] = uint8(float64(c.G)*a + float64(dst.Pix[i+1])*(1-a))
	dst.Pix[i+2] = uint8(float64(c.B)*a + float64(dst.Pix[i+2])*(1-a))
	dst.Pix[i+3] = 0xFF
}

func fillRect(dst *image.RGBA, x0, y0, x1, y1 int, c color.RGBA) {
	for y := y0; y <= y1; y++ {
		for x := x0; x <= x1; x++ {
			blendPixel(dst, x, y, c)
		}
	}
}

func fillCircle(dst *image.RGBA, cx, cy, r int, c color.RGBA) {
	rr := float64(r * r)
	for y := cy - r; y <= cy+r; y++ {
		for x := cx - r; x <= cx+r; x++ {
			dx, dy := float64(x-cx), float64(y-cy)
			if dx*dx+dy*dy <= rr {
				blendPixel(dst, x, y, c)
			}
		}
	}
}

// fillRing 画圆环：外半径 outer、内半径 inner。
func fillRing(dst *image.RGBA, cx, cy, outer, inner int, c color.RGBA) {
	ro, ri := float64(outer*outer), float64(inner*inner)
	for y := cy - outer; y <= cy+outer; y++ {
		for x := cx - outer; x <= cx+outer; x++ {
			dx, dy := float64(x-cx), float64(y-cy)
			if d := dx*dx + dy*dy; d <= ro && d >= ri {
				blendPixel(dst, x, y, c)
			}
		}
	}
}

func fillRoundRect(dst *image.RGBA, x0, y0, x1, y1, r int, c color.RGBA) {
	if r < 1 {
		fillRect(dst, x0, y0, x1, y1, c)
		return
	}
	rr := float64(r * r)
	for y := y0; y <= y1; y++ {
		for x := x0; x <= x1; x++ {
			// 只对四个角做圆角判定，中间区域直接填。
			dx, dy := 0, 0
			if x < x0+r {
				dx = x0 + r - x
			} else if x > x1-r {
				dx = x - (x1 - r)
			}
			if y < y0+r {
				dy = y0 + r - y
			} else if y > y1-r {
				dy = y - (y1 - r)
			}
			if dx != 0 && dy != 0 && float64(dx*dx+dy*dy) > rr {
				continue
			}
			blendPixel(dst, x, y, c)
		}
	}
}

// fillSegment 画一条带圆头的线段，thickness 是整条线宽。
func fillSegment(dst *image.RGBA, x0, y0, x1, y1 float64, thickness int, c color.RGBA) {
	r := float64(thickness) / 2
	minX := int(math.Floor(math.Min(x0, x1) - r))
	maxX := int(math.Ceil(math.Max(x0, x1) + r))
	minY := int(math.Floor(math.Min(y0, y1) - r))
	maxY := int(math.Ceil(math.Max(y0, y1) + r))
	for y := minY; y <= maxY; y++ {
		for x := minX; x <= maxX; x++ {
			if pointSegmentDistance(float64(x), float64(y), x0, y0, x1, y1) <= r {
				blendPixel(dst, x, y, c)
			}
		}
	}
}

// fillArc 画圆弧：只保留 [from, to) 角度（弧度，屏幕坐标系顺时针）。
func fillArc(dst *image.RGBA, cx, cy, radius, thickness int, from, to float64, c color.RGBA) {
	outer := float64(radius) + float64(thickness)/2
	inner := float64(radius) - float64(thickness)/2
	for y := cy - int(outer) - 1; y <= cy+int(outer)+1; y++ {
		for x := cx - int(outer) - 1; x <= cx+int(outer)+1; x++ {
			dx, dy := float64(x-cx), float64(y-cy)
			d := math.Hypot(dx, dy)
			if d < inner || d > outer {
				continue
			}
			angle := math.Atan2(dy, dx)
			if angle < 0 {
				angle += 2 * math.Pi
			}
			if angle >= from && angle <= to {
				blendPixel(dst, x, y, c)
			}
		}
	}
}

// fillPolygon 用奇偶规则填充多边形。
func fillPolygon(dst *image.RGBA, points []image.Point, c color.RGBA) {
	if len(points) < 3 {
		return
	}
	minX, maxX := points[0].X, points[0].X
	minY, maxY := points[0].Y, points[0].Y
	for _, p := range points {
		minX, maxX = min(minX, p.X), max(maxX, p.X)
		minY, maxY = min(minY, p.Y), max(maxY, p.Y)
	}
	for y := minY; y <= maxY; y++ {
		for x := minX; x <= maxX; x++ {
			if pointInPolygon(float64(x)+0.5, float64(y)+0.5, points) {
				blendPixel(dst, x, y, c)
			}
		}
	}
}

func pointInPolygon(x, y float64, points []image.Point) bool {
	inside := false
	j := len(points) - 1
	for i := range points {
		xi, yi := float64(points[i].X), float64(points[i].Y)
		xj, yj := float64(points[j].X), float64(points[j].Y)
		if (yi > y) != (yj > y) && x < (xj-xi)*(y-yi)/(yj-yi)+xi {
			inside = !inside
		}
		j = i
	}
	return inside
}

func pointSegmentDistance(px, py, x0, y0, x1, y1 float64) float64 {
	dx, dy := x1-x0, y1-y0
	lengthSq := dx*dx + dy*dy
	if lengthSq == 0 {
		return math.Hypot(px-x0, py-y0)
	}
	t := ((px-x0)*dx + (py-y0)*dy) / lengthSq
	t = math.Max(0, math.Min(1, t))
	return math.Hypot(px-(x0+t*dx), py-(y0+t*dy))
}

func hexagon(cx, cy, r int) []image.Point {
	points := make([]image.Point, 0, 6)
	for i := 0; i < 6; i++ {
		angle := math.Pi/3*float64(i) - math.Pi/2
		points = append(points, image.Point{
			X: cx + int(float64(r)*math.Cos(angle)),
			Y: cy + int(float64(r)*math.Sin(angle)),
		})
	}
	return points
}
