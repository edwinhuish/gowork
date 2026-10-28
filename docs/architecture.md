# 架构说明

**职责**：讲清楚每个设计**为什么**这么定，以及哪些地方可以换实现（三库两缓存）。
**不管**：怎么做（`apps/backend/AGENTS.md` 的五步与硬性规则）、有什么差距
（`docs/gap-analysis.md` 第五章）、下一步做什么（`docs/roadmap.md`）。
**相关**：`AGENTS.md` 第 3 节（依赖方向的一句话版）、`docs/glossary.md`（域模型）。

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
| `internal/model` | 表结构、表名、状态枚举 | 查询、校验 |
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

所有模型嵌入 `gorm.DeletedAt`。ERP 里删除往来单位或产品不能真的删——历史订单、对账、追溯都还引用着它。删除只置位，查询自动过滤。

### 8. 按层分包，不按域分包

ERP 与 CRM 是两个域，但代码不按域分目录——`internal/service/product.go` 不放进
`internal/service/erp/` 下。

理由是当前规模：主数据只有 product 与 partner 两个模型，而 partner 被两个域同时引用，
按域分反而要先给共享件找个第三个目录。域归属靠术语表与表前缀
（`md_` / `biz_` / `crm_`）表达，已经够清楚。

新增模块时也不用先判断"这属于哪个域"：五个文件走同一条路径，按第六节走一遍就行。

**什么时候该切**：CRM 侧模块超过三个，或出现只属于一侧的复杂逻辑（订单状态机、
库存扣减规则）。那时按域拆才有收益——拆的应该是"变化频率不同的东西"，
不是"名字不同但改动同步的东西"。

**现状与本条的裁决**：CRM 侧已有 contact / activity / lead / opportunity 四个模块，
条件按其自身标准已满足。但对「现在拆不拆」的结论是**暂不拆目录、改用 import 约束**，
理由与真正的触发条件写在 `docs/gap-analysis.md` 5.2，本节不重复。

### 9. 客户与供应商合成一张表，角色用位掩码

`md_customer` 与 `md_supplier` 字段几乎相同（客户只多一个 `country`）。分开建的
代价在一个很常见的场景里立刻显现：同一家公司既卖料件给我们、又买我们的成品。
两张表意味着两行数据、两套联系方式、两个编码，对账时还要人工确认它们是同一家。

合并成 `md_partner` 后用 `role` 区分。**用位掩码而不是 `1/2/3` 枚举**：角色是集合，
将来加货代、报关行时，连续枚举要补到 7 个值，而且每加一种都要回头改所有判断分支。

三个必须记住的推论：

| 推论 | 做错会怎样 |
|---|---|
| 判角色一律按位与（`role.Has(x)`、`(role & bit) !== 0`） | 写 `role === 1` 会把 `role=3` 判成"不是客户"，两个列表里都显示成未知角色 |
| 列表按角色筛选要展开成 `role IN (...)` | 直接 `WHERE role = 2` 会漏掉客供两用的主体；直接写位运算条件在 PG/MySQL 上都用不上索引 |
| 产品上的 `supplier_id` 指向 partner，不是"供应商表" | 保存产品必须校验该主体确实含 supplier 角色，否则"选了个客户当供应商"能写进库 |

兼容入口 `/customers` 与 `/suppliers` 是同一份数据的角色切片：同一个 handler，
路由层注入默认角色。它们不是两个资源，新增字段时只改一处。
用客户 id 访问 `/suppliers/:id` 返回 404 而不是 403，避免用状态码反推某个 id
属于哪种角色；通过这两个入口编辑也不允许改 role，否则在客户页面上改一次，
就能把"客供两用"主体的供应商身份抹掉，而那个页面上根本没有这个选项。

### 10. 结构同步与数据迁移分开

`AutoMigrate` 只让表结构追上模型：它加列、加索引，但不会搬数据、不会改名、
更不会在两张表之间建立对应关系。

凡是"已有数据的形状要变"，一律写在 `internal/migrate`，一份迁移一个函数，要求：

- **幂等**：`sys_migration` 记录已执行的迁移，重复启动不会重跑。
- **事务内完成**：搬数据、记 id 映射、重写引用三段要么都成功，要么都不留痕。
- **保留业务时间戳与软删除状态**：搬运不是业务操作，不该让所有记录看起来都是
  今天建的，也不该让已删除的数据复活。读旧表记得加 `Unscoped`——软删除条件由
  目标 struct 的 schema 决定，用 `Table()` 指定表名绕不过去，漏了就是静默少搬。
- **状态不明时中止**：目标表已有数据却没有迁移记录，说明上次中途断了或有人手工
  导过数。这时停下来报错，比"继续插入"造出一批难以识别的重复数据要好。
- **重编历史单号必须留新旧对照表**：改单号格式（存量 `SH-` → `DN-`、给历史单据补内部编号）
  也是"已有数据的形状要变"，走同一套迁移，不允许在业务代码里顺手改号。旧号 → 新号的映射
  要落进库里，因为客户手里的送货单、货代的提单副本印的都是旧号，半年后靠记忆对不上。

`md_customer` / `md_supplier` 在合并完成后**刻意保留不删**，留着回退用。
确认新表数据无误后可手工 `DROP`。迁移代码不替你删表——那一步不该由程序决定。

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

**机械步骤见 `apps/backend/AGENTS.md` 第 8 节**（model → repository → dto → service →
handler+router，最后在 `bootstrap/app.go` 装配一次），那节的「硬性规则」也是全项目唯一一份。
本节只讲**为什么**按这个顺序，以及步骤里最容易做错的三处。

**为什么顺序不能反**：repository 不认识 dto、service 不认识 gin，这条约束让「换 HTTP 框架」
与「换存储」互不牵连。反过来先写 handler，校验最终一定会落进 handler——因为它拿得到请求体，
而 service 那时还不存在。

**三处最容易做错的地方**：

| 位置 | 错法 | 后果 |
|---|---|---|
| model | 单据状态复用 `EnableStatus` | 两者都是小整数，编译器不拦，只在筛选条件上出错 |
| model | 金额自创第二套表示 | 跨模块汇总时「同一笔钱两个精度」；统一口径与迁移计划见 T-37 |
| repository | 把业务字段塞进泛型基类 | 基类不再通用，下一个模块要把这套逻辑再抄一遍 |

表前缀、编码唯一性、分页上限、三库兼容、错误码一律以 `apps/backend/AGENTS.md` 第 8 节为准，
本节不再抄一遍。

## 七、下一批模块的建模提醒

已落地的模块（采购 / 销售订单、库存、出货、货源、联系人、跟进、商机）不再写"该怎么做"——
照现有五层抄即可。还没开工的那几批，坑在 `docs/gap-analysis.md` 5.3（数据模型）与
5.7（与外贸场景不匹配的六处），这里只留三条不与那里重复的：

1. **应收应付不要各建一套汇率**：四件套与「汇率随单据锁定」已在 6 张表上验证过，照抄。
2. **合同 / PI 不要做成订单的打印视图**：PI 常常先于订单成立，做成订单附属会在还没订单时无处安放。
3. **附件与审计不要每个单据各做一遍**：先做 `sys_attachment` / `sys_audit_log` 两张通用表。

## 八、前端分层

目录结构见 `apps/frontend/AGENTS.md` 第 2 节。这里只讲**为什么这么分**——
调用链是 `pages → @refinedev/antd → providers → api/client.ts → types`，
每一层只解决一类问题，不越界：

各层只管自己的事：

- **`pages/`**：字段怎么展示、前端能拦的校验（必填、格式）。不发请求、不拼 URL——
  那是 provider 的活。
- **`providers/dataProvider.ts`**：唯一把 refine 的抽象翻译成后端实际接口的地方。
  分页参数名（`page` / `page_size`）、筛选白名单、`{code,message,data}` 拆包都在这里。
  后端加查询参数时改它，页面不用动。
- **`api/client.ts`**：唯一的 fetch 出口。令牌注入、滑动续期、401 清令牌因此只写一次，
  任何绕过它的请求都会静默缺掉这些横切行为。
- **`components/`**：只放无业务语义的展示件（`fields.tsx` 的 StatusTag / Money）
  与布局外壳，不放数据获取。

**不再有 `hooks/` 与 `store/`。** 列表状态机（分页 / 关键字 / 加载 / 错误 / 竞态）由
`useTable` 承接，跨页面的登录态由 `authProvider` + TanStack Query 承接。
这两层在 refine 改造前是手写的，改造后若继续留着，会形成两套并行的状态源。

### 包管理：只认 pnpm

规则表在 `README.md`「前端只认 pnpm」一节，这里只留一句**理由**：迁到 pnpm 不是为了快，
是为了让依赖边界能说话——npm 的扁平结构会把没声明的依赖摆到顶层，问题只在换机器或 CI 暴露。

