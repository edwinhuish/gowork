package pdf

import (
	"bytes"
	"fmt"
	"math"
	"regexp"
	"strings"

	"github.com/signintech/gopdf"
)

const (
	pageW        = 210.0 // A4 宽（mm）
	pageH        = 297.0
	marginX      = 15.0
	marginTop    = 15.0
	marginBottom = 16.0
	contentW     = pageW - 2*marginX

	rowH    = 6.4
	headerH = 6.4
	gray    = 110
)

// Data 是渲染数据：Values 供 {key} 取值，Items 是明细行，Tables 是附表行（如付款分期）。
//
// 用扁平字符串 map 而不是把 model 直接交给模板：模板不该知道数据库字段长什么样，
// 改一个列名不该让所有模板跟着改；缺键也能一眼看出来（渲染成空串），
// 而不是在模板里静默地取到一个零值。
type Data struct {
	Values map[string]string
	Items  []map[string]string
	Tables map[string][]map[string]string
}

// Renderer 渲染 PDF。字体只读一次并缓存在内存里：每份单据重读几 MB 没必要。
type Renderer struct {
	set      *Set
	fontName string
	fontData []byte
}

// NewRenderer 读字体文件并校验覆盖面。
//
// 必须是 TrueType（.ttf）：gopdf 不支持 .ttc 字体集合，而多数发行版的中文字体
// 恰好是 .ttc。校验交给 loadFont：拿到一个只含汉字的回退字体时，
// 渲染不会报错、只会静默丢掉所有拉丁字符，那种 PDF 看上去"只是有点空"，
// 而单号与金额恰恰就在拉丁字符里。
func NewRenderer(set *Set, fontPath string) (*Renderer, error) {
	b, err := loadFont(fontPath)
	if err != nil {
		return nil, err
	}
	return &Renderer{set: set, fontName: "doc", fontData: b}, nil
}

// Render 渲染指定模板（文件名去掉 .json），返回 PDF 字节。
func (r *Renderer) Render(name string, d Data) ([]byte, error) {
	t, err := r.set.Get(name)
	if err != nil {
		return nil, err
	}

	p := &gopdf.GoPdf{}
	p.Start(gopdf.Config{PageSize: *gopdf.PageSizeA4, Unit: gopdf.UnitMM})
	p.SetMargins(marginX, marginTop, marginX, marginBottom)
	// 字体注册一次，之后只用 SetFont 切字号。
	if err := p.AddTTFFontData(r.fontName, r.fontData); err != nil {
		return nil, fmt.Errorf("加载 PDF 字体: %w", err)
	}
	p.AddPage()

	w := &writer{p: p, y: marginTop}
	w.header(t, d)
	w.parties(t, d)
	w.items(t, d)
	w.summary(t, d)
	w.sections(t, d)
	w.notes(t, d)

	var buf bytes.Buffer
	if err := p.Write(&buf); err != nil {
		return nil, fmt.Errorf("生成 PDF: %w", err)
	}
	return buf.Bytes(), nil
}

type writer struct {
	p *gopdf.GoPdf
	y float64
}

func (w *writer) font(size float64) {
	_ = w.p.SetFont("doc", "", size)
}

// cell 在 (x, y) 画一段文本。坐标一律显式给：gopdf 的光标会随 Float 变化，
// 靠"上一步把光标留在了哪"排版，分页一次就全错位。
func (w *writer) cell(x, y, width, height float64, s string, align int) {
	w.p.SetXY(x, y)
	_ = w.p.CellWithOption(&gopdf.Rect{W: width, H: height}, s, gopdf.CellOption{Align: align})
}

func (w *writer) ensure(h float64) {
	if w.y+h <= pageH-marginBottom {
		return
	}
	w.p.AddPage()
	w.y = marginTop
}

// header 画公司抬头、单据标题、右上角键值对，并以一条横线收尾。
func (w *writer) header(t *Template, d Data) {
	w.font(8)
	w.p.SetTextColor(120, 120, 120)
	companyY := w.y
	for _, line := range t.Company {
		w.cell(marginX, companyY, contentW*0.55, 4.6, fill(line, d.Values), gopdf.Left)
		companyY += 4.6
	}

	w.font(16)
	w.p.SetTextColor(0, 0, 0)
	w.cell(marginX, w.y, contentW, 8, fill(t.Title, d.Values), gopdf.Right)
	metaY := w.y + 9
	if t.Subtitle != "" {
		w.font(9)
		w.p.SetTextColor(gray, gray, gray)
		w.cell(marginX, metaY, contentW, 5, fill(t.Subtitle, d.Values), gopdf.Right)
		metaY += 6
	}

	// 键值对右对齐成两列：标签靠右对齐到固定位置，值再靠右。
	w.font(9)
	for _, f := range t.Meta {
		w.p.SetTextColor(gray, gray, gray)
		w.cell(marginX+contentW-90, metaY, 42, 4.6, fill(f.Label, d.Values), gopdf.Left)
		w.p.SetTextColor(0, 0, 0)
		w.cell(marginX+contentW-48, metaY, 48, 4.6, fill(f.Value, d.Values), gopdf.Right)
		metaY += 4.6
	}

	y := math.Max(companyY, metaY) + 2
	w.p.SetLineWidth(0.4)
	w.p.SetStrokeColor(150, 150, 150)
	w.p.Line(marginX, y, pageW-marginX, y)
	w.y = y + 7
}

// parties 画客户 / 参考信息区块，两列并排，每行高度取两列较高者。
func (w *writer) parties(t *Template, d Data) {
	if len(t.Parties) == 0 {
		return
	}
	w.font(8.5)
	const colW = contentW / 2

	y := w.y
	for i := 0; i < len(t.Parties); i += 2 {
		rowY := y
		rowHeight := 0.0
		for j := 0; j < 2 && i+j < len(t.Parties); j++ {
			party := t.Parties[i+j]
			x := marginX + float64(j)*colW

			w.p.SetTextColor(gray, gray, gray)
			w.cell(x, rowY, colW-4, 4.8, fill(party.Label, d.Values), gopdf.Left)
			w.p.SetTextColor(0, 0, 0)

			yy := rowY + 5
			for _, line := range party.Lines {
				s := fill(line, d.Values)
				// 空行不占位：模板里为了排版留的空串不该变成一段空白。
				if strings.TrimSpace(s) == "" {
					continue
				}
				// 占位为空时会剩下孤零零的标签（"Attn:"），看着像漏打了字；
				// 这一行整体跳过比印一个空标签好。
				if strings.HasSuffix(strings.TrimSpace(s), ":") {
					continue
				}
				w.cell(x, yy, colW-4, 4.6, s, gopdf.Left)
				yy += 4.6
			}
			if h := yy - rowY; h > rowHeight {
				rowHeight = h
			}
		}
		y += rowHeight + 2
	}
	w.y = y + 1
}

func (w *writer) items(t *Template, d Data) {
	cols := t.Items.Columns
	w.ensure(headerH + rowH)
	w.tableHeader(cols)

	if len(d.Items) == 0 {
		w.font(9)
		w.p.SetTextColor(gray, gray, gray)
		w.cell(marginX, w.y, contentW, rowH, t.Items.Empty, gopdf.Center)
		w.p.SetTextColor(0, 0, 0)
		w.y += rowH + 2
		return
	}

	w.tableRows(cols, d.Items)
	w.y += 2
}

func (w *writer) tableHeader(cols []Column) {
	w.font(8.5)
	w.p.SetTextColor(gray, gray, gray)
	x := marginX
	for _, c := range cols {
		w.cell(x, w.y, c.Width-2, headerH, c.Title, alignOf(c.Align))
		x += c.Width
	}
	w.p.SetTextColor(0, 0, 0)
	w.p.SetLineWidth(0.3)
	w.p.SetStrokeColor(130, 130, 130)
	w.p.Line(marginX, w.y+headerH, pageW-marginX, w.y+headerH)
	w.y += headerH
}

// tableRows 画数据行，跨页时重画表头。
//
// 行高固定、超宽截断：一行折成两行会打乱行高，表格上下就错位；
// 而"这行到底写了什么"在系统里能查，PDF 是给客户看的摘要。
func (w *writer) tableRows(cols []Column, rows []map[string]string) {
	w.font(9)
	for _, row := range rows {
		if w.y+rowH > pageH-marginBottom {
			w.p.AddPage()
			w.y = marginTop
			w.tableHeader(cols)
			w.font(9)
		}
		x := marginX
		for _, c := range cols {
			text := fitWidth(w.p, row[c.Key], c.Width-4)
			w.cell(x, w.y, c.Width-2, rowH, text, alignOf(c.Align))
			x += c.Width
		}
		w.p.SetLineWidth(0.15)
		w.p.SetStrokeColor(215, 215, 215)
		w.p.Line(marginX, w.y+rowH, pageW-marginX, w.y+rowH)
		w.y += rowH
	}
}

// summary 画合计，靠右两列：标签左对齐、金额右对齐。
func (w *writer) summary(t *Template, d Data) {
	if len(t.Summary) == 0 {
		return
	}
	w.ensure(float64(len(t.Summary))*5.4 + 3)

	x := marginX + contentW - 110
	for i, f := range t.Summary {
		// 最后一行是总额，给更大字号：客户扫一眼看的就是它。
		if i == len(t.Summary)-1 {
			w.font(11)
		} else {
			w.font(9.5)
		}
		w.cell(x, w.y, 55, 5.4, fill(f.Label, d.Values), gopdf.Left)
		w.cell(x+55, w.y, 55, 5.4, fill(f.Value, d.Values), gopdf.Right)
		w.y += 5.4
	}
	w.font(9)
	w.y += 3
}

func (w *writer) sections(t *Template, d Data) {
	for _, sec := range t.Sections {
		rows := d.Tables[sec.Rows]
		w.ensure(headerH*2 + rowH)
		if w.y > marginTop+1 {
			w.y += 2
		}

		w.font(9)
		w.p.SetTextColor(gray, gray, gray)
		w.cell(marginX, w.y, contentW, 5.2, fill(sec.Title, d.Values), gopdf.Left)
		w.p.SetTextColor(0, 0, 0)
		w.y += 6

		w.tableHeader(sec.Columns)
		if len(rows) == 0 {
			w.font(9)
			w.p.SetTextColor(gray, gray, gray)
			w.cell(marginX, w.y, contentW, rowH, "—", gopdf.Left)
			w.p.SetTextColor(0, 0, 0)
			w.y += rowH
		} else {
			w.tableRows(sec.Columns, rows)
		}

		for _, f := range sec.Total {
			w.font(9.5)
			x := marginX + contentW - 110
			w.cell(x, w.y+1, 55, 5.2, fill(f.Label, d.Values), gopdf.Left)
			w.cell(x+55, w.y+1, 55, 5.2, fill(f.Value, d.Values), gopdf.Right)
			w.y += 5.2
		}
		w.font(9)
		w.y += 3
	}
}

func (w *writer) notes(t *Template, d Data) {
	if len(t.Notes) == 0 {
		return
	}
	w.font(8)
	w.p.SetTextColor(gray, gray, gray)
	w.y += 3
	for _, n := range t.Notes {
		s := fill(n, d.Values)
		if strings.TrimSpace(s) == "" {
			continue
		}
		if w.y+4.4 > pageH-marginBottom {
			w.p.AddPage()
			w.y = marginTop
		}
		w.cell(marginX, w.y, contentW, 4.4, s, gopdf.Left)
		w.y += 4.4
	}
}

var placeholder = regexp.MustCompile(`\{([a-zA-Z0-9_]+)\}`)

// fill 替换 {key}。缺键渲染成空串而不是原样保留：
// 模板语法漏到客户面前，比少一行字更糟。
func fill(s string, values map[string]string) string {
	if !strings.Contains(s, "{") {
		return s
	}
	return placeholder.ReplaceAllStringFunc(s, func(m string) string {
		return values[m[1:len(m)-1]]
	})
}

func alignOf(s string) int {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "right":
		return gopdf.Right
	case "center":
		return gopdf.Center
	default:
		return gopdf.Left
	}
}

// fitWidth 把超宽文本截断并加省略号。
func fitWidth(p *gopdf.GoPdf, s string, width float64) string {
	if s == "" {
		return ""
	}
	if sw, err := p.MeasureTextWidth(s); err == nil && sw <= width {
		return s
	}
	runes := []rune(s)
	for len(runes) > 1 {
		runes = runes[:len(runes)-1]
		cand := string(runes) + "…"
		if sw, err := p.MeasureTextWidth(cand); err == nil && sw <= width {
			return cand
		}
	}
	return ""
}
