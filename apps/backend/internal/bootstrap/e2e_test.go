package bootstrap_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/edwinhuish/gowork/internal/bootstrap"
	"github.com/edwinhuish/gowork/internal/config"
	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/server/middleware"
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
  # 关掉落盘：默认会写 data/cache.snapshot，于是幂等结果与会话跨测试、跨运行残留，
  # 用例变成"第一次跑过、第二次就不对"（幂等键命中上次的回放，数据一行没建）。
  # 生产上落盘是对的（重启后会话与幂等结果仍在），测试里必须隔离。
  memory:
    persist: false
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
			ID         uint   `json:"id"`
			AmountBase string `json:"amount_base"`
		} `json:"items"`
	}
	res = call(t, srv, http.MethodGet, fmt.Sprintf("/api/v1/sales-orders/%d", orderID), token, nil)
	_, _, data := decodeEnvelope(t, res)
	if err := json.Unmarshal(data, &order); err != nil || len(order.Items) == 0 {
		t.Fatalf("订单详情里没有明细: %s", data)
	}
	itemID := order.Items[0].ID
	itemAmountBase := order.Items[0].AmountBase

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

		// 金额一致性：整行 10 件全出，这票的收入应等于订单行的本币金额。
		// 报价 → 订单 → 出货 → 报表，汇率被锁在报价那一刻；只要中间任何一处
		// 重算过汇率或截过精度，这里就对不上。
		res = call(t, srv, http.MethodGet,
			"/api/v1/reports/shipment-profits?page_size=5", token, nil)
		_, _, profitData := decodeEnvelope(t, res)
		var profits struct {
			List []struct {
				ShipmentID uint   `json:"shipment_id"`
				Revenue    string `json:"revenue"`
			} `json:"list"`
		}
		if err := json.Unmarshal(profitData, &profits); err != nil {
			t.Fatalf("单票毛利不是预期格式: %s", profitData)
		}
		var revenue string
		for _, row := range profits.List {
			if row.ShipmentID == shipmentID {
				revenue = row.Revenue
			}
		}
		if revenue == "" {
			t.Fatalf("单票毛利里没有这一票: %s", profitData)
		}
		if !sameAmount(revenue, itemAmountBase) {
			t.Errorf("出货收入 %s 与订单行本币金额 %s 不一致（汇率或精度在某处被重算了）",
				revenue, itemAmountBase)
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

// sameAmount 比较两个定点金额字符串是否相等（容忍 1 分以内的舍入差）。
func sameAmount(a, b string) bool {
	x, errA := strconv.ParseFloat(a, 64)
	y, errB := strconv.ParseFloat(b, 64)
	if errA != nil || errB != nil {
		return false
	}
	d := x - y
	return d < 0.01 && d > -0.01
}

// 权限边界放到真实链路上验：单测用的是"假身份"，这里用的是真登录、真令牌、真中间件。
//
// T-32 的范围贯穿全部列表与详情，而它每一处都靠 service 里一句
// `filter.OwnerID = ownerScope(ctx)`；漏接一处的表现是"看不到应该看到的"
// 或"看得到不该看到的"，两种都不会报错。
func TestE2EScopeIsolation(t *testing.T) {
	app, srv := newE2EApp(t)
	adminToken := login(t, srv)

	// 建一个业务员账号：没有用户维护接口（刻意不做），所以在测试里直接落库。
	// 密码也真的走 bcrypt——绕过哈希就测不到登录那条路。
	hash, err := bcrypt.GenerateFromPassword([]byte("staff123"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("哈希密码失败: %v", err)
	}
	if err := app.DB.Create(&model.User{
		Username: "staff-e2e", Password: string(hash), Nickname: "业务员",
		Role: model.RoleStaff, Status: model.EnableStatusOn,
	}).Error; err != nil {
		t.Fatalf("建业务员失败: %v", err)
	}

	// 管理员名下一家、业务员名下一家，编码不同便于按关键字区分。
	adminPartner := createPartner(t, srv, adminToken, "ADM001", "管理员客户")
	staffToken := loginAs(t, srv, "staff-e2e", "staff123")
	staffPartner := createPartner(t, srv, staffToken, "STF001", "业务员客户")

	t.Run("业务员看不到别人的客户", func(t *testing.T) {
		res := call(t, srv, http.MethodGet, "/api/v1/partners?page_size=100", staffToken, nil)
		_, _, data := decodeEnvelope(t, res)
		if bytes.Contains(data, []byte("ADM001")) {
			t.Errorf("业务员看到了管理员的客户: %s", data)
		}
		if !bytes.Contains(data, []byte("STF001")) {
			t.Errorf("业务员看不到自己的客户: %s", data)
		}
	})

	t.Run("管理员看得到全部", func(t *testing.T) {
		res := call(t, srv, http.MethodGet, "/api/v1/partners?page_size=100", adminToken, nil)
		_, _, data := decodeEnvelope(t, res)
		for _, code := range []string{"ADM001", "STF001"} {
			if !bytes.Contains(data, []byte(code)) {
				t.Errorf("管理员应看得到 %s: %s", code, data)
			}
		}
	})

	t.Run("越权读详情返回 403 而不是 404", func(t *testing.T) {
		res := call(t, srv, http.MethodGet,
			fmt.Sprintf("/api/v1/partners/%d", adminPartner), staffToken, nil)
		defer res.Body.Close()
		if res.StatusCode != http.StatusForbidden {
			t.Errorf("读别人的客户应 403，得到 %d", res.StatusCode)
		}
	})

	t.Run("业务员不能转移负责人", func(t *testing.T) {
		res := call(t, srv, http.MethodPut,
			fmt.Sprintf("/api/v1/partners/%d/owner", staffPartner), staffToken,
			[]byte(`{"owner_id":0}`))
		defer res.Body.Close()
		if res.StatusCode != http.StatusForbidden {
			t.Errorf("业务员转移负责人应 403，得到 %d", res.StatusCode)
		}
	})

	t.Run("操作日志只有管理员能看", func(t *testing.T) {
		res := call(t, srv, http.MethodGet, "/api/v1/audit-logs?page_size=1", staffToken, nil)
		defer res.Body.Close()
		if res.StatusCode != http.StatusForbidden {
			t.Errorf("业务员看操作日志应 403，得到 %d", res.StatusCode)
		}
	})
}

func loginAs(t *testing.T, srv *httptest.Server, username, password string) string {
	t.Helper()
	body := fmt.Sprintf(`{"username":%q,"password":%q}`, username, password)
	res := call(t, srv, http.MethodPost, "/api/v1/auth/login", "", []byte(body))
	if res.StatusCode != http.StatusOK {
		t.Fatalf("%s 登录失败: %d", username, res.StatusCode)
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

func createPartner(t *testing.T, srv *httptest.Server, token, code, name string) uint {
	t.Helper()
	body := fmt.Sprintf(`{"code":%q,"name":%q,"role":1,"entity_type":1}`, code, name)
	res := call(t, srv, http.MethodPost, "/api/v1/partners", token, []byte(body))
	if res.StatusCode != http.StatusOK && res.StatusCode != http.StatusCreated {
		_, msg, _ := decodeEnvelope(t, res)
		t.Fatalf("建主体 %s 失败: %d %s", code, res.StatusCode, msg)
	}
	return idOf(t, res)
}

// 出入库单：草稿不动库存、确认才入账、确认后不可再确认。
//
// 这是库存唯一的"入口"，也是单池模型下唯一能解释"账上为什么是这个数"的地方。
// 之前的单测直接调 service，跳过了 HTTP 层的类型转换（`type` 是 int8、
// `qty` 是指针 Decimal），而这几处恰好是最容易在联调时出错的地方。
func TestE2EStockDocuments(t *testing.T) {
	_, srv := newE2EApp(t)
	token := login(t, srv)

	productID := firstID(t, srv, token, "/api/v1/products?page_size=1")

	// 建一张采购入库草稿：此刻库存不该有任何变化。
	body := fmt.Sprintf(`{
		"type": 10, "doc_date": "2026-10-29", "source_doc_no": "PO-E2E-1",
		"items": [{"product_id": %d, "qty": "100"}]
	}`, productID)
	res := call(t, srv, http.MethodPost, "/api/v1/stock-documents", token, []byte(body))
	if res.StatusCode != http.StatusOK && res.StatusCode != http.StatusCreated {
		_, msg, _ := decodeEnvelope(t, res)
		t.Fatalf("建入库单失败: %d %s", res.StatusCode, msg)
	}
	docID := idOf(t, res)

	snapshot := func() string {
		t.Helper()
		res := call(t, srv, http.MethodGet,
			fmt.Sprintf("/api/v1/stock-snapshots?product_id=%d", productID), token, nil)
		_, _, data := decodeEnvelope(t, res)
		return string(data)
	}

	if got := snapshot(); bytes.Contains([]byte(got), []byte("100")) {
		t.Errorf("草稿不该动库存，当前快照: %s", got)
	}

	t.Run("确认入库后库存为 100", func(t *testing.T) {
		res := call(t, srv, http.MethodPost,
			fmt.Sprintf("/api/v1/stock-documents/%d/confirm", docID), token, nil)
		if res.StatusCode != http.StatusOK {
			_, msg, _ := decodeEnvelope(t, res)
			t.Fatalf("确认入库失败: %d %s", res.StatusCode, msg)
		}
		decodeEnvelope(t, res)

		if got := snapshot(); !bytes.Contains([]byte(got), []byte("100")) {
			t.Errorf("确认后库存应为 100，实际: %s", got)
		}
	})

	t.Run("重复确认被拒", func(t *testing.T) {
		// 流水已经落地，再确认一次就等于把同一批货入库两遍。
		res := call(t, srv, http.MethodPost,
			fmt.Sprintf("/api/v1/stock-documents/%d/confirm", docID), token, nil)
		defer res.Body.Close()
		if res.StatusCode != http.StatusConflict {
			t.Errorf("重复确认应 409，得到 %d", res.StatusCode)
		}
	})

	t.Run("出库单把库存减到 90", func(t *testing.T) {
		body := fmt.Sprintf(`{
			"type": 20, "doc_date": "2026-10-29", "source_doc_no": "SH-E2E-1",
			"items": [{"product_id": %d, "qty": "10"}]
		}`, productID)
		res := call(t, srv, http.MethodPost, "/api/v1/stock-documents", token, []byte(body))
		if res.StatusCode != http.StatusOK && res.StatusCode != http.StatusCreated {
			_, msg, _ := decodeEnvelope(t, res)
			t.Fatalf("建出库单失败: %d %s", res.StatusCode, msg)
		}
		outID := idOf(t, res)

		res = call(t, srv, http.MethodPost,
			fmt.Sprintf("/api/v1/stock-documents/%d/confirm", outID), token, nil)
		if res.StatusCode != http.StatusOK {
			_, msg, _ := decodeEnvelope(t, res)
			t.Fatalf("确认出库失败: %d %s", res.StatusCode, msg)
		}
		decodeEnvelope(t, res)

		if got := snapshot(); !bytes.Contains([]byte(got), []byte("90")) {
			t.Errorf("出库 10 之后库存应为 90，实际: %s", got)
		}
	})
}

// 打印链路：报价单 PDF。
//
// 这条特别值得验，因为"没有中文字体"不会让服务起不来（代码里明确降级），
// 于是最坏的情况是：上线后点打印，界面弹一句看不懂的错，而没人知道是字体问题。
// 用例把两种可接受的结果都钉住——能打印就断言是真 PDF；不能打印就必须是
// 一句能照着做的提示，而不是 500 或者空文件。
func TestE2EPdf(t *testing.T) {
	_, srv := newE2EApp(t)
	token := login(t, srv)

	productID := firstID(t, srv, token, "/api/v1/products?page_size=1")
	customerID := firstID(t, srv, token, "/api/v1/partners?role=1&page_size=1")
	supplierID := firstID(t, srv, token, "/api/v1/partners?role=2&page_size=1")

	// 报价要先有货源（没有成本就没法算毛利，服务层会拒）。
	res := call(t, srv, http.MethodPost, "/api/v1/product-sources", token,
		[]byte(fmt.Sprintf(`{"product_id":%d,"partner_id":%d,"purchase_price":"60","moq":1,"delivery_days":7}`,
			productID, supplierID)))
	if res.StatusCode != http.StatusOK && res.StatusCode != http.StatusCreated {
		_, msg, _ := decodeEnvelope(t, res)
		t.Fatalf("建货源失败: %d %s", res.StatusCode, msg)
	}
	decodeEnvelope(t, res)

	res = call(t, srv, http.MethodPost, "/api/v1/quotations", token,
		[]byte(fmt.Sprintf(`{"partner_id":%d,"trade_term":"FOB","currency_code":"USD",
			"valid_until":"2026-12-31","items":[{"product_id":%d,"qty":"10","unit_price":"100"}]}`,
			customerID, productID)))
	if res.StatusCode != http.StatusOK && res.StatusCode != http.StatusCreated {
		_, msg, _ := decodeEnvelope(t, res)
		t.Fatalf("建报价失败: %d %s", res.StatusCode, msg)
	}
	quotationID := idOf(t, res)

	res = call(t, srv, http.MethodGet,
		fmt.Sprintf("/api/v1/quotations/%d/pdf", quotationID), token, nil)
	defer res.Body.Close()

	if res.StatusCode == http.StatusOK {
		t.Log("本机有可用中文字体：走的是渲染分支")
		if ct := res.Header.Get("Content-Type"); !strings.Contains(ct, "application/pdf") {
			t.Errorf("Content-Type = %q，期望 application/pdf", ct)
		}
		if cd := res.Header.Get("Content-Disposition"); !strings.Contains(cd, "attachment") {
			t.Errorf("Content-Disposition = %q，期望附件下载", cd)
		}
		raw, err := io.ReadAll(res.Body)
		if err != nil {
			t.Fatalf("读 PDF 失败: %v", err)
		}
		if !bytes.HasPrefix(raw, []byte("%PDF")) {
			t.Errorf("返回的不是 PDF（前 8 字节 %q）", raw[:min(8, len(raw))])
		}
		// 一页带表格的报价单不可能只有几百字节；太小说明模板渲染出了空文档。
		if len(raw) < 1024 {
			t.Errorf("PDF 只有 %d 字节，像是渲染失败", len(raw))
		}
		return
	}

	// 不能打印时必须说清楚原因，而不是让用户对着 500 猜。
	t.Log("本机没有候选中文字体：走的是降级分支（渲染分支在有字体的机器上才会被执行）")
	_, msg, _ := decodeEnvelope(t, res)
	if !strings.Contains(msg, "字体") {
		t.Errorf("打印不可用时应提示字体未配置，实际：%d %q", res.StatusCode, msg)
	}
}

// 收款与核销：钱到账之后，应收的未结清额与账龄要跟着变。
//
// 这是"钱"的另一半——上一轮的主链验的是"货出去、毛利算对"，
// 这一段验的是"钱进来、应收销掉"。两者的共同点是：每一步都返回 200，
// 而数字错在哪一步只能靠对账发现。
func TestE2EPaymentAllocation(t *testing.T) {
	_, srv := newE2EApp(t)
	token := login(t, srv)

	customerID := firstID(t, srv, token, "/api/v1/partners?role=1&page_size=1")
	productID := firstID(t, srv, token, "/api/v1/products?page_size=1")
	supplierID := firstID(t, srv, token, "/api/v1/partners?role=2&page_size=1")

	// 先凑一张已确认的销售订单：应收的来源只能是销售订单。
	res := call(t, srv, http.MethodPost, "/api/v1/product-sources", token,
		[]byte(fmt.Sprintf(`{"product_id":%d,"partner_id":%d,"purchase_price":"60","moq":1,"delivery_days":7}`,
			productID, supplierID)))
	if res.StatusCode != http.StatusOK && res.StatusCode != http.StatusCreated {
		_, msg, _ := decodeEnvelope(t, res)
		t.Fatalf("建货源失败: %d %s", res.StatusCode, msg)
	}
	decodeEnvelope(t, res)

	res = call(t, srv, http.MethodPost, "/api/v1/quotations", token,
		[]byte(fmt.Sprintf(`{"partner_id":%d,"trade_term":"FOB","currency_code":"USD",
			"valid_until":"2026-12-31","items":[{"product_id":%d,"qty":"10","unit_price":"100"}]}`,
			customerID, productID)))
	if res.StatusCode != http.StatusOK && res.StatusCode != http.StatusCreated {
		_, msg, _ := decodeEnvelope(t, res)
		t.Fatalf("建报价失败: %d %s", res.StatusCode, msg)
	}
	quotationID := idOf(t, res)

	res = call(t, srv, http.MethodPut,
		fmt.Sprintf("/api/v1/quotations/%d/status", quotationID), token, []byte(`{"status":20}`))
	if res.StatusCode != http.StatusOK {
		_, msg, _ := decodeEnvelope(t, res)
		t.Fatalf("确认报价失败: %d %s", res.StatusCode, msg)
	}
	decodeEnvelope(t, res)

	res = call(t, srv, http.MethodPost,
		fmt.Sprintf("/api/v1/quotations/%d/convert", quotationID), token,
		[]byte(`{"order_date":"2026-10-01"}`))
	if res.StatusCode != http.StatusOK && res.StatusCode != http.StatusCreated {
		_, msg, _ := decodeEnvelope(t, res)
		t.Fatalf("转订单失败: %d %s", res.StatusCode, msg)
	}
	orderID := idOf(t, res)

	res = call(t, srv, http.MethodPut,
		fmt.Sprintf("/api/v1/sales-orders/%d/status", orderID), token, []byte(`{"status":20}`))
	if res.StatusCode != http.StatusOK {
		_, msg, _ := decodeEnvelope(t, res)
		t.Fatalf("确认订单失败: %d %s", res.StatusCode, msg)
	}
	decodeEnvelope(t, res)

	// 开一张应收：金额留 0 = 把来源的未开票余额全开出来（1000 USD）。
	res = call(t, srv, http.MethodPost, "/api/v1/receivables", token,
		[]byte(fmt.Sprintf(`{"source_type":10,"source_id":%d,"due_date":"2026-10-20"}`, orderID)))
	if res.StatusCode != http.StatusOK && res.StatusCode != http.StatusCreated {
		_, msg, _ := decodeEnvelope(t, res)
		t.Fatalf("开应收失败: %d %s", res.StatusCode, msg)
	}
	receivableID := idOf(t, res)

	receivable := func() (amount, settled string) {
		t.Helper()
		res := call(t, srv, http.MethodGet,
			fmt.Sprintf("/api/v1/receivables/%d", receivableID), token, nil)
		if res.StatusCode != http.StatusOK {
			t.Fatalf("查应收失败: %d", res.StatusCode)
		}
		_, _, data := decodeEnvelope(t, res)
		var m struct {
			Amount        string `json:"amount"`
			SettledAmount string `json:"settled_amount"`
		}
		if err := json.Unmarshal(data, &m); err != nil {
			t.Fatalf("应收不是预期格式: %s", data)
		}
		return m.Amount, m.SettledAmount
	}

	amount, settled := receivable()
	if !sameAmount(amount, "1000") || !sameAmount(settled, "0") {
		t.Fatalf("应收应为 1000 且未核销，实际 %s / %s", amount, settled)
	}

	t.Run("收款 300 核销到这张应收", func(t *testing.T) {
		// 到账日取"今天"而不是写死某一天：汇率 fixture 插的是当天，
		// 写死日期会让这个测试在第二天全线变红——它验的是核销，不是日期。
		body := fmt.Sprintf(`{
			"direction": 10, "partner_id": %d, "pay_date": "%s",
			"currency_code": "USD", "amount": "300", "method": "TT",
			"allocations": [{"target_type": 10, "target_id": %d, "amount": "300"}]
		}`, customerID, time.Now().Format("2006-01-02"), receivableID)
		res := call(t, srv, http.MethodPost, "/api/v1/payments", token, []byte(body))
		if res.StatusCode != http.StatusOK && res.StatusCode != http.StatusCreated {
			_, msg, _ := decodeEnvelope(t, res)
			t.Fatalf("收款失败: %d %s", res.StatusCode, msg)
		}
		decodeEnvelope(t, res)

		_, settled := receivable()
		if !sameAmount(settled, "300") {
			t.Errorf("核销后已收应为 300，实际 %s", settled)
		}
	})

	t.Run("超额核销被拒", func(t *testing.T) {
		// 只剩 700 可核销，收 800 等于替客户多记了一笔回款。
		// 到账日同样取"今天"：这个子测验的是核销上限，不是汇率表里有哪几天。
		body := fmt.Sprintf(`{
			"direction": 10, "partner_id": %d, "pay_date": "%s",
			"currency_code": "USD", "amount": "800", "method": "TT",
			"allocations": [{"target_type": 10, "target_id": %d, "amount": "800"}]
		}`, customerID, time.Now().Format("2006-01-02"), receivableID)
		res := call(t, srv, http.MethodPost, "/api/v1/payments", token, []byte(body))
		defer res.Body.Close()
		if res.StatusCode == http.StatusOK || res.StatusCode == http.StatusCreated {
			t.Fatal("超额核销不该成功")
		}
		if _, settled := receivable(); !sameAmount(settled, "300") {
			t.Errorf("失败之后不该改已收金额，实际 %s", settled)
		}
	})

	// 收款要折本币，所以必须有**那天**的汇率。这条规则写在服务层，
	// 而"待办"页的"缺当日汇率"提醒正是为它服务的——这里把两头连起来验一次。
	t.Run("没有当日汇率不能收款", func(t *testing.T) {
		body := fmt.Sprintf(`{
			"direction": 10, "partner_id": %d, "pay_date": "2026-06-01",
			"currency_code": "USD", "amount": "100", "method": "TT"
		}`, customerID)
		res := call(t, srv, http.MethodPost, "/api/v1/payments", token, []byte(body))
		defer res.Body.Close()
		if res.StatusCode == http.StatusOK || res.StatusCode == http.StatusCreated {
			t.Fatal("缺汇率时不该收款成功")
		}
		if _, msg, _ := decodeEnvelope(t, res); !strings.Contains(msg, "汇率") {
			t.Errorf("提示应指出缺汇率，实际 %q", msg)
		}
	})

	t.Run("回款报表认这笔钱", func(t *testing.T) {
		res := call(t, srv, http.MethodGet,
			"/api/v1/reports/collection?direction=10&from=2026-10-01&to=2026-10-31", token, nil)
		if res.StatusCode != http.StatusOK {
			_, msg, _ := decodeEnvelope(t, res)
			t.Fatalf("取回款报表失败: %d %s", res.StatusCode, msg)
		}
		_, _, data := decodeEnvelope(t, res)
		if !bytes.Contains(data, []byte(`"received_base"`)) {
			t.Errorf("回款报表缺少 received_base: %s", data)
		}
	})
}

// 主体导入：两阶段（先报告后落库）、失败行带行号、重导不会重复建。
//
// 这一段走 multipart，而之前的单测是直接调 service 传结构体——文件解析、
// BOM 剥离、表头映射、空行跳过这些都在 HTTP 这一侧，出错了只会表现成
// "导进去的东西少了几行"。
func TestE2EPartnerImport(t *testing.T) {
	_, srv := newE2EApp(t)
	token := login(t, srv)

	// 第二行故意缺名称：它必须出现在失败行里，且行号指向文件里的第 3 行。
	csvBody := "\ufeff编码,名称,角色,国家/地区\n" +
		"IMP001,导入客户甲,customer,智利\n" +
		"IMP002,,customer,智利\n"

	type report struct {
		DryRun   bool `json:"dry_run"`
		Total    int  `json:"total"`
		Created  int  `json:"created"`
		Failures []struct {
			Line   int    `json:"line"`
			Reason string `json:"reason"`
		} `json:"failures"`
	}

	upload := func(dryRun bool) report {
		t.Helper()
		var buf bytes.Buffer
		w := multipart.NewWriter(&buf)
		part, err := w.CreateFormFile("file", "partners.csv")
		if err != nil {
			t.Fatalf("构造 multipart 失败: %v", err)
		}
		if _, err := part.Write([]byte(csvBody)); err != nil {
			t.Fatalf("写文件内容失败: %v", err)
		}
		if err := w.Close(); err != nil {
			t.Fatalf("关闭 multipart 失败: %v", err)
		}

		url := fmt.Sprintf("%s/api/v1/imports/partners?dry_run=%t", srv.URL, dryRun)
		req, err := http.NewRequest(http.MethodPost, url, &buf)
		if err != nil {
			t.Fatalf("构造请求失败: %v", err)
		}
		req.Header.Set("Content-Type", w.FormDataContentType())
		req.Header.Set("Authorization", "Bearer "+token)
		res, err := srv.Client().Do(req)
		if err != nil {
			t.Fatalf("上传失败: %v", err)
		}
		if res.StatusCode != http.StatusOK {
			_, msg, _ := decodeEnvelope(t, res)
			t.Fatalf("导入失败: %d %s", res.StatusCode, msg)
		}
		_, _, data := decodeEnvelope(t, res)
		var out report
		if err := json.Unmarshal(data, &out); err != nil {
			t.Fatalf("报告不是预期格式: %s", data)
		}
		return out
	}

	count := func(keyword string) int {
		t.Helper()
		res := call(t, srv, http.MethodGet,
			"/api/v1/partners?page_size=50&keyword="+keyword, token, nil)
		_, _, data := decodeEnvelope(t, res)
		var page struct {
			List []struct{ ID uint } `json:"list"`
		}
		if err := json.Unmarshal(data, &page); err != nil {
			t.Fatalf("列表不是预期格式: %s", data)
		}
		return len(page.List)
	}

	t.Run("校验阶段不落库", func(t *testing.T) {
		r := upload(true)
		if !r.DryRun {
			t.Error("报告应标明这是 dry_run")
		}
		if r.Created != 0 {
			t.Errorf("校验阶段不该建数据，created=%d", r.Created)
		}
		if r.Total != 2 {
			t.Errorf("应读到 2 行数据，得到 %d", r.Total)
		}
		if len(r.Failures) != 1 {
			t.Fatalf("缺名称那行应报失败，得到 %+v", r.Failures)
		}
		if r.Failures[0].Line != 3 {
			t.Errorf("行号应指向文件第 3 行，得到 %d", r.Failures[0].Line)
		}
		if n := count("IMP001"); n != 0 {
			t.Errorf("校验阶段不该把数据写进去，查到 %d 条", n)
		}
	})

	t.Run("确认后才落库，失败行不连累其他行", func(t *testing.T) {
		r := upload(false)
		if r.Created != 1 {
			t.Errorf("应只建成 1 家（另一行缺名称），created=%d", r.Created)
		}
		if n := count("IMP001"); n != 1 {
			t.Errorf("导入的那家应能查到，得到 %d 条", n)
		}
		if n := count("IMP002"); n != 0 {
			t.Errorf("失败行不该被建出来，查到 %d 条", n)
		}
	})

	t.Run("重导同一份文件不会建重复", func(t *testing.T) {
		r := upload(false)
		if r.Created != 0 {
			t.Errorf("编码已存在，不该再建，created=%d", r.Created)
		}
		// 两行都失败：一行编码已存在、一行缺名称。
		if len(r.Failures) != 2 {
			t.Errorf("应报 2 条失败，得到 %+v", r.Failures)
		}
		if n := count("IMP001"); n != 1 {
			t.Errorf("重导之后仍应只有 1 家，得到 %d", n)
		}
	})

	t.Run("缺少必需列时拒绝整份文件", func(t *testing.T) {
		var buf bytes.Buffer
		w := multipart.NewWriter(&buf)
		part, _ := w.CreateFormFile("file", "bad.csv")
		_, _ = part.Write([]byte("名称,角色\n没有编码列,customer\n"))
		_ = w.Close()

		url := srv.URL + "/api/v1/imports/partners?dry_run=true"
		req, _ := http.NewRequest(http.MethodPost, url, &buf)
		req.Header.Set("Content-Type", w.FormDataContentType())
		req.Header.Set("Authorization", "Bearer "+token)
		res, err := srv.Client().Do(req)
		if err != nil {
			t.Fatalf("上传失败: %v", err)
		}
		defer res.Body.Close()
		if res.StatusCode == http.StatusOK {
			t.Fatal("缺「编码」列时不该成功")
		}
		if _, msg, _ := decodeEnvelope(t, res); !strings.Contains(msg, "编码") {
			t.Errorf("提示应指出缺哪一列，得到 %q", msg)
		}
	})
}

// 幂等键（T-46）：同一个「意图」重复提交只落一次库，换一个键就是新意图。
//
// 这条以前只在一次手工验证里验过（起真进程 + python 脚本），没进测试套件。
// 而幂等一旦失效，表现是"多了一张单"，事后只能靠人删——前端每个新建页
// 都挂着这个键，失效一次会连着所有页面一起失效。
func TestE2EIdempotency(t *testing.T) {
	_, srv := newE2EApp(t)
	token := login(t, srv)

	body := []byte(`{"code":"IDEM001","name":"幂等测试客户","role":1,"entity_type":1}`)

	send := func(key string) (*http.Response, []byte) {
		t.Helper()
		req, err := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/partners", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("构造请求失败: %v", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set(middleware.IdempotencyKeyHeader, key)
		res, err := srv.Client().Do(req)
		if err != nil {
			t.Fatalf("请求失败: %v", err)
		}
		defer res.Body.Close()
		raw, _ := io.ReadAll(res.Body)
		return res, raw
	}

	first, firstBody := send("e2e-key-1")
	if first.StatusCode != http.StatusOK && first.StatusCode != http.StatusCreated {
		t.Fatalf("首次提交失败: %d %s", first.StatusCode, firstBody)
	}
	if first.Header.Get(middleware.IdempotentReplayHeader) != "" {
		t.Error("首次提交不该带重放标记")
	}

	t.Run("同一个键再发一次是回放，不会建第二家", func(t *testing.T) {
		second, secondBody := send("e2e-key-1")
		if second.Header.Get(middleware.IdempotentReplayHeader) != "true" {
			t.Errorf("重复提交应带 Idempotent-Replay: true，实际 %q",
				second.Header.Get(middleware.IdempotentReplayHeader))
		}
		if second.StatusCode != first.StatusCode {
			t.Errorf("回放的状态码应与首次一致：%d vs %d", second.StatusCode, first.StatusCode)
		}
		if !bytes.Equal(firstBody, secondBody) {
			t.Errorf("回放应返回与首次相同的响应体\nfirst:  %s\nsecond: %s", firstBody, secondBody)
		}
		if n := countByKeyword(t, srv, token, "IDEM001"); n != 1 {
			t.Errorf("同一个键重复提交后应只有 1 家，得到 %d", n)
		}
	})

	t.Run("换一个键就是新意图，靠唯一约束拦住重复", func(t *testing.T) {
		// 不是幂等键挡住了它，而是编码唯一——两种保护分别覆盖不同的错误：
		// 键挡"网络抖动后的重试"，唯一索引挡"真的重复录入"。
		third, thirdBody := send("e2e-key-2")
		defer third.Body.Close()
		if third.StatusCode != http.StatusConflict {
			t.Errorf("换键重复录入应 409，得到 %d（%s）", third.StatusCode, thirdBody)
		}
		if n := countByKeyword(t, srv, token, "IDEM001"); n != 1 {
			t.Errorf("仍应只有 1 家，得到 %d", n)
		}
	})
}

// countByKeyword 按关键字数主体条数。
func countByKeyword(t *testing.T, srv *httptest.Server, token, keyword string) int {
	t.Helper()
	res := call(t, srv, http.MethodGet,
		"/api/v1/partners?page_size=50&keyword="+keyword, token, nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("查列表失败: %d", res.StatusCode)
	}
	_, _, data := decodeEnvelope(t, res)
	var page struct {
		List []struct{ ID uint } `json:"list"`
	}
	if err := json.Unmarshal(data, &page); err != nil {
		t.Fatalf("列表不是预期格式: %s", data)
	}
	return len(page.List)
}

// TestE2EStatementPDF 走完整链路：一张已确认订单 → 应收 → 客户对账单 JSON 与 PDF。
//
// 重点不是 PDF 长什么样，而是三件事对得上：
// 1. 路由真的挂上了（`/reports/receivable-statement/pdf` 与 JSON 版同一入参）；
// 2. 纸面上的期末余额与 JSON 里的 `closing` 是同一个数——两处各算一遍迟早差一分；
// 3. 没有中文字体时说明原因，而不是给一张空白纸。
func TestE2EStatementPDF(t *testing.T) {
	_, srv := newE2EApp(t)
	token := login(t, srv)

	customerID := firstID(t, srv, token, "/api/v1/partners?role=1&page_size=1")
	productID := firstID(t, srv, token, "/api/v1/products?page_size=1")
	supplierID := firstID(t, srv, token, "/api/v1/partners?role=2&page_size=1")

	res := call(t, srv, http.MethodPost, "/api/v1/product-sources", token,
		[]byte(fmt.Sprintf(`{"product_id":%d,"partner_id":%d,"purchase_price":"60","moq":1,"delivery_days":7}`,
			productID, supplierID)))
	if res.StatusCode != http.StatusOK && res.StatusCode != http.StatusCreated {
		_, msg, _ := decodeEnvelope(t, res)
		t.Fatalf("建货源失败: %d %s", res.StatusCode, msg)
	}
	decodeEnvelope(t, res)

	res = call(t, srv, http.MethodPost, "/api/v1/quotations", token,
		[]byte(fmt.Sprintf(`{"partner_id":%d,"trade_term":"FOB","currency_code":"USD",
			"valid_until":"2026-12-31","items":[{"product_id":%d,"qty":"10","unit_price":"100"}]}`,
			customerID, productID)))
	if res.StatusCode != http.StatusOK && res.StatusCode != http.StatusCreated {
		_, msg, _ := decodeEnvelope(t, res)
		t.Fatalf("建报价失败: %d %s", res.StatusCode, msg)
	}
	quotationID := idOf(t, res)

	res = call(t, srv, http.MethodPut,
		fmt.Sprintf("/api/v1/quotations/%d/status", quotationID), token, []byte(`{"status":20}`))
	if res.StatusCode != http.StatusOK {
		_, msg, _ := decodeEnvelope(t, res)
		t.Fatalf("确认报价失败: %d %s", res.StatusCode, msg)
	}
	decodeEnvelope(t, res)

	res = call(t, srv, http.MethodPost,
		fmt.Sprintf("/api/v1/quotations/%d/convert", quotationID), token,
		[]byte(`{"order_date":"2026-10-01"}`))
	if res.StatusCode != http.StatusOK && res.StatusCode != http.StatusCreated {
		_, msg, _ := decodeEnvelope(t, res)
		t.Fatalf("转订单失败: %d %s", res.StatusCode, msg)
	}
	orderID := idOf(t, res)

	res = call(t, srv, http.MethodPut,
		fmt.Sprintf("/api/v1/sales-orders/%d/status", orderID), token, []byte(`{"status":20}`))
	if res.StatusCode != http.StatusOK {
		_, msg, _ := decodeEnvelope(t, res)
		t.Fatalf("确认订单失败: %d %s", res.StatusCode, msg)
	}
	decodeEnvelope(t, res)

	res = call(t, srv, http.MethodPost, "/api/v1/receivables", token,
		[]byte(fmt.Sprintf(`{"source_type":10,"source_id":%d,"due_date":"2026-10-20"}`, orderID)))
	if res.StatusCode != http.StatusOK && res.StatusCode != http.StatusCreated {
		_, msg, _ := decodeEnvelope(t, res)
		t.Fatalf("开应收失败: %d %s", res.StatusCode, msg)
	}
	receivableID := idOf(t, res)

	// 再收 400：对账单要能看出「开票 1000、收 400、还剩 600」。
	// 有收有付才验得到余额列的累加——只有一张应收的话，那列等于原样抄一遍。
	body := fmt.Sprintf(`{
		"direction": 10, "partner_id": %d, "pay_date": "%s",
		"currency_code": "USD", "amount": "400", "method": "TT",
		"allocations": [{"target_type": 10, "target_id": %d, "amount": "400"}]
	}`, customerID, time.Now().Format("2006-01-02"), receivableID)
	res = call(t, srv, http.MethodPost, "/api/v1/payments", token, []byte(body))
	if res.StatusCode != http.StatusOK && res.StatusCode != http.StatusCreated {
		_, msg, _ := decodeEnvelope(t, res)
		t.Fatalf("收款失败: %d %s", res.StatusCode, msg)
	}
	decodeEnvelope(t, res)

	// 先取 JSON：PDF 的期末必须与它逐字一致。
	args := fmt.Sprintf("partner_id=%d&currency_code=USD", customerID)
	res = call(t, srv, http.MethodGet, "/api/v1/reports/receivable-statement?"+args, token, nil)
	if res.StatusCode != http.StatusOK {
		_, msg, _ := decodeEnvelope(t, res)
		t.Fatalf("查对账单失败: %d %s", res.StatusCode, msg)
	}
	_, _, data := decodeEnvelope(t, res)
	var st struct {
		Currency string `json:"currency_code"`
		Opening  string `json:"opening"`
		Debit    string `json:"period_debit"`
		Closing  string `json:"closing"`
		Lines    []struct {
			Kind  string `json:"kind"`
			DocNo string `json:"doc_no"`
		} `json:"lines"`
	}
	if err := json.Unmarshal(data, &st); err != nil {
		t.Fatalf("对账单不是预期格式: %s", data)
	}
	if st.Currency != "USD" {
		t.Fatalf("币种 = %q，期望 USD", st.Currency)
	}
	// 两笔：一张应收、一笔收款。少一笔说明有一条来源没被算进来。
	if len(st.Lines) != 2 {
		t.Fatalf("对账单应有 2 笔发生额（开票 + 收款），实际 %d 笔：%+v", len(st.Lines), st.Lines)
	}
	// 同一天先开票后收款：否则余额列会先跳到 −400 再回到 600。
	if st.Lines[0].Kind != "receivable" || st.Lines[1].Kind != "payment" {
		t.Errorf("同日发生额顺序应为「应收 → 收款」，实际 %v", []string{st.Lines[0].Kind, st.Lines[1].Kind})
	}
	// 金额序列化成字符串时不会补零（600 而不是 600.00），按数值比。
	if got, err := strconv.ParseFloat(st.Closing, 64); err != nil || got != 600 {
		t.Errorf("期末应为 600（1000 开票 − 400 收款），实际 %s（期初 %s + 本期 %s）",
			st.Closing, st.Opening, st.Debit)
	}

	res = call(t, srv, http.MethodGet, "/api/v1/reports/receivable-statement/pdf?"+args, token, nil)
	defer res.Body.Close()

	if res.StatusCode == http.StatusOK {
		t.Log("本机有可用中文字体：走的是渲染分支")
		if ct := res.Header.Get("Content-Type"); !strings.Contains(ct, "application/pdf") {
			t.Errorf("Content-Type = %q，期望 application/pdf", ct)
		}
		if cd := res.Header.Get("Content-Disposition"); !strings.Contains(cd, "attachment") {
			t.Errorf("Content-Disposition = %q，期望附件下载", cd)
		}
		raw, err := io.ReadAll(res.Body)
		if err != nil {
			t.Fatalf("读 PDF 失败: %v", err)
		}
		if !bytes.HasPrefix(raw, []byte("%PDF")) {
			t.Errorf("返回的不是 PDF（前 8 字节 %q）", raw[:min(8, len(raw))])
		}
		// 带表头、明细与合计的一页对账单不可能只有几百字节。
		if len(raw) < 1024 {
			t.Errorf("PDF 只有 %d 字节，像是渲染失败", len(raw))
		}
		return
	}

	t.Log("本机没有候选中文字体：走的是降级分支（渲染分支在有字体的机器上才会被执行）")
	_, msg, _ := decodeEnvelope(t, res)
	if !strings.Contains(msg, "字体") {
		t.Errorf("打印不可用时应提示字体未配置，实际：%d %q", res.StatusCode, msg)
	}
}
