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

### 4. 缓存不可用时降级，而不是启动失败

`infra/cache.New` 先 ping Redis，失败时按 `redis.fallback_to_memory` 决定：

- `true`（开发默认）：降级为进程内实现，服务照常启动
- `false`（生产）：直接返回错误，宁可起不来也不要带着错误假设运行

上层的 `Cache` 接口不变，业务代码不需要知道自己连的是哪种实现。

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

## 五、新增一个业务模块

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

## 六、后续模块的扩展建议

| 模块 | 关键点 |
|---|---|
| 采购订单 | 主表 + 明细表（`biz_purchase_order_item`），金额在 service 里汇总，不允许前端提交总额 |
| 销售订单 | 同上；订单状态用枚举 + 状态机，禁止任意跳转 |
| 库存 | 用「出入库流水 + 当前库存快照」两张表，快照可由流水重算，便于对账 |
| 出货批次 | 出货单与订单是多对多（分批/合并），必须有分配表，不要试图把订单号编进编码 |
| SKU 货源 | 一个产品对多个供应商货号，单独建 `md_product_source`，不要往产品表塞 `supplier_id2` |

## 七、前端分层

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
