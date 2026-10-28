# 架构说明

## 一、分层与依赖方向

```
handler  →  service  →  repository  →  model
   ↓           ↓            ↓
 gin         dto          gorm
```

依赖只向下，**没有反向引用**。具体表现：

- `service` 不认识 gin，也不写 SQL
- `repository` 不认识 dto，也不做业务判断
- `model` 不依赖任何上层，只有字段定义
- `handler` 不做业务判断，只做参数绑定 + 调用 + 写响应

接口定义在**被依赖方**（`repository` 包导出 `ProductRepository` 接口，`service` 包导出 `ProductService` 接口），上层持有接口，实现可以整体替换。

## 二、各层职责

| 目录 | 职责 | 不该出现的东西 |
|---|---|---|
| `cmd/server` | 解析参数、启动、优雅退出 | 业务代码 |
| `internal/config` | 读配置、默认值、环境变量覆盖、启动期校验 | 业务逻辑 |
| `internal/bootstrap` | 依赖装配（DB/缓存/仓储/服务/handler 串联） | 业务逻辑 |
| `internal/model` | 表结构、表名、状态常量 | 查询、校验 |
| `internal/dto` | 请求体、查询条件、分页结果 | 数据库字段映射 |
| `internal/repository` | 查询构造、事务、软删除 | 业务规则、HTTP 概念 |
| `internal/service` | 业务规则、唯一性校验、编排多个仓储 | SQL、gin.Context |
| `internal/handler` | 参数绑定与校验、调 service、写响应 | 业务判断 |
| `internal/server` | 路由表、中间件链 | 具体业务 |
| `internal/infra` | 数据库/缓存/日志的初始化 | 业务 |
| `internal/pkg` | 统一错误码、统一响应体 | 依赖上层 |

## 三、一个请求的完整路径

以 `GET /api/v1/products?keyword=hinge` 为例：

```
gin Engine
  └─ RequestID       生成/透传 X-Request-ID
  └─ AccessLog       记录耗时（c.Next 之后再写）
  └─ Recovery        兜住 panic
  └─ CORS
  └─ Auth            解析 Bearer → 校验签名 → 校验会话 → 注入 Principal
  └─ ProductHandler.List
       ├─ bindQuery 绑定 dto.ProductQuery 并校验
       ├─ ProductService.List
       │    ├─ PageQuery.Normalize()  修正非法分页
       │    ├─ 组装 repository.ProductFilter / Pagination
       │    └─ ProductRepository.List → Base.Paginate
       └─ response.OK 包成统一结构
```

出错时任何一层返回 `error`，`response.Fail` 统一转成 `{code, message}`，`code` 前三位决定 HTTP 状态码。

## 四、几个设计决策

### 1. 泛型仓储基类

`repository.Base[T]` 用 Go 泛型承载了 Create / UpdateFields / DeleteByID / FindByID / Exists / Count / Paginate 七个通用方法，具体仓储只需嵌入它并补自己的独有查询：

```go
type productRepo struct {
    *Base[model.Product]
}
```

新增一个实体时省掉的不是几行代码，而是"分页 + 计数 + 软删 + NotFound 转换"这套容易写错、写漏的逻辑。

### 2. 查询条件用 `Scope` 而不是往基类塞参数

`Scope` 是 gorm 条件构造器的类型别名：

```go
type Scope = func(*gorm.DB) *gorm.DB
```

用**类型别名**而非命名类型，是为了能直接展开传给 `db.Scopes(...)`。
各模块的 Filter 自己实现 `scopes()`，基类完全不知道业务字段，保持通用。

### 3. 计数与取数分别构建查询

`Base.Paginate` 里 `Count` 和 `Find` 各自重新构建 `*gorm.DB`。复用同一个实例会让 Count 的 statement（select count(*)）残留到 Find 上，这是 gorm 最常见的坑之一。

### 4. 缓存与数据库都靠配置选实现，代码里没有分支

`infra/cache.New` 和 `infra/database.Open` 都只做一件事：读 `driver` 字段，返回对应实现。

- `cache.driver`：`memory`（默认，零依赖）/ `redis`
- `database.driver`：`sqlite`（默认，零依赖）/ `postgres` / `mysql`

**不采用"连不上就悄悄降级"的隐式规则。** 只有显式把 `cache.fallback_to_memory` 打开时才降级，
且生产配置里它是 `false` —— 多实例部署下退回进程内缓存，症状是"数据时有时无"，
这种错误不会在启动时暴露，只会在业务对不上账时才被发现。

详细矩阵见第五节。

### 5. Option 模式注入可选依赖

```go
service.NewProductService(repo, service.WithProductLogger(log))
```

必填依赖走位置参数（编译器强制），可选依赖走 Option（签名叫得清、加参数不破坏调用方）。

### 6. 编辑走全量覆盖

`dto.ProductInput` 同时用于新增与编辑，编辑按 PUT 语义全量覆盖。
理由：前端表单本来就提交全部可见字段，维护两套几乎相同的校验规则（Create 用值类型、Update 用指针）只会让它们逐渐漂移。

仓储层仍然用 `UpdateFields(map[string]any)` 而不是 `Save`，避免零值字段被意外写成 NULL。

### 7. 软删除

所有模型嵌入 `gorm.DeletedAt`。ERP 里删除客户或产品不能真的删——历史订单、对账、追溯都还引用着它。删除只置位，查询自动过滤。

## 五、存储可替换性：三库 + 两缓存

### 1. 组合矩阵

| 场景 | `database.driver` | `cache.driver` | 外部依赖 |
|---|---|---|---|
| 本地开发（默认 `config.yaml`） | `sqlite` | `memory` + 落盘 | 无 |
| 本地换库验证（`config.postgres.yaml` / `config.mysql.yaml`） | `postgres` / `mysql` | `redis` | compose |
| 生产（`config.prod.yaml`） | `postgres` | `redis` | 有 |

切换只改配置，不改一行 Go 代码：

```bash
GOWORK_DB_DRIVER=postgres \
GOWORK_DB_HOST=10.0.0.5 GOWORK_DB_USER=gowork GOWORK_DB_PASSWORD=*** GOWORK_DB_NAME=gowork \
GOWORK_CACHE_DRIVER=redis GOWORK_REDIS_ADDR=10.0.0.6:6379 \
task backend:run
```

### 2. 数据库：连接串可以整段给，也可以拆开给

```yaml
database:
  driver: postgres
  host: 127.0.0.1
  port: 5432
  user: gowork
  password: ""            # 走 GOWORK_DB_PASSWORD
  name: gowork
  params: {sslmode: disable, TimeZone: Asia/Shanghai}
```

`postgres` 与 `mysql` **共用同一组键名**，所以换库时只有 `driver` 和 `params` 的写法要变：
postgres 的 params 是 `key=value`，mysql 的是 query string。想完全掌控连接串就直接写 `dsn`，此时其余字段被忽略。

驱动差异全部收敛在 `infra/database`：

| 关注点 | 处理方式 |
|---|---|
| 驱动选择 | `postgres.Open` / `mysql.Open` / `sqlite.Open`，返回同一个 `*gorm.DB` |
| 连接数 | sqlite 收敛到 ≤8（单写者模型），Server 型按配置 |
| 目录 | sqlite 的父目录不存在时自动创建 |
| 事务包装 | sqlite 关闭 gorm 默认事务，缩短写锁持有时间 |
| 日志 | `database.log_level` 统一控制 gorm 日志 |

### 3. 行为一致性：仓库层只写一次 SQL

| 语义 | 三库差异 | 统一做法 |
|---|---|---|
| 关键字搜索大小写 | pg 的 `LIKE` 区分大小写 | `likeAny` 在 pg 上用 `ILIKE` |
| 用户输入含 `%` `_` | 各库转义符不同 | 统一转义并 `ESCAPE '!'`（避开反斜杠在 mysql 字符串里的二次转义） |
| 时间 | pg/pgx、mysql parseTime、sqlite 文本 | 驱动层统一 `parseTime=True&loc=Local` |
| 软删除 | 一致 | `gorm.DeletedAt` |
| 自增主键 | bigserial / auto_increment / rowid | 统一 `uint` 主键，业务层不关心 |

`internal/repository/storage_test.go` 用同一套断言跑三种库（`task test:storage`），
任何一条方言差异导致的行为漂移都会在这里暴露。

### 4. 内存缓存的持久化边界

```yaml
cache:
  driver: memory
  memory:
    persist: true
    path: data/cache.snapshot
    save_interval: 30
```

机制：JSON 快照 → 同目录临时文件 → `fsync` → `rename` 原子替换；启动时加载并丢弃过期键；
写操作只置脏标记，由后台按 `save_interval` 合并落盘，`Close()` 再补一次。

必须清楚的四点：

1. **不是数据库。** 它只是让"重启后登录态还在"这件事成立，不能当成持久化存储来设计业务。
2. **有丢失窗口。** 进程被 `SIGKILL` 或掉电时，最后一次落盘之后的数据会丢（最多一个 `save_interval`）。
3. **单进程。** 两个实例各写各的快照文件，不共享、会互相覆盖。
4. **损坏不阻塞启动。** 快照解析失败会被改名成 `.corrupt` 并留日志，服务以空缓存启动。

`fallback_to_memory` 只影响 `driver=redis`：开发环境打开可以"没起 Redis 也能跑"，
生产必须关闭——降级后多实例之间的会话不再共享。

## 六、新增一个业务模块

以"采购订单 `purchase_order`"为例，按顺序加五层，最后装配一次：

### 1. model

```go
// internal/model/purchase_order.go
type PurchaseOrder struct {
    Base
    OrderNo    string  `gorm:"size:32;uniqueIndex;not null" json:"order_no"`
    SupplierID uint    `gorm:"index;not null" json:"supplier_id"`
    TotalAmount float64 `gorm:"type:decimal(14,2)" json:"total_amount"`
    Status     int8    `gorm:"not null;default:1;index" json:"status"`
}

func (PurchaseOrder) TableName() string { return "biz_purchase_order" }
```

在 `bootstrap/app.go` 的 `AutoMigrate` 列表里加上 `&model.PurchaseOrder{}`。

### 2. repository

```go
type PurchaseOrderRepository interface {
    Create(ctx context.Context, m *model.PurchaseOrder) error
    List(ctx context.Context, f PurchaseOrderFilter, p Pagination) ([]model.PurchaseOrder, int64, error)
}

type purchaseOrderRepo struct{ *Base[model.PurchaseOrder] }

func NewPurchaseOrderRepository(db *gorm.DB) PurchaseOrderRepository {
    return &purchaseOrderRepo{Base: NewBase[model.PurchaseOrder](db)}
}
```

### 3. dto

```go
type PurchaseOrderQuery struct {
    PageQuery
    SupplierID *uint `form:"supplier_id"`
}

type PurchaseOrderInput struct {
    OrderNo    string `json:"order_no" binding:"required,max=32"`
    SupplierID uint   `json:"supplier_id" binding:"required"`
}
```

### 4. service

```go
type PurchaseOrderService interface {
    Create(ctx context.Context, in dto.PurchaseOrderInput) (*model.PurchaseOrder, error)
    List(ctx context.Context, q dto.PurchaseOrderQuery) (*dto.PageResult[model.PurchaseOrder], error)
}

type purchaseOrderService struct {
    repo repository.PurchaseOrderRepository
    log  *slog.Logger
}

type PurchaseOrderOption func(*purchaseOrderService)

func NewPurchaseOrderService(repo repository.PurchaseOrderRepository, opts ...PurchaseOrderOption) PurchaseOrderService {
    s := &purchaseOrderService{repo: repo, log: slog.Default()}
    for _, opt := range opts {
        opt(s)
    }
    return s
}
```

### 5. handler + router

```go
type PurchaseOrderHandler struct {
    orders service.PurchaseOrderService
}
```

在 `server/router.go` 的 `Handlers` 结构里加字段，在 `secured` 分组下注册路由；
在 `bootstrap/app.go` 里 new 出 repo → service → handler 三件套。

### 检查清单

- [ ] 表名带模块前缀（`sys_` / `md_` / `biz_` / `inv_`），避免后续命名打架
- [ ] 编码类字段加 `uniqueIndex`，service 里做前置查重
- [ ] 列表接口统一走 `Pagination`，不要出现无上限的 `Find(&list)`
- [ ] service 不 import gin；handler 不 import gorm
- [ ] 新错误用 `errs` 里已有的码，确需新增时保持"前三位对齐 HTTP 状态码"的约定
- [ ] 自定义 SQL 必须三库兼容：模糊匹配用 `likeAny`，不要写 `IFNULL` / `group_concat` 这类单库函数

## 七、后续模块的扩展建议

| 模块 | 关键点 |
|---|---|
| 采购订单 | 主表 + 明细表（`biz_purchase_order_item`），金额在 service 里汇总，不允许前端提交总额 |
| 销售订单 | 同上；订单状态用枚举 + 状态机，禁止任意跳转 |
| 库存 | 用「出入库流水 + 当前库存快照」两张表，快照可由流水重算，便于对账 |
| 出货批次 | 出货单与订单是多对多（分批/合并），必须有分配表，不要试图把订单号编进编码 |
| SKU 货源 | 一个产品对多个供应商货号，单独建 `md_product_source`，不要往产品表塞 `supplier_id2` |

## 八、前端分层

```
pages/       页面：组装数据与交互
  ↓
hooks/       列表页状态机（分页 / 关键字 / 加载 / 错误 / 竞态）
  ↓
api/         接口封装 + 统一 fetch 客户端（令牌注入、错误拆包、401 清理）
  ↓
types/       与后端 dto 对齐的类型定义
```

- `components/` 只放无业务语义的通用件（Modal / Pagination / Field / StatusBadge）
- `store/` 只放跨页面共享的状态（当前登录用户），页面级状态留在页面里
- 所有请求走 `api/client.ts`，不在页面里直接 `fetch`——令牌注入和错误处理只应该有一处
