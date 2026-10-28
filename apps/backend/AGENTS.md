# apps/backend —— Go 服务（面向 AI 协作者）

范围：**只讲后端**。项目整体目标与技术栈见根目录 `AGENTS.md`；设计取舍的理由见 `docs/architecture.md`。
本文讲的是"代码怎么组织、改东西动哪、为什么这么写会报错"。

---

## 1. 职责边界

**负责**：HTTP API、鉴权、共享主数据（产品 / 供应商 / 客户）、统计聚合、
数据库与缓存访问。

**ERP 与 CRM 是这个服务的两个域，不是两个服务。** 当前只落地了共享主数据层，
ERP 侧的采购/库存/出货、CRM 侧的联系人/跟进/商机都在同一个进程里按同一套分层
往后加。域归属见 `docs/glossary.md` 第二节，判断标准是"这张表会不会被两个域
同时写"——会，就放 `md_` 下。

**不负责**：

| 不属于这里 | 归哪 |
|---|---|
| 页面渲染、静态资源 | `apps/frontend` |
| 权限矩阵、多租户 | 当前不做（见根 `AGENTS.md` 第 1 节） |
| 订单 / 库存 / 出货业务 | 未实现；套路见第 8 节 |
| CRM 跟进、商机 | 未实现；名称已在 `docs/glossary.md` 定好 |

单个进程、单二进制、无外部 RPC 依赖。启动即自检：配置不合法直接退出，不带病运行。

## 2. 目录结构

```
apps/backend/
├── cmd/server/main.go           # 入口：参数 → 配置 → 装配 → 启停（不做业务）
├── configs/                     # 4 份配置样例，见第 7 节
├── Dockerfile                   # 多阶段构建，上下文是本目录
└── internal/                    # 全部业务代码，外部不可 import
    ├── config/                  配置结构体、默认值、env 覆盖、启动期校验
    ├── bootstrap/               依赖装配（app.go）+ 演示数据（seed.go）
    ├── server/                  gin 引擎与路由表
    │   └── middleware/          RequestID / AccessLog / Recovery / CORS / Auth
    ├── handler/                 HTTP 适配：绑定 → 调 service → 写响应
    ├── service/                 业务规则；不 import gin
    ├── repository/              数据访问；不 import gin，实现可换
    │   └── dialect.go           三库差异收敛点（模糊搜索 / 转义）
    ├── model/                   gorm 模型，只有数据形状与 tag
    ├── dto/                     请求 / 响应 / 查询契约
    ├── migrate/                 一次性数据迁移（改结构之外的数据形状都在这里）
    ├── infra/
    │   ├── database/            连接、连接池、自动迁移
    │   ├── cache/               统一 Cache 接口 + redis / memory 两实现（含落盘）
    │   └── logger/              slog 构造
    └── pkg/
        ├── errs/                错误码与错误包装
        ├── money/               金额舍入规则（全站唯一一套）
        └── response/            统一响应体
```

测试与实现同目录：`config/config_test.go`、`infra/cache/cache_test.go`、
`repository/storage_test.go`、`migrate/legacy_merge_test.go`、`migrate/rename_legacy_test.go`。

**`migrate` 与 `database.AutoMigrate` 的分工别搞混**：AutoMigrate 只让表结构追上
模型（加列、加索引），不搬数据、不改名、不建立表间对应关系。凡是要动已有数据，
写成 `migrate` 里一个具名函数，并在 `Run` 里按顺序注册。

**唯一的例外是表改名**：`RenameLegacyPartnerTable` 必须在 AutoMigrate **之前**跑，
由 `bootstrap` 直接调用而不是注册进 `Run`。因为 AutoMigrate 只认模型，模型叫
`md_partner` 它就直接建一张空表，等表建完再改名就撞名了，而旧表里的数据会被晾在
原地 —— 启动照常成功，列表却是空的。

## 3. 依赖方向（编译期约束）

```
cmd/server → bootstrap → {server, handler, service, repository, infra, config}
handler    → service → repository → model
handler    → pkg/response, pkg/errs
service    → dto, pkg/errs, repository, infra/cache
```

反向依赖一律禁止，两条最容易被破坏：

- **`service` 里出现 `gin.Context`** → 业务逻辑被绑死在 HTTP 上，将来接 gRPC 或定时任务就得重写。
- **`handler` 里出现 `gorm.DB`** → SQL 漏到 HTTP 层，查询条件无法按库适配。

自检方式（只看**直接** import，`go list -deps` 会把传递依赖也算进来，不能用来判断分层）：

```bash
# handler 允许 import gin（它就是 HTTP 层），但不应直接碰 gorm
go list -f '{{join .Imports "\n"}}' ./internal/handler | grep -c 'gorm.io/gorm'   # 期望 0
# service 两个都不该有
go list -f '{{join .Imports "\n"}}' ./internal/service | grep -cE 'gin-gonic|gorm.io'  # 期望 0
```

## 4. 核心模块

| 模块 | 关键设计 | 改动时的注意点 |
|---|---|---|
| `config` | yaml → 默认值 → `GOWORK_*` env → `Validate()`；数据库/缓存的 `driver` 决定实现 | 新增配置项三件事一起做：结构体字段、`applyDefaults`、`applyEnv`，并在 `Validate` 里给启动期报错 |
| `infra/database` | 三驱动 dialector；sqlite 自动建目录、WAL、连接数收敛（单写者）；连接池按驱动给默认值 | 驱动差异只能出现在这里，不要外溢 |
| `migrate` | 一次性数据迁移：`sys_migration` 记已执行项，幂等；每份迁移一个具名函数，事务内完成 | 只搬数据不改结构（结构归 AutoMigrate）；读旧表加 `Unscoped`；状态不明时中止而不是继续插 |
| `infra/cache` | `Cache` 接口 7 个方法；redis 与 memory **语义对齐**（未命中 `ErrMiss`、`Incr` 滑动窗口、非数字返回 `ErrNotInteger`、ttl≤0 永不过期） | memory 有落盘：`Close()` 必须被调用，否则最后一段数据丢失 |
| `repository` | 泛型 `Base[T]` 提供 Create / UpdateFields / DeleteByID / FindByID / Exists / Count / Paginate；查询条件用 `Scope` 解耦 | 新增实体嵌 `Base[T]` 复用，只写自己独有的查询；复杂查询用 `r.DB()` |
| `service` | 每个领域一个接口 + 私有实现，构造函数用 **Option 模式**注入 logger 等可选依赖 | 业务规则（唯一性、状态、金额）只写在这一层 |
| `handler` | 只做 `ShouldBind*` + 调 service + `response.OK/Fail` | 不要在这里写 if 业务分支 |
| `pkg/errs` | 错误码前三位对齐 HTTP 状态码；`Wrap` 保留底层错误用于日志 | 新增码沿用规则，`response.Fail` 自动映射状态码 |
| `pkg/response` | 全站唯一响应体 `{code,message,data,request_id}` | 加字段要考虑前端 `api/client.ts` 的拆包逻辑 |
| `bootstrap` | 所有 `New` 集中在此，装配顺序：config → logger → db → 表改名迁移 → auto_migrate → migrate → cache → repo → service → handler → router → seed | 新模块只改这一个文件就能接上线 |

**分页有三处同名但不同的东西**，别混：`dto.PageQuery`（HTTP 入参）、
`repository.Pagination`（数据层入参）、`dto.PageResult[T]`（响应体）。转换在 service 层做。

## 5. 接口清单

统一前缀 `/api/v1`。除 `POST /auth/login` 与 `/healthz` 外都要 `Authorization: Bearer <token>`。

域归属见 `docs/glossary.md` 第三节：`products` 与 `partners` 会被 ERP、CRM 两侧引用。
新增接口时先确认它落在哪个域，再决定表前缀。

| 方法 | 路径 | handler | service |
|---|---|---|---|
| GET | `/healthz` | `handler.Health` | — |
| POST | `/api/v1/auth/login` | `Auth.Login` | `AuthService.Login` |
| GET | `/api/v1/auth/profile` | `Auth.Profile` | 取中间件注入的 `Principal` |
| POST | `/api/v1/auth/logout` | `Auth.Logout` | `AuthService.Logout` |
| GET | `/api/v1/dashboard/summary` | `Dashboard.Summary` | `DashboardService.Summary`（缓存 30s） |
| GET / POST | `/api/v1/products` | `Product.List` / `.Create` | `ProductService.List` / `.Create` |
| GET / PUT / DELETE | `/api/v1/products/:id` | `Product.Get` / `.Update` / `.Delete` | 同名方法 |
| GET / POST | `/api/v1/partners` | `Partner.List(nil)` / `.Create(nil)` | `PartnerService.List` / `.Create` |
| GET / PUT / DELETE | `/api/v1/partners/:id` | `Partner.Get(nil)` / `.Update(nil)` / `.Delete(nil)` | 同名方法 |
| GET / POST | `/api/v1/customers` | `Partner.*(&customerRole)` | 同上，路由注入角色 |
| GET / POST | `/api/v1/suppliers` | `Partner.*(&supplierRole)` | 同上 |
| GET / POST | `/api/v1/currencies` | `Currency.List` / `.Create` | `CurrencyService.List` / `.Create` |
| GET / PUT / DELETE | `/api/v1/currencies/:id` | `Currency.Get` / `.Update` / `.Delete` | 同名方法 |
| GET / POST | `/api/v1/exchange-rates` | `Rate.List` / `.Create` | `ExchangeRateService.List` / `.Create` |
| GET / DELETE | `/api/v1/exchange-rates/:id` | `Rate.Get` / `.Delete` | 同名方法（汇率不可编辑） |
| GET / POST | `/api/v1/contacts` | `Contact.List` / `.Create` | `ContactService.List` / `.Create` |
| GET / PUT / DELETE | `/api/v1/contacts/:id` | `Contact.Get` / `.Update` / `.Delete` | 同名方法（不能改 partner_id） |
| GET / POST | `/api/v1/activities` | `Activity.List` / `.Create` | `ActivityService.List` / `.Create`（`?due=1` 查已到期未跟进） |
| GET / PUT / DELETE | `/api/v1/activities/:id` | `Activity.Get` / `.Update` / `.Delete` | 同名方法（按 occurred_at 倒序，不可改 partner_id） |
| GET / POST | `/api/v1/opportunities` | `Oppty.List` / `.Create` | `OpportunityService.List` / `.Create`（按预计成交日升序） |
| GET / PUT / DELETE | `/api/v1/opportunities/:id` | `Oppty.Get` / `.Update` / `.Delete` | 同名方法，阶段变更走商机状态机 |
| GET | `/api/v1/opportunities-summary` | `Oppty.Summary` | 按阶段汇总；独立前缀，避免与 `/:id` 抢路径 |

`/customers` 与 `/suppliers` 是 `/partners` 的角色切片，不是两套实现：同一个
`PartnerHandler`，路由层把默认角色传进去。新增字段只改 `PartnerHandler` 一处。

切片的行为约定：列表强制按角色过滤、创建时补 role、`Get/Update/Delete` 对
角色外的 id 返回 404、并通过这两个入口编辑时**忽略** role 字段（不让客户页面
有机会抹掉供应商身份）。改这套逻辑前先看 `internal/handler/partner.go` 的注释。

列表通用查询参数：`page`（默认 1）、`page_size`（默认 20，上限 200）、`keyword`；
产品另有 `category` / `supplier_id` / `status`，往来单位另有 `role` / `entity_type` /
`country` / `status`。

**路由表只有一处**：`internal/server/router.go`。加接口先在这里挂，再写 handler。

## 6. 数据流

### 一次普通列表请求

```
GET /api/v1/products?page=2&keyword=hinge
 1 middleware.RequestID   生成/透传 X-Request-ID，写入 gin.Context
 2 middleware.AccessLog   记方法、路径、状态、耗时
 3 middleware.Recovery    panic → 500，进程不崩
 4 middleware.CORS        按 http.cors_origins 放行
 5 middleware.Auth        解析 Bearer → AuthService.Verify → 身份写入上下文
 6 Product.List           ShouldBindQuery(dto.ProductQuery) → .Normalize() 修正分页
 7 ProductService.List    dto → repository.Pagination + ProductFilter
 8 productRepo.List       走 Base.Paginate：先 Count 再 Find（不能复用同一个 *gorm.DB）
 9 response.OK            {code:0, data:{list,total,page,page_size}, request_id}
```

任何一层返回 error，都汇到 `response.Fail`：业务错误按自己码返回，未知错误折叠成 50001，
内部细节只进日志不进响应体。

### 登录与会话

```
POST /auth/login
 1 查缓存 login_fail:{username}，≥5 次直接 403（窗口 15 分钟，Incr 每次重置 TTL）
 2 查用户 → 校验启用状态 → bcrypt 比对
   失败一律回"用户名或密码错误"（防账号枚举），并 Incr 失败计数
 3 生成 tokenID（随机 16 字节 hex）
 4 写会话：session:{tokenID} → user_id，TTL = jwt.expire
 5 签 JWT：HS256，claims 含 uid/usr/nick/role + jti=tokenID
 6 清空失败计数，返回 {token, expires_at, user}
```

**为什么令牌有效还要查会话**：只有签名的话，登出后旧令牌还能用到过期。
`Verify` 里强制查一次 `session:{jti}`，所以 `Logout` 只要删键就立即失效——这是本设计的关键点，
不要为了省一次缓存读写把它去掉。

### 令牌续期

`Verify` 发现令牌剩余有效期不足 `jwt.expire/2` 时自动续期：

```
1 用原 claims 重签（jti 不变），exp = now + jwt.expire
2 session:{jti} 的 TTL 重置为 jwt.expire
3 新令牌经响应头 X-Renewed-Token 下发，前端自动替换本地值
```

**为什么复用 jti**：一次页面加载会并发打好几个请求，每个都可能在续期。
jti 不变意味着它们指向同一条会话，续期天然幂等；若每次换新 jti，会话条数会随请求数增长，
还得额外做去重和旧键清理。

**为什么走响应头**：`{code,message,data}` 是全局响应契约，
不该为横切副作用去改每个接口的 data 结构。

**顺序不能反**：先重置会话 TTL，再下发新令牌。反过来新令牌的 exp 可能超出会话寿命，
客户端会拿着一个看似有效、实际已被吊销的令牌反复重试。

**CORS 必须暴露这个头**：`Access-Control-Expose-Headers` 里漏了 `X-Renewed-Token`，
跨域部署下该头对 JS 不可见，续期会静默失效而没有任何报错。开发期走 vite proxy 同源，
这个坑测不出来，靠 `middleware/auth_test.go` 兜住。

续期失败（签名或写缓存出错）只放弃本次续期，请求照常放行——此刻手上的令牌还没过期。

**已知副作用**：`role` 在 claims 里，所以改角色/降权在令牌过期前不生效。
要立即生效必须按 uid 反查并删掉该用户的全部 `session:*`；目前没有「管理员改别人权限」的入口，
先接受这个延迟。

## 7. 运行与配置

```bash
task backend:dev        # 热重载启动（air），SQLite + 内存缓存（默认，零外部依赖），:8080
task backend:dev:once   # 同上但不监听变更（调启动流程 / 没有 air 时用）
task backend:dev:pg     # PostgreSQL + Redis，:8081（需先 task infra:up）
task backend:dev:mysql  # MySQL + Redis，:8082（需先 infra:up + compose --profile mysql）
task backend:build      # 产出 bin/gowork-server
task backend:test       # go test ./... -count=1
task backend:test:storage  # 三库两缓存一致性测试
task backend:tool:air   # 安装 air（一次性，版本锁在 Taskfile 的 vars.AIR_VERSION）
```

直接跑二进制：`./bin/gowork-server -config configs/config.yaml`（`-config` 是唯一启动参数）。

### 热重载（air）

`task backend:dev` 走 [air](https://github.com/air-verse/air)：改 `*.go` → 重新编译 → 自动重启。
前端 `pnpm dev` 本来就是 Vite HMR，根目录 `task dev` 因此是两端都热更新的。

| 事项 | 约定 |
|---|---|
| 配置文件 | `apps/backend/.air.toml`，与 `go.mod` 同级。air 只从**工作目录**找它，所以必须在 `apps/backend` 下启动（Taskfile 的 `dir` 已保证）；在别处跑要用 `-c` 显式指，且 `root` 会对不上，不推荐 |
| 安装 | `task backend:tool:air`。air 是开发工具，**不进 `go.mod`**；版本锁在 `Taskfile.yml` 的 `vars.AIR_VERSION` |
| 监听范围 | `root = "."`（就是 `apps/backend`），`include_ext = ["go"]`：只有 `.go` 变更触发编译 |
| 忽略目录 | `bin`（产物与 `build-errors.log`，不排除会自己触发自己）、`data`（sqlite 的 `db`/`-wal`/`-shm` 与缓存快照运行期一直在写）、`.git`、`vendor`、`node_modules`、`.vscode`、`.idea` |
| 排除文件 | `_test.go`（不进服务二进制，改了不必重启）、编辑器临时/交换文件（`.#*`、`*~`）。另有 `exclude_unchanged = true`：内容没变不重启 |
| 编译命令 | `go build -o ./bin/dev/gowork-server ./cmd/server`。不带 `-trimpath` / `-ldflags "-s -w"`——那是 `task backend:build` 的发布参数，热重载剥符号只拖慢重启 |
| 运行命令 | `entrypoint = ["./bin/dev/gowork-server"]`，不写 `-config`，走 `main.go` 里 `-config` 的默认值 `configs/config.yaml` |
| 产物位置 | `bin/dev/gowork-server`；`bin/` 已在 `.gitignore`。它与 `task backend:build` 的 `bin/gowork-server` 是两个文件，互不覆盖 |
| 日志 | 业务日志仍是 slog 打 **stdout**，不经 air、不落文件。air 自己的 `building... / running... / failed to build` 带 `[HH:MM:SS]` 前缀（`log.time = true`），与业务日志一眼分得开。编译报错详情在终端，`bin/dev/build-errors.log` 只有一行 `exit status N` |
| 重启语义 | `send_interrupt = true` + `kill_delay = "3s"`：先 SIGINT 走优雅关闭（刷缓存快照、关连接池），3 秒不退才 SIGKILL。`stop_on_error = true`：编译不过就停掉旧进程，不留着旧二进制继续服务（见第 9 节最后一条） |
| 改配置 | `.yaml` / `.air.toml` 不在监听范围内，改完要手动 Ctrl-C 再起——air 不重载自身配置，被启动的进程也只在启动时读一次配置 |

换库启动仍可热重载：`task backend:dev:pg` / `dev:mysql` 用 `air -c .air.toml -- -config configs/config.postgres.yaml`，
`--` 之后的参数由 air 原样透传给被启动的二进制。

想临时调 air 自身参数，用命令行覆盖而不是改文件：`air -c .air.toml -build.delay 1000`。
容器 / 网络挂载卷上 inotify 收不到事件时再加 `-build.poll true`。

**团队协作**：`.air.toml` 提交进仓库、全组共用一份——air 不合并多份配置，谁本地改一份，
谁的重启行为就和别人不一样，而症状是"同样的改动我这边重启了、你那边没有"。
个人差异往两处走：端口/库这类环境差异用 `GOWORK_*` 环境变量
（`GOWORK_HTTP_ADDR=:8090 task backend:dev`），air 自身参数用命令行覆盖。
升级 air 版本只改 `Taskfile.yml` 的 `AIR_VERSION` 并单独提交——行为变化表现为"重启时机变了"，
混在功能提交里没人查得出来。

### 配置文件与端口

| 配置文件（均在 `configs/`） | 数据库 | 缓存 | 端口 |
|---|---|---|---|
| `config.yaml` | SQLite `data/gowork.db` | memory（落盘 `data/cache.snapshot`） | 8080 |
| `config.postgres.yaml` | PostgreSQL | redis | 8081 |
| `config.mysql.yaml` | MySQL | redis | 8082 |
| `config.prod.yaml` | PostgreSQL | redis（`fallback_to_memory: false`） | 8080 |

配置优先级：**配置文件 < `GOWORK_*` 环境变量**。常用覆盖项：
`GOWORK_HTTP_ADDR`、`GOWORK_DB_DRIVER`、`GOWORK_DB_DSN`、`GOWORK_DB_PASSWORD`、
`GOWORK_CACHE_DRIVER`、`GOWORK_REDIS_ADDR`、`GOWORK_JWT_SECRET`、`GOWORK_LOG_LEVEL`。
生产环境的密码与密钥只走环境变量，不写进配置文件。

启动顺序里有两个开关值得记住：`database.auto_migrate`（建表）与 `database.seed`（演示数据，
账号 `admin / admin123`），生产两者都应为 false。

## 8. 开发约定

### 新增一个业务模块（五步）

1. `internal/model/xxx.go`：嵌 `Base`，写 `TableName()`（域前缀 `sys_` / `md_` / `biz_` / `inv_` / `crm_`，
   含义见 `docs/glossary.md` 第二节），在 `bootstrap.New` 的 `AutoMigrate` 列表里加一行。
2. `internal/repository/xxx.go`：定义 `XxxFilter` + `XxxRepository` 接口，实现嵌 `Base[model.Xxx]`；
   模糊搜索用 `likeAny`，不要自己拼 `LIKE`。
3. `internal/dto/xxx.go`：`XxxQuery` 嵌 `PageQuery`；`XxxInput` 做全量入参（新增与编辑共用一套校验）。
4. `internal/service/xxx.go`：`XxxService` 接口 + 私有实现 + `WithXxxLogger` Option；
   业务规则（唯一性、状态归一、跨表校验）都在这里。
5. `internal/handler/xxx.go` + `server/router.go` 挂路由 + `bootstrap/app.go` 装配。

### 硬性规则

- **编码字段**（`sku_code` / `code`）必须 `uniqueIndex`，service 层做前置查重，冲突返回 `40901`。
  查重方法名要带列语义：产品的叫 `ExistsBySKUCode`（查 `sku_code`），主体的叫 `ExistsByCode`
  （查 `code`）。合并前客户与供应商各有一个 `ExistsByCode`，三个方法同名不同义，
  调用方只能靠记参数分辨。
- **角色判断必须按位与**：`Partner.Role` 是位掩码（1 客户 / 2 供应商 / 3 两者）。
  写 `role == model.PartnerRoleSupplier` 会把客供两用的主体判成"不是供应商"。
  列表按角色筛选走 `model.RoleValuesContaining` 展开成 `IN`，不要直写位运算条件
  （PG 与 MySQL 上都用不了 btree 索引）。
- **两类状态不要混**：主数据与账号用 `model.EnableStatus`（1 启用 / 0 停用）；
  单据状态是 `model.DocumentStatus`（10 草稿 / 20 已确认 / 30 备货中 / 40 已出货 /
  50 已完成 / 90 已取消）。两者取值都是小整数，赋错了编译器不报，只会在筛选条件上出错。
- **单据状态只能按 `CanTransitTo` 跳转**，校验统一走 `service.checkDocumentTransit`。
  各模块自己写 if 的那一刻，状态机就有了第二份实现，分叉的表现是"同样一次取消，
  报价单允许、订单拒绝"。已出货的单据不允许取消——货在海上，要冲出得走退货单。
- **单号一律走 `DocNoService.Next`**（`SO-202610-0001`），不要在业务代码里拼
  "前缀 + 时间 + 查最大号 + 1"：并发下两个请求会拿到同一个号，撞的是单据表的唯一索引，
  表现为保存失败而不是重试。号段表是 `sys_doc_seq`（`sys_` 前缀，不嵌 `Base`）。
- **商机阶段（`OpportunityStage`）不是单据状态**，不要复用 `DocumentStatus`：
  商机阶段可以回退（客户预算冻结时真实地退回去了），单据状态只能前进与取消。
  两者的"终态不可再改"是同一条规则，其余全不同。
- **更新用 `UpdateFields(map)`**，不要 `Updates(struct)`：零值会被当成"未提交"而被忽略。
- **删除是软删除**（`gorm.DeletedAt`）。查重、统计都要意识到已删数据仍占行。
- **分页必须走 `Pagination`**，禁止无上限的 `Find(&list)`。
- **自定义 SQL 必须三库兼容**：没有 `IFNULL`、`group_concat`、`ILIKE` 直写；
  大小写不敏感搜索用 `repository.likeAny`。
- **错误用 `errs`**：业务错误返 `errs.New/Wrap`，不要 `errors.New` 直接往外抛。
- **日志用注入的 `*slog.Logger`**，键值对形式，不要把变量拼进消息串。
- 改完跑 `gofmt -l .`（应为空）、`go vet ./...`、`go test ./... -count=1`。

## 9. 已知陷阱

| 现象 | 原因 | 处理 |
|---|---|---|
| `go build` 报 `no space left on device` | 某些环境 `/tmp` 很小，构建临时目录写满 | `export TMPDIR=<大分区目录>` |
| sqlite 下并发写报 `database is locked` | SQLite 单写者模型 | 已配 WAL + `busy_timeout`，连接数已收敛；批量写要考虑合并事务 |
| PostgreSQL 上 `keyword` 搜不到小写结果 | PG 的 `LIKE` 区分大小写 | 用 `likeAny`（内部按方言选 `ILIKE`），别自己拼 SQL |
| 登出后旧令牌还能用 | 绕过了会话校验 | `AuthService.Verify` 里那次 `sessions.Get` 不能删 |
| 重启后登录态全没了 | memory 缓存没落盘 | `cache.memory.persist: true` 且 `Close()` 被调用（`main` 里靠 defer 保证） |
| 生产误用了内存缓存 | `cache.driver` 配错 | 生产 `fallback_to_memory: false`，连不上 Redis 直接启动失败 |
| 第 3 节的自检命令计数不为 0 | 新代码引入了反向依赖 | 按分层把逻辑挪回对应层 |
| 改了结构体字段名之后那一列数据全变空 | 没写 `gorm:"column:..."`，gorm 按新字段名建了新列，旧数据留在旧列里 | 列名与字段名不一致时必须显式写 `column:`，例：`model.Partner.ContactName` 绑的是 `contact` 列 |
| 筛选查不到记录，但库里明明有 | 把 `DocumentStatus` 的值传给了 `EnableStatus` 字段（或反过来） | 两者都是小整数，编译器不拦；先按 `docs/glossary.md`「状态」一节确认这个字段属于哪套 |
| 明明同时是客户和供应商，却在供应商列表里找不到 | 用 `role = ?` 判角色 | 角色是位掩码，必须按位与（`role.Has(...)`）；列表筛选走 `RoleValuesContaining` 展开成 IN，既覆盖 `role=3` 又能命中索引 |
| 某主体在两个列表里都显示成"未知角色" | 前端写了 `role === 1` / `role === 2` | 同样按位与：`(role & PARTNER_ROLE.supplier) !== 0` |
| 新记录提交"停用"，存进去却是"启用" | 字段带 `gorm:"default:1"` 且值恰为零值，gorm 会省略该列让数据库填默认值 | 新表不要再给 `EnableStatus` 加 `default` tag；老表改状态走 `UpdateFields(map)`（map 显式指定列，不受影响） |
| 迁移后少了若干条旧数据 | 读旧表时漏了 `Unscoped`，软删除条件由目标 struct 的 schema 决定，`Table()` 指定表名也绕不过去 | `tx.Table("md_xxx").Unscoped().Find(&rows)`，`internal/migrate/legacy_merge_test.go` 有对应用例 |
| 接口整批 500，`/auth/login` 却正常 | 跑着的是改表名前编译的旧二进制，仍按 `md_party` 查询已被改名的 `md_partner`；登录只碰 `sys_user` 所以不受影响 | 重启 dev 即可，不是代码缺陷。判断版本：`/partners` 404 + `/parties` 401 就是旧二进制。`response.Fail` 已把真实原因写进日志，先看日志再猜 |
