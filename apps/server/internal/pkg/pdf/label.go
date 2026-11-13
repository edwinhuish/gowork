package pdf

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"

	"github.com/signintech/gopdf"
)

// LabelTemplate 是箱唛（shipping mark）的版式。
//
// 与单据模板分开：箱唛是"一张纸贴一个箱子"的等大排版，没有抬头、没有明细表、
// 也没有合计与跨页续行；它只有一列大字加几行留白，等人工用笔把箱号、毛净重填上。
// 把它硬塞进单据模板那套原语里，只会让两边都变形。
//
// JSON 里整块挂在 label 键下，解析时据此与单据模板区分。
type LabelTemplate struct {
	Title string `json:"title"`
	// Size 是每一张唛的边框尺寸（毫米）。
	Size Rect `json:"size"`
	// PerPage 是每张 A4 放几张唛。
	PerPage int `json:"per_page"`
	// Gap 是唛与唛之间的留白（毫米），中间画一条裁切线。
	Gap   float64     `json:"gap"`
	Lines []LabelLine `json:"lines"`
}

// Rect 是毫米为单位的宽高。
type Rect struct {
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

// LabelLine 是箱唛上的一行字，可含 {key} 占位。
//
// Muted 让"这张纸是箱唛"这类辅助说明变灰：它不该和订单号抢注意力。
type LabelLine struct {
	Text        string  `json:"text"`
	Size        float64 `json:"size"`
	Align       string  `json:"align"`
	SpaceBefore float64 `json:"space_before"`
	SpaceAfter  float64 `json:"space_after"`
	Muted       bool    `json:"muted"`
}

func parseLabel(name string, b []byte) (*LabelTemplate, error) {
	var wrap struct {
		Label LabelTemplate `json:"label"`
	}
	if err := json.Unmarshal(b, &wrap); err != nil {
		return nil, fmt.Errorf("解析箱唛模板 %s: %w", name, err)
	}
	lt := wrap.Label
	if lt.Size.Width <= 0 || lt.Size.Height <= 0 {
		return nil, fmt.Errorf("箱唛模板 %s 的 label.size 不合法", name)
	}
	if lt.PerPage < 1 {
		return nil, fmt.Errorf("箱唛模板 %s 的 per_page 至少为 1", name)
	}
	if float64(lt.PerPage)*lt.Size.Height > pageH-2*marginTop {
		return nil, fmt.Errorf("箱唛模板 %s 每页放不下 %d 张（单张高 %.0fmm）",
			name, lt.PerPage, lt.Size.Height)
	}
	if len(lt.Lines) == 0 {
		return nil, fmt.Errorf("箱唛模板 %s 的 label.lines 不能为空", name)
	}
	return &lt, nil
}

// RenderLabels 逐张渲染箱唛：一页放 per_page 张，张与张之间留裁切线。
//
// 不折行、不省略：唛上的字要么印全，要么就是模板写错了。超宽只可能是订单号变长，
// 那种情况应当调小字号，而不是让客户看到一个省略号。
func (r *Renderer) RenderLabels(name string, labels []map[string]string) ([]byte, error) {
	lt, err := r.set.GetLabel(name)
	if err != nil {
		return nil, err
	}
	if len(labels) == 0 {
		return nil, fmt.Errorf("没有要打印的箱唛")
	}

	p := &gopdf.GoPdf{}
	p.Start(gopdf.Config{PageSize: *gopdf.PageSizeA4, Unit: gopdf.UnitMM})
	p.SetMargins(marginX, marginTop, marginX, marginBottom)
	if err := p.AddTTFFontData(r.fontName, r.fontData); err != nil {
		return nil, fmt.Errorf("加载 PDF 字体: %w", err)
	}

	x := (pageW - lt.Size.Width) / 2
	for i, values := range labels {
		slot := i % lt.PerPage
		if slot == 0 {
			p.AddPage()
		}
		top := marginTop + float64(slot)*(lt.Size.Height+lt.Gap)
		drawLabel(p, lt, x, top, values)
		if slot < lt.PerPage-1 {
			cutLine(p, top+lt.Size.Height+lt.Gap/2)
		}
	}

	var buf bytes.Buffer
	if err := p.Write(&buf); err != nil {
		return nil, fmt.Errorf("生成箱唛 PDF: %w", err)
	}
	return buf.Bytes(), nil
}

func drawLabel(p *gopdf.GoPdf, lt *LabelTemplate, x, y float64, values map[string]string) {
	p.SetLineWidth(0.6)
	p.SetStrokeColor(90, 90, 90)
	p.RectFromUpperLeft(x, y, lt.Size.Width, lt.Size.Height)

	// 整块垂直居中：改字号或增删一行不会把内容挤到上边框。
	total := 0.0
	for _, l := range lt.Lines {
		total += l.SpaceBefore + lineHeight(l.Size) + l.SpaceAfter
	}
	yy := y + math.Max((lt.Size.Height-total)/2, 4)

	for _, l := range lt.Lines {
		yy += l.SpaceBefore
		h := lineHeight(l.Size)
		_ = p.SetFont("doc", "", l.Size)
		if l.Muted {
			p.SetTextColor(gray, gray, gray)
		} else {
			p.SetTextColor(0, 0, 0)
		}
		p.SetXY(x+8, yy)
		_ = p.CellWithOption(&gopdf.Rect{W: lt.Size.Width - 16, H: h},
			fill(l.Text, values), gopdf.CellOption{Align: alignOf(l.Align)})
		yy += h + l.SpaceAfter
	}
}

// cutLine 画一条细线当裁切标记：两张唛之间没有线，剪的时候容易把边框剪缺。
func cutLine(p *gopdf.GoPdf, y float64) {
	p.SetLineWidth(0.15)
	p.SetStrokeColor(190, 190, 190)
	p.Line(marginX, y, pageW-marginX, y)
}

// lineHeight 是字号对应的行高（毫米）。1pt ≈ 0.353mm，留 1.3 倍行距。
func lineHeight(size float64) float64 {
	return size * 0.46
}
