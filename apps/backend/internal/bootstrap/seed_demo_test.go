package bootstrap_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/edwinhuish/gowork/internal/bootstrap"
	"github.com/edwinhuish/gowork/internal/config"
)

// newDemoE2EApp 起一个开了 demo_seed 的真实装配。
//
// 这块数据是给"打开页面就能看到东西"用的：演示数据写错（少一张应收、
// 账龄没数、待办全空）在界面上不像报错，只会让人以为功能没做。
// 所以用同一条 HTTP 链路去断言每一页的列表都非空。
func newDemoE2EApp(t *testing.T) (*bootstrap.App, *httptest.Server) {
	t.Helper()

	dir := t.TempDir()
	dsn := "file:" + filepath.Join(dir, "demo.db") +
		"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"

	cfgPath := filepath.Join(dir, "config.yaml")
	raw := fmt.Sprintf(`
app:
  name: gowork-demo
  env: test
database:
  driver: sqlite
  dsn: %q
  auto_migrate: true
  seed: true
  demo_seed: true
cache:
  driver: memory
  memory:
    persist: false
jwt:
  secret: %s
  expire: 1
log:
  level: warn
  format: text
`, dsn, strings.Repeat("d", 32))
	if err := os.WriteFile(cfgPath, []byte(raw), 0o600); err != nil {
		t.Fatalf("写临时配置失败: %v", err)
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("读配置失败: %v", err)
	}

	app, err := bootstrap.New(cfg)
	if err != nil {
		t.Fatalf("装配失败: %v", err)
	}
	t.Cleanup(app.Close)

	srv := httptest.NewServer(app.Engine)
	t.Cleanup(srv.Close)
	return app, srv
}

// listTotal 取列表接口的 total，顺手断言业务码与 HTTP 状态。
func listTotal(t *testing.T, srv *httptest.Server, token, path string) int {
	t.Helper()
	res := call(t, srv, http.MethodGet, path, token, nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET %s = %d", path, res.StatusCode)
	}
	code, msg, data := decodeEnvelope(t, res)
	if code != 0 {
		t.Fatalf("GET %s 业务错误 %d %s", path, code, msg)
	}
	var page struct {
		Total int `json:"total"`
	}
	if err := json.Unmarshal(data, &page); err != nil {
		t.Fatalf("GET %s 返回的不是分页结果: %s", path, data)
	}
	return page.Total
}

// TestDemoSeedCoversMainChain 断言演示数据把主链每一环都填上了。
//
// 顺序按业务发生顺序：主数据 → CRM → 报价/合同/订单 → 出货/报关/费用
// → 应收/应付/收付款 → 库存 → 报表。任何一环空了，
// 对应的页面打开就是"没有数据"，而这一页的功能等于没演示到。
func TestDemoSeedCoversMainChain(t *testing.T) {
	_, srv := newDemoE2EApp(t)
	token := login(t, srv)

	cases := []struct {
		name string
		path string
		min  int
	}{
		{"客户", "/api/v1/customers", 6},
		{"供应商", "/api/v1/suppliers", 4},
		{"货代", "/api/v1/forwarders", 2},
		{"产品", "/api/v1/products", 16},
		{"联系人", "/api/v1/contacts", 5},
		{"跟进记录", "/api/v1/activities", 6},
		{"线索", "/api/v1/leads", 4},
		{"商机", "/api/v1/opportunities", 4},
		{"条款库", "/api/v1/clauses", 4},
		{"报价单", "/api/v1/quotations", 3},
		{"合同", "/api/v1/contracts", 1},
		{"销售订单", "/api/v1/sales-orders", 4},
		{"采购订单", "/api/v1/purchase-orders", 1},
		{"出货单", "/api/v1/shipments", 2},
		{"运杂费", "/api/v1/shipment-charges", 3},
		{"报关单", "/api/v1/customs-declarations", 1},
		{"应收", "/api/v1/receivables", 4},
		{"应付", "/api/v1/payables", 1},
		{"收付款", "/api/v1/payments", 3},
		{"出入库单", "/api/v1/stock-documents", 2},
		{"库存流水", "/api/v1/stock-movements", 1},
		{"当前库存", "/api/v1/stock-snapshots", 1},
	}
	for _, c := range cases {
		if total := listTotal(t, srv, token, c.path); total < c.min {
			t.Errorf("%s 只有 %d 条，期望至少 %d 条（%s）", c.name, total, c.min, c.path)
		}
	}

	// 有单据不等于数据对：下面几处断言的是"算出来的东西不为零"，
	// 它们分别代表账龄、回款率、销售看板与汇兑损益四个报表的口径。
	assertReportNotZero(t, srv, token, "待跟进（todos.followup_count）", "/api/v1/reports/todos", "followup_count")
	assertReportNotZero(t, srv, token, "应收待催（todos.receivable_count）", "/api/v1/reports/todos", "receivable_count")
	assertReportNotZero(t, srv, token, "待验货（todos.inspection_count）", "/api/v1/reports/todos", "inspection_count")
	assertReportNotZero(t, srv, token, "合同待收（todos.contract_term_count）", "/api/v1/reports/todos", "contract_term_count")
	assertReportNotZero(t, srv, token, "账龄合计（aging.grand_total_base）", "/api/v1/reports/aging?direction=10", "grand_total_base")
	assertReportNotZero(t, srv, token, "本期回款（collection.received_base）", "/api/v1/reports/collection?direction=10", "received_base")
	assertReportNotZero(t, srv, token, "逾期余额（collection.overdue_base）", "/api/v1/reports/collection?direction=10", "overdue_base")
	assertReportNotZero(t, srv, token, "销售看板收入（sales-dashboard）", "/api/v1/reports/sales-dashboard?dimension=customer", "total")
	assertReportNotZero(t, srv, token, "汇兑损益（exchange-gains）", "/api/v1/reports/exchange-gains", "total_gain_base")
	assertReportNotZero(t, srv, token, "单票毛利（shipment-profits）", "/api/v1/reports/shipment-profits", "total")

	// 采购单详情里的两个面板：收货与验货不单独成页，没有它们那条详情页就少两块内容。
	assertHasRows(t, srv, token, "收货单", "/api/v1/purchase-orders/1/receipts")
	assertHasRows(t, srv, token, "验货记录", "/api/v1/purchase-orders/1/inspections")

	// 产品图：卡片视图全靠它，一张没有就是一片占位图标。
	assertEveryProductHasImage(t, srv, token)
}

// assertEveryProductHasImage 断言每个产品都有主图，且缩略图真的取得到。
//
// 只断言"有 primary_image"不够：URL 拼错了照样能过，界面上却是 16 个占位图标。
// 所以再拿第一张缩略图真的下载一次，看它是不是一张 PNG。
func assertEveryProductHasImage(t *testing.T, srv *httptest.Server, token string) {
	t.Helper()
	res := call(t, srv, http.MethodGet, "/api/v1/products?page_size=100", token, nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("产品列表: %d", res.StatusCode)
	}
	_, msg, data := decodeEnvelope(t, res)
	var page struct {
		List []struct {
			SKU   string `json:"sku_code"`
			Image *struct {
				ThumbURL string `json:"thumb_url"`
			} `json:"primary_image"`
		} `json:"list"`
	}
	if err := json.Unmarshal(data, &page); err != nil {
		t.Fatalf("产品列表解码失败: %v（%s）", err, msg)
	}
	if len(page.List) == 0 {
		t.Fatal("产品列表是空的")
	}
	missing := make([]string, 0, len(page.List))
	for _, p := range page.List {
		if p.Image == nil || p.Image.ThumbURL == "" {
			missing = append(missing, p.SKU)
		}
	}
	if len(missing) > 0 {
		t.Errorf("这些产品没有主图：%v", missing)
		return
	}

	thumb := call(t, srv, http.MethodGet, "/api/v1"+page.List[0].Image.ThumbURL, token, nil)
	if thumb.StatusCode != http.StatusOK {
		t.Fatalf("取缩略图 %s = %d", page.List[0].Image.ThumbURL, thumb.StatusCode)
	}
	// 缩略图由上传服务重新编码（统一转 JPEG 省体积），所以只断言"是图"，
	// 不锁死具体格式 —— 换成 webp 缩略图不该让这条断言变红。
	if ct := thumb.Header.Get("Content-Type"); !strings.HasPrefix(ct, "image/") {
		t.Errorf("缩略图 Content-Type = %q，期望 image/*", ct)
	}
}

// assertHasRows 断言接口回来了至少一行数据。
//
// 挂在单据下的子列表形状不统一：有的直接返回数组，有的包一层（收货单是
// `{receipts, received_by_item}`）。所以两种都试：能解成非空数组就通过，
// 否则在对象里找第一个非空数组。要的是"有数据"，不是锁死某个外层字段名。
func assertHasRows(t *testing.T, srv *httptest.Server, token, name, path string) {
	t.Helper()
	res := call(t, srv, http.MethodGet, path, token, nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("%s: GET %s = %d", name, path, res.StatusCode)
	}
	code, msg, data := decodeEnvelope(t, res)
	if code != 0 {
		t.Fatalf("%s: 业务错误 %d %s", name, code, msg)
	}

	var list []json.RawMessage
	if err := json.Unmarshal(data, &list); err == nil {
		if len(list) == 0 {
			t.Errorf("%s 是空的（%s），这一块在详情页里会是空白", name, path)
		}
		return
	}

	var object map[string]json.RawMessage
	if err := json.Unmarshal(data, &object); err != nil {
		t.Fatalf("%s: 返回既不是数组也不是对象: %s", name, data)
	}
	for _, raw := range object {
		var rows []json.RawMessage
		if err := json.Unmarshal(raw, &rows); err == nil && len(rows) > 0 {
			return
		}
	}
	t.Errorf("%s 是空的（%s），这一块在详情页里会是空白: %s", name, path, data)
}

// assertReportNotZero 只断言字段存在且不为零：这些字段都是数字或对象，
// 具体金额由业务决定，钉死在测试里会随汇率调整而天天变红。
func assertReportNotZero(t *testing.T, srv *httptest.Server, token, name, path, field string) {
	t.Helper()
	res := call(t, srv, http.MethodGet, path, token, nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("%s: GET %s = %d", name, path, res.StatusCode)
	}
	code, msg, data := decodeEnvelope(t, res)
	if code != 0 {
		t.Fatalf("%s: 业务错误 %d %s", name, code, msg)
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("%s: 返回不是对象: %s", name, data)
	}
	raw, ok := payload[field]
	if !ok {
		t.Fatalf("%s: 返回里没有 %s 字段: %s", name, field, data)
	}
	if string(raw) == "0" || string(raw) == `"0"` || string(raw) == "null" ||
		string(raw) == `"0.00"` || string(raw) == "{}" || string(raw) == "[]" {
		t.Errorf("%s 是空的（%s = %s），页面打开会看不到数", name, field, raw)
	}
}
