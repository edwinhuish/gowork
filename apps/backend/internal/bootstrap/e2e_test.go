package bootstrap_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/edwinhuish/gowork/internal/bootstrap"
	"github.com/edwinhuish/gowork/internal/config"
)

// 这一组是**跨层冒烟**：走真实装配（配置 → 数据库 → 仓储 → 服务 → 中间件 → 路由），
// 用 httptest 起真 HTTP 服务。单元测试各自替身把边界都擦干净了，
// 而"中间件顺序反了""某条路由忘了挂""登录后拿到的令牌进不去列表"这类问题
// 只会在整条链路上暴露。
//
// 不占端口也不留进程：httptest 在测试结束时自己关掉。
func newE2EApp(t *testing.T) (*bootstrap.App, *httptest.Server) {
	t.Helper()

	dir := t.TempDir()
	dsn := "file:" + filepath.Join(dir, "e2e.db") +
		"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"

	// 走**真实配置路径**（写临时 YAML → Load）而不是拼一个 Config 字面量：
	// 字面量会绕过 applyDefaults，`auto_migrate` 于是是零值 false —— 表不会被建，
	// 随后的数据迁移报"no such table"。这个坑第一次写这条用例时就踩到了。
	cfgPath := filepath.Join(dir, "config.yaml")
	raw := fmt.Sprintf(`
app:
  name: gowork-e2e
  env: test
database:
  driver: sqlite
  dsn: %q
  auto_migrate: true
  seed: true
cache:
  driver: memory
jwt:
  secret: %s
  expire: 1
log:
  level: error
  format: text
`, dsn, strings.Repeat("s", 32))
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

func call(t *testing.T, srv *httptest.Server, method, path, token string, body []byte) *http.Response {
	t.Helper()
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, srv.URL+path, reader)
	if err != nil {
		t.Fatalf("构造请求失败: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("请求 %s %s 失败: %v", method, path, err)
	}
	return res
}

func decodeEnvelope(t *testing.T, res *http.Response) (code int, message string, data json.RawMessage) {
	t.Helper()
	defer res.Body.Close()
	var body struct {
		Code    int             `json:"code"`
		Message string          `json:"message"`
		Data    json.RawMessage `json:"data"`
	}
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("读响应失败: %v", err)
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("响应不是约定的信封格式: %v, raw=%s", err, raw)
	}
	return body.Code, body.Message, body.Data
}

func login(t *testing.T, srv *httptest.Server) string {
	t.Helper()
	res := call(t, srv, http.MethodPost, "/api/v1/auth/login", "",
		[]byte(`{"username":"admin","password":"admin123"}`))
	if res.StatusCode != http.StatusOK {
		t.Fatalf("管理员登录失败: %d", res.StatusCode)
	}
	_, _, data := decodeEnvelope(t, res)
	var payload struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(data, &payload); err != nil || payload.Token == "" {
		t.Fatalf("登录响应里没有令牌: %s", data)
	}
	return payload.Token
}

func TestE2ESmoke(t *testing.T) {
	app, srv := newE2EApp(t)

	t.Run("探活不需要令牌", func(t *testing.T) {
		res := call(t, srv, http.MethodGet, "/healthz", "", nil)
		defer res.Body.Close()
		if res.StatusCode != http.StatusOK {
			t.Errorf("healthz = %d，期望 200", res.StatusCode)
		}
	})

	t.Run("没有令牌进不去业务接口", func(t *testing.T) {
		res := call(t, srv, http.MethodGet, "/api/v1/partners", "", nil)
		if res.StatusCode != http.StatusUnauthorized {
			t.Fatalf("无令牌应 401，得到 %d", res.StatusCode)
		}
		_, msg, _ := decodeEnvelope(t, res)
		if msg == "" {
			t.Error("401 也该有可读的提示")
		}
	})

	token := login(t, srv)

	t.Run("登录后可建主体并出现在列表里", func(t *testing.T) {
		res := call(t, srv, http.MethodPost, "/api/v1/partners", token,
			[]byte(`{"code":"E2E001","name":"冒烟测试客户","role":1,"entity_type":1}`))
		if res.StatusCode != http.StatusOK && res.StatusCode != http.StatusCreated {
			t.Fatalf("建主体失败: %d", res.StatusCode)
		}
		code, msg, _ := decodeEnvelope(t, res)
		if code != 0 {
			t.Fatalf("建主体返回业务错误: %d %s", code, msg)
		}

		res = call(t, srv, http.MethodGet, "/api/v1/partners?keyword=E2E001", token, nil)
		if res.StatusCode != http.StatusOK {
			t.Fatalf("列表失败: %d", res.StatusCode)
		}
		_, _, data := decodeEnvelope(t, res)
		if !bytes.Contains(data, []byte("E2E001")) {
			t.Errorf("新建的主体没出现在列表里: %s", data)
		}
	})

	// 导出跨了「鉴权 → 数据范围 → 分页取数 → CSV 写出」四层，
	// 是这条链路上最容易在某个环节悄悄返回空文件的地方。
	t.Run("导出返回带 BOM 的 CSV 且含刚建的数据", func(t *testing.T) {
		res := call(t, srv, http.MethodGet, "/api/v1/exports/partners?role=1", token, nil)
		defer res.Body.Close()
		if res.StatusCode != http.StatusOK {
			t.Fatalf("导出失败: %d", res.StatusCode)
		}
		if ct := res.Header.Get("Content-Type"); !strings.Contains(ct, "text/csv") {
			t.Errorf("Content-Type = %q，期望 text/csv", ct)
		}
		if cd := res.Header.Get("Content-Disposition"); !strings.Contains(cd, "attachment") {
			t.Errorf("Content-Disposition = %q，期望下载", cd)
		}

		raw, err := io.ReadAll(res.Body)
		if err != nil {
			t.Fatalf("读导出内容失败: %v", err)
		}
		if !bytes.HasPrefix(raw, []byte{0xEF, 0xBB, 0xBF}) {
			t.Error("导出缺少 UTF-8 BOM，Excel 打开会乱码")
		}
		if !bytes.Contains(raw, []byte("E2E001")) {
			t.Errorf("导出里没有刚建的主体: %s", string(raw))
		}
	})

	t.Run("超大请求体在中间件层就被挡住", func(t *testing.T) {
		// 不真的造 32MB 数据：直接打引擎，只伪造 Content-Length。
		// （走 http.Client 不行——它会校验 Content-Length 与实际 body 长度一致。）
		// 用例要验的正是"按头就拒，还没读 body"。
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{}`))
		req.ContentLength = 64 << 20
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		app.Engine.ServeHTTP(w, req)

		if w.Code != http.StatusRequestEntityTooLarge {
			t.Errorf("超大请求应 413，得到 %d（body=%s）", w.Code, w.Body.String())
		}
	})

	t.Run("错误密码不泄露账号是否存在", func(t *testing.T) {
		known := call(t, srv, http.MethodPost, "/api/v1/auth/login", "",
			[]byte(`{"username":"admin","password":"wrong"}`))
		_, knownMsg, _ := decodeEnvelope(t, known)

		unknown := call(t, srv, http.MethodPost, "/api/v1/auth/login", "",
			[]byte(`{"username":"nobody-here","password":"wrong"}`))
		_, unknownMsg, _ := decodeEnvelope(t, unknown)

		if knownMsg != unknownMsg {
			t.Errorf("两种失败必须同一文案（防账号枚举）：%q vs %q", knownMsg, unknownMsg)
		}
	})
}

// 主链冒烟：报价 → 确认 → 转订单 → 出货 → 确认出货（触发库存扣减）。
//
// 这条链上每一步都换一个服务、换一张表：金额在报价里算、库存与分配在出货里算，
// 而"确认出货就该动库存"（T-40）是跨两个服务的同一个事务。
// 单测各自验证了自己的那一段，但"整条链能不能走通"只有连起来才知道。
func TestE2ESalesChain(t *testing.T) {
	_, srv := newE2EApp(t)
	token := login(t, srv)

	// 种子数据里有产品与客户：不硬编码 id，按列表查第一个。
	productID := firstID(t, srv, token, "/api/v1/products?page_size=1")
	customerID := firstID(t, srv, token, "/api/v1/partners?role=1&page_size=1")
	supplierID := firstID(t, srv, token, "/api/v1/partners?role=2&page_size=1")

	// 报价要算成本与毛利，所以产品必须先有货源——种子数据只建了产品没有货源，
	// 直接报价会被服务层拒掉（提示"没有启用的货源"）。
	// 这条路径也顺带验证了"没有成本就不让报价"这条业务规则真的生效。
	sourceBody := fmt.Sprintf(`{
		"product_id": %d, "partner_id": %d, "supplier_sku": "E2E-SKU",
		"purchase_price": "60", "moq": 1, "delivery_days": 7
	}`, productID, supplierID)
	res := call(t, srv, http.MethodPost, "/api/v1/product-sources", token, []byte(sourceBody))
	if res.StatusCode != http.StatusOK && res.StatusCode != http.StatusCreated {
		_, msg, _ := decodeEnvelope(t, res)
		t.Fatalf("建货源失败: %d %s", res.StatusCode, msg)
	}
	decodeEnvelope(t, res)

	quotationBody := fmt.Sprintf(`{
		"partner_id": %d,
		"trade_term": "FOB",
		"currency_code": "USD",
		"valid_until": "2026-12-31",
		"items": [{"product_id": %d, "qty": "10", "unit_price": "100"}]
	}`, customerID, productID)
	res = call(t, srv, http.MethodPost, "/api/v1/quotations", token, []byte(quotationBody))
	if res.StatusCode != http.StatusOK && res.StatusCode != http.StatusCreated {
		_, msg, _ := decodeEnvelope(t, res)
		t.Fatalf("建报价失败: %d %s", res.StatusCode, msg)
	}
	quotationID := idOf(t, res)
	if quotationID == 0 {
		t.Fatal("报价没有返回 id")
	}

	t.Run("确认报价并转成订单", func(t *testing.T) {
		res := call(t, srv, http.MethodPut,
			fmt.Sprintf("/api/v1/quotations/%d/status", quotationID), token,
			[]byte(`{"status":20}`))
		if res.StatusCode != http.StatusOK {
			t.Fatalf("确认报价失败: %d", res.StatusCode)
		}
		decodeEnvelope(t, res)

		res = call(t, srv, http.MethodPost,
			fmt.Sprintf("/api/v1/quotations/%d/convert", quotationID), token,
			[]byte(`{"order_date":"2026-10-29"}`))
		if res.StatusCode != http.StatusOK && res.StatusCode != http.StatusCreated {
			t.Fatalf("转订单失败: %d", res.StatusCode)
		}
		decodeEnvelope(t, res)
	})

	// 订单行 id 是出货分配的锚点，从详情里取。
	orderID := firstID(t, srv, token, "/api/v1/sales-orders?page_size=1")
	var order struct {
		Items []struct {
			ID uint `json:"id"`
		} `json:"items"`
	}
	res = call(t, srv, http.MethodGet, fmt.Sprintf("/api/v1/sales-orders/%d", orderID), token, nil)
	_, _, data := decodeEnvelope(t, res)
	if err := json.Unmarshal(data, &order); err != nil || len(order.Items) == 0 {
		t.Fatalf("订单详情里没有明细: %s", data)
	}
	itemID := order.Items[0].ID

	// 转出来的订单还是草稿：草稿订单不能出货（数量还没算数）。
	res = call(t, srv, http.MethodPut,
		fmt.Sprintf("/api/v1/sales-orders/%d/status", orderID), token, []byte(`{"status":20}`))
	if res.StatusCode != http.StatusOK {
		_, msg, _ := decodeEnvelope(t, res)
		t.Fatalf("确认订单失败: %d %s", res.StatusCode, msg)
	}
	decodeEnvelope(t, res)

	t.Run("出货并确认，库存随之扣减", func(t *testing.T) {
		res := call(t, srv, http.MethodPost, "/api/v1/shipments", token,
			[]byte(fmt.Sprintf(`{"allocations":[{"sales_order_item_id":%d,"qty":"10"}]}`, itemID)))
		if res.StatusCode != http.StatusOK && res.StatusCode != http.StatusCreated {
			_, msg, _ := decodeEnvelope(t, res)
			t.Fatalf("建出货批次失败: %d %s", res.StatusCode, msg)
		}
		shipmentID := idOf(t, res)
		if shipmentID == 0 {
			t.Fatal("出货批次没有返回 id")
		}

		// 草稿 → 已确认 → 备货中：都还不该动库存。
		for _, status := range []int{20, 30} {
			res := call(t, srv, http.MethodPut,
				fmt.Sprintf("/api/v1/shipments/%d/status", shipmentID), token,
				[]byte(fmt.Sprintf(`{"status":%d}`, status)))
			if res.StatusCode != http.StatusOK {
				t.Fatalf("流转到 %d 失败: %d", status, res.StatusCode)
			}
			decodeEnvelope(t, res)
		}

		// 备货中 → 已出货：这一刻才出库。
		res = call(t, srv, http.MethodPut,
			fmt.Sprintf("/api/v1/shipments/%d/status", shipmentID), token,
			[]byte(`{"status":40}`))
		if res.StatusCode != http.StatusOK {
			t.Fatalf("确认出货失败: %d", res.StatusCode)
		}
		decodeEnvelope(t, res)

		// 库存为 -10：种子没有入库，出库只能把快照压到负数。
		res = call(t, srv, http.MethodGet,
			fmt.Sprintf("/api/v1/stock-snapshots?product_id=%d", productID), token, nil)
		_, _, snapData := decodeEnvelope(t, res)
		if !bytes.Contains(snapData, []byte("\"-10")) {
			t.Errorf("确认出货后库存应变为 -10，实际: %s", snapData)
		}
	})

	t.Run("重复确认出货被拒", func(t *testing.T) {
		// 两次确认同一票：第二次在状态机上就过不去（40 不能到 40），
		// 这正是"一票货只能出一次库"的第一道闸。
		res := call(t, srv, http.MethodGet, "/api/v1/shipments?page_size=1", token, nil)
		_, _, data := decodeEnvelope(t, res)
		var page struct {
			List []struct {
				ID uint `json:"id"`
			} `json:"list"`
		}
		if err := json.Unmarshal(data, &page); err != nil || len(page.List) == 0 {
			t.Fatalf("查不到出货批次: %s", data)
		}
		res = call(t, srv, http.MethodPut,
			fmt.Sprintf("/api/v1/shipments/%d/status", page.List[0].ID), token,
			[]byte(`{"status":40}`))
		defer res.Body.Close()
		if res.StatusCode != http.StatusConflict {
			t.Errorf("已出货再确认应 409，得到 %d", res.StatusCode)
		}
	})
}

// firstID 取列表第一条的 id（种子数据或刚建的数据都行）。
func firstID(t *testing.T, srv *httptest.Server, token, path string) uint {
	t.Helper()
	res := call(t, srv, http.MethodGet, path, token, nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET %s 失败: %d", path, res.StatusCode)
	}
	_, _, data := decodeEnvelope(t, res)
	var page struct {
		List []struct {
			ID uint `json:"id"`
		} `json:"list"`
	}
	if err := json.Unmarshal(data, &page); err != nil || len(page.List) == 0 {
		t.Fatalf("%s 没有数据: %s", path, data)
	}
	return page.List[0].ID
}

// idOf 从新建响应里取 id（单对象信封）。
func idOf(t *testing.T, res *http.Response) uint {
	t.Helper()
	_, _, data := decodeEnvelope(t, res)
	var obj struct {
		ID uint `json:"id"`
	}
	if err := json.Unmarshal(data, &obj); err != nil {
		t.Fatalf("响应里没有 id: %s", data)
	}
	return obj.ID
}
