// Package pdf 把单据渲染成 PDF。
//
// 版式写在 JSON 模板里而不是 Go 代码里：单据格式会跟着客户要求变
// （加一列、换标题、补两行条款），改一次模板不该要求改代码、重新编译、重新发版。
// 为此渲染只认一小组原语：抬头、右上角键值、客户区块、明细表、合计、
// 附表（付款分期）与页脚说明。**不支持任意排版是有意的**：
// 模板能做的事越多，"改一下模板"就越容易变成改程序。
//
// 箱唛（shipping mark）不走这套原语，见 label.go：它是等大贴纸排版，
// 一张纸贴一个箱子，只有一列大字加几行留白，塞进单据模板里两边都会变形。
package pdf

import (
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

//go:embed templates/*.json
var builtinFS embed.FS

// builtinDir 不带尾斜杠：embed.FS 的 ReadDir 不接受 "templates/" 这种写法，
// 会直接报 file does not exist。
const builtinDir = "templates"

// Field 是一行「标签 + 值」，两处都可含 {key} 占位。
type Field struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

// Party 是客户 / 参考信息区块：一个标签加若干行。
type Party struct {
	Label string   `json:"label"`
	Lines []string `json:"lines"`
}

// Column 是表格的一列。Width 单位为毫米，Align 取 left / right / center。
type Column struct {
	Title string  `json:"title"`
	Key   string  `json:"key"`
	Width float64 `json:"width"`
	Align string  `json:"align"`
}

// Items 是明细表。
type Items struct {
	Columns []Column `json:"columns"`
	// Empty 是没有明细时的提示语：留空会渲染出一张只有表头的空表，
	// 看不出是"没有货"还是"数据没取到"。
	Empty string `json:"empty"`
}

// Section 是明细之后的附表，例如合同的付款分期。
type Section struct {
	Title   string   `json:"title"`
	Columns []Column `json:"columns"`
	// Rows 是数据里这张表的键。
	Rows string `json:"rows"`
	// Total 是表格下方的合计行。
	Total []Field `json:"total"`
}

// Template 是一份单据的版式。
type Template struct {
	Title    string    `json:"title"`
	Subtitle string    `json:"subtitle"`
	Company  []string  `json:"company"`
	Meta     []Field   `json:"meta"`
	Parties  []Party   `json:"parties"`
	Items    Items     `json:"items"`
	Summary  []Field   `json:"summary"`
	Sections []Section `json:"sections"`
	Notes    []string  `json:"notes"`
}

// Set 是一组按名字取用的模板。
//
// 目录模式下按文件修改时间缓存：改完模板下一次请求就生效，
// 「不用改代码」要连「不用重启」一起做到才算数。
type Set struct {
	dir string

	mu    sync.Mutex
	cache map[string]cachedTemplate
}

// cachedTemplate 两种模板共用一份缓存：单据（tmpl）与箱唛（label）。
//
// 同一个目录、同一套"按修改时间失效"的逻辑，只是解析成不同的结构；
// 分成两个目录会让 pdf.template_dir 的覆盖规则变成两句话。
type cachedTemplate struct {
	tmpl  *Template
	label *LabelTemplate
	mod   time.Time
}

// Load 加载模板集合。dir 为空时用随二进制打包的内置模板。
func Load(dir string) (*Set, error) {
	s := &Set{dir: dir, cache: make(map[string]cachedTemplate)}

	if dir != "" {
		st, err := os.Stat(dir)
		if err != nil {
			return nil, fmt.Errorf("模板目录不可用 %s: %w", dir, err)
		}
		if !st.IsDir() {
			return nil, fmt.Errorf("模板路径不是目录: %s", dir)
		}
		return s, nil
	}

	entries, err := builtinFS.ReadDir(builtinDir)
	if err != nil {
		return nil, fmt.Errorf("读取内置模板: %w", err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		b, err := builtinFS.ReadFile(builtinDir + "/" + e.Name())
		if err != nil {
			return nil, fmt.Errorf("读取内置模板 %s: %w", e.Name(), err)
		}
		name := strings.TrimSuffix(e.Name(), ".json")
		c, err := parseAnyTemplate(name, b)
		if err != nil {
			return nil, err
		}
		s.cache[name] = c
	}
	return s, nil
}

// Get 按名字取单据模板（文件名去掉 .json）。
func (s *Set) Get(name string) (*Template, error) {
	c, err := s.lookup(name)
	if err != nil {
		return nil, err
	}
	if c.tmpl == nil {
		return nil, fmt.Errorf("PDF 模板 %q 是箱唛，不能当单据打印", name)
	}
	return c.tmpl, nil
}

// GetLabel 按名字取箱唛模板。
func (s *Set) GetLabel(name string) (*LabelTemplate, error) {
	c, err := s.lookup(name)
	if err != nil {
		return nil, err
	}
	if c.label == nil {
		return nil, fmt.Errorf("PDF 模板 %q 是单据，不能当箱唛打印", name)
	}
	return c.label, nil
}

// lookup 取模板并按需重新解析：目录模式下文件改了，下一次请求就生效。
func (s *Set) lookup(name string) (cachedTemplate, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.dir == "" {
		c, ok := s.cache[name]
		if !ok {
			return cachedTemplate{}, fmt.Errorf("未知的 PDF 模板 %q", name)
		}
		return c, nil
	}

	path := filepath.Join(s.dir, name+".json")
	st, err := os.Stat(path)
	if err != nil {
		return cachedTemplate{}, fmt.Errorf("PDF 模板 %q 不存在: %w", name, err)
	}
	if c, ok := s.cache[name]; ok && !st.ModTime().After(c.mod) {
		return c, nil
	}

	b, err := os.ReadFile(path)
	if err != nil {
		return cachedTemplate{}, fmt.Errorf("读取 PDF 模板 %s: %w", path, err)
	}
	c, err := parseAnyTemplate(name, b)
	if err != nil {
		return cachedTemplate{}, err
	}
	c.mod = st.ModTime()
	s.cache[name] = c
	return c, nil
}

// parseAnyTemplate 按内容分派：带 label 段的是箱唛，其余按单据模板校验。
//
// 用一个目录而不是两个，是因为"覆盖模板"只需要记住一条规则：
// 把同名文件放进 pdf.template_dir 就生效。
func parseAnyTemplate(name string, b []byte) (cachedTemplate, error) {
	var probe struct {
		Label json.RawMessage `json:"label"`
	}
	if err := json.Unmarshal(b, &probe); err != nil {
		return cachedTemplate{}, fmt.Errorf("解析 PDF 模板 %s: %w", name, err)
	}
	if len(probe.Label) > 0 {
		lt, err := parseLabel(name, b)
		if err != nil {
			return cachedTemplate{}, err
		}
		return cachedTemplate{label: lt}, nil
	}

	t, err := parseTemplate(name, b)
	if err != nil {
		return cachedTemplate{}, err
	}
	return cachedTemplate{tmpl: t}, nil
}

// Names 列出可用模板名，启动自检与测试用。
func (s *Set) Names() []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	names := make([]string, 0, len(s.cache))
	for n := range s.cache {
		names = append(names, n)
	}
	if s.dir != "" {
		matches, _ := filepath.Glob(filepath.Join(s.dir, "*.json"))
		for _, m := range matches {
			names = append(names, strings.TrimSuffix(filepath.Base(m), ".json"))
		}
	}
	sort.Strings(names)
	return names
}

func parseTemplate(name string, b []byte) (*Template, error) {
	var t Template
	if err := json.Unmarshal(b, &t); err != nil {
		return nil, fmt.Errorf("解析 PDF 模板 %s: %w", name, err)
	}
	if strings.TrimSpace(t.Title) == "" {
		return nil, fmt.Errorf("PDF 模板 %s 缺少 title", name)
	}
	if len(t.Items.Columns) == 0 {
		return nil, fmt.Errorf("PDF 模板 %s 的 items.columns 不能为空", name)
	}
	for _, sec := range t.Sections {
		if sec.Rows == "" || len(sec.Columns) == 0 {
			return nil, fmt.Errorf("PDF 模板 %s 的附表 %q 缺少 rows 或 columns", name, sec.Title)
		}
	}
	return &t, nil
}
