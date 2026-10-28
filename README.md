# GoWork

外贸采购代理的 **ERP + CRM 集成系统**：Go 后端（gin + gorm + redis）+ React 管理端，
用 Taskfile 统一管理指令。

一条主数据（产品 / 往来单位）供两侧业务共用——ERP 管采购、库存、出货与订单，
CRM 管联系人、跟进与商机，两侧引用同一行往来单位记录。

客户与供应商本身就合并在这张表里，靠 `role` 位掩码区分：同一个主体可以既是客户
又是供应商，此时只有一条记录、一套联系方式。

> 面向 AI 协作者的文档：根目录 [`AGENTS.md`](AGENTS.md) 讲目标与整体架构，
> [`docs/glossary.md`](docs/glossary.md) 是术语与域边界的唯一事实来源，
> `apps/backend/AGENTS.md`、`apps/frontend/AGENTS.md` 讲各自的分层、接口与约定。
> 本文只负责"怎么把它跑起来"。

目录分两块，构建与运行彼此独立，靠 HTTP API 解耦：

```
GoWork/
├── AGENTS.md                 # 面向 AI 的项目总览（目标 / 架构 / 模块划分）
├── Taskfile.yml              # 根编排：只调用子项目的 Taskfile
├── apps/
│   ├── backend/              # Go 服务（+ AGENTS.md：分层、接口、数据流）
│   └── frontend/             # React 应用（+ AGENTS.md：分层、数据流、页面套路）
├── deployments/
│   └── docker-compose.yml    # PostgreSQL + Redis（MySQL 走 profile）
└── docs/
    ├── glossary.md           # 术语表：叫什么、归哪个域、哪些词不许用
    └── architecture.md       # 分层约定、存储可替换性、扩展指引
```

## 技术栈

| 层 | 选型 |
|---|---|
| HTTP | gin v1 |
| ORM | gorm（PostgreSQL / MySQL / SQLite 三驱动，配置切换） |
| 缓存 | go-redis v9 / 进程内实现（可落盘），配置切换 |
| 鉴权 | JWT（HS256）+ 服务端会话，登出即失效、滑动续期 |
| 日志 | 标准库 log/slog，结构化输出 |
| 前端 | React 19 + TypeScript + Vite + refine 5 + Ant Design 5 + react-router（pnpm 管理依赖） |

## 快速开始

前置：Go ≥ 1.26（`go.mod` 声明）、Node ≥ 20、pnpm ≥ 11。Taskfile 需要 `task`（安装见文末）。

前端包管理器是 **pnpm**，用 corepack 一条命令对齐版本（版本锁在 `apps/frontend/package.json` 的 `packageManager`）：

```bash
corepack enable pnpm        # 之后 pnpm 命令会按 packageManager 自动取对应版本
```

```bash
task install           # 装依赖（后端 go mod + 前端 pnpm install）
task backend:tool:air  # 一次性：装后端热重载工具 air（go install，不进 go.mod）
task dev               # 并行起后端 :8080（air 热重载）与前端 :5173（Vite HMR）
```

打开 http://localhost:5173，默认账号 **admin / admin123**。

改后端 `*.go` 会自动重新编译并重启，改前端由 Vite HMR 直接生效，两侧都不用手动重启。
`task backend:dev` 若提示找不到 air，就是上面那条 `tool:air` 没执行（版本锁在
`apps/backend/Taskfile.yml` 的 `AIR_VERSION`，配置见 `apps/backend/.air.toml`）。

首次启动会自动建表并写入演示数据（5 个往来单位、4 个产品）。演示数据刻意包含两种
合并前无法表达的情况：一个"客供两用"主体（`BOTH001`，role=3）和一个个人主体
（`PER001`，entity_type=1），用来验证角色位与主体类型确实生效。

没有 `task` 时等价操作：

```bash
cd apps/backend && air -c .air.toml              # 后端（热重载；不想热重载就 go run ./cmd/server）
cd apps/frontend && pnpm install && pnpm run dev # 前端
```

用 npm / yarn 装前端依赖会被 `preinstall` 守卫直接拦下（报错退出，不会生成第二份锁文件）。

### 存储：默认零依赖，按环境切换

开发默认 **SQLite + 内存缓存**，不装任何数据库就能跑：

- `data/gowork.db` 本地库文件（WAL 模式）
- `data/cache.snapshot` 内存缓存的落盘快照，重启后登录态和限流计数还在

两者都在 `.gitignore` 里。三种组合都已实测（见下表），切换只改配置：

| 想跑的组合 | 执行 |
|---|---|
| SQLite + 内存（默认） | `task backend:dev` |
| PostgreSQL + Redis | `task infra:up` 然后 `task backend:dev:pg` |
| MySQL + Redis | `task infra:up` + `docker compose --profile mysql up -d`，再 `task backend:dev:mysql` |

也可以完全用环境变量覆盖，不碰配置文件：

```bash
GOWORK_DB_DRIVER=postgres GOWORK_DB_HOST=127.0.0.1 GOWORK_DB_USER=gowork \
GOWORK_DB_PASSWORD=gowork GOWORK_DB_NAME=gowork \
GOWORK_CACHE_DRIVER=redis GOWORK_REDIS_ADDR=127.0.0.1:6379 \
task backend:run
```

配置优先级：配置文件 < `GOWORK_*` 环境变量。生产模板见 `configs/config.prod.yaml`。

一致性验证（同一套断言跑三库两缓存）：

```bash
task test:storage   # postgres/mysql/redis 没起来时对应子用例自动跳过
```

## Taskfile 命令

根目录：

| 命令 | 作用 |
|---|---|
| `task` | 列出全部任务 |
| `task install` | 安装前后端依赖（前端 pnpm install） |
| `task install:ci` | 按锁文件严格安装（前端 `--frozen-lockfile`，依赖与锁文件不一致直接失败） |
| `task dev` | 并行启动前后端（后端 air 热重载，前端 Vite HMR） |
| `task backend:dev:once` | 后端单次启动、不监听变更 |
| `task backend:tool:air` | 安装 air（一次性；版本锁在 `apps/backend/Taskfile.yml`） |
| `task build` | 构建后端二进制 + 前端静态产物 |
| `task test` | 后端单元测试 |
| `task test:storage` | 三库两缓存的一致性测试（未启动的服务自动跳过） |
| `task lint` | `go vet` + 前端类型检查 |
| `task fmt` | gofmt |
| `task clean` | 清理 `bin/` 与 `dist/` |
| `task infra:up` / `infra:down` | 启停 PostgreSQL + Redis（MySQL 加 `--profile mysql`） |

子目录也有各自的任务：`task -d apps/backend -- --list`、`task -d apps/frontend -- --list`。

### 前端包管理：只认 pnpm

| 事项 | 约定 |
|---|---|
| 包管理器版本 | `apps/frontend/package.json` 的 `packageManager: pnpm@12.10.1`，corepack / CI 都读它 |
| 锁文件 | 只有 `pnpm-lock.yaml`；`package-lock.json`、`yarn.lock` 已列入 `.gitignore` |
| 安装 | 开发 `pnpm install`（可更新锁文件）；CI / 发布 `pnpm install --frozen-lockfile` |
| node_modules | 保留 pnpm 默认的符号链接结构，不开 `shamefully-hoist`：用了没声明的依赖当场报错 |
| 依赖构建脚本 | `pnpm-workspace.yaml` 的 `allowBuilds` 白名单，目前只放行 `esbuild` |

两条容易踩的：

1. **pnpm 不会装太新的版本**。默认 `minimumReleaseAge=1440`（分钟），发不满一天的版本会被跳过。所以同样的 `^` 范围，pnpm 解析出的版本可能比 npm 旧一版——这是有意为之，不是锁文件坏了。要立刻装最新版得显式放行（`minimumReleaseAgeExclude`）或临时 `pnpm update --config.minimumReleaseAge=0`。
2. **pnpm 12 的锁文件是两段 YAML**。第一段锁定 pnpm 自己的版本（`packageManagerDependencies`），第二段才是项目依赖。看到文件里出现两个 `importers:` 是正常的，别手改。


## API

统一响应格式：`{"code":0,"message":"ok","data":...,"request_id":"..."}`，`code != 0` 时按前三位对应 HTTP 状态码。

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/healthz` | 健康检查（免鉴权） |
| POST | `/api/v1/auth/login` | 登录 |
| GET | `/api/v1/auth/profile` | 当前用户 |
| POST | `/api/v1/auth/logout` | 登出 |
| GET | `/api/v1/dashboard/summary` | 概览统计 |
| GET/POST | `/api/v1/products` | 产品列表 / 新建 |
| GET/PUT/DELETE | `/api/v1/products/:id` | 产品详情 / 编辑 / 删除 |
| GET/POST | `/api/v1/partners` | 往来单位列表 / 新建（客户与供应商的统一资源） |
| GET/PUT/DELETE | `/api/v1/partners/:id` | 往来单位详情 / 编辑 / 删除 |
| GET/POST | `/api/v1/suppliers` | 供应商列表 / 新建（`/partners` 锁定供应商角色） |
| GET/PUT/DELETE | `/api/v1/suppliers/:id` | 供应商详情 / 编辑 / 删除 |
| GET/POST | `/api/v1/customers` | 客户列表 / 新建（`/partners` 锁定客户角色） |
| GET/PUT/DELETE | `/api/v1/customers/:id` | 客户详情 / 编辑 / 删除 |

除 `/healthz` 与 `/auth/login` 外，均需 `Authorization: Bearer <token>`。

列表接口通用查询参数：`page`、`page_size`（上限 200）、`keyword`，另有各模块的 `status`、`category` 等；
往来单位另有 `role`（位掩码，按包含语义过滤）与 `entity_type`（0 公司 / 1 个人）。

## 已实现模块

**共享主数据**（ERP 与 CRM 同时引用）

- **产品**：SKU 编码唯一校验、分类、默认货源（供应商）、成本/售价、软删除
- **往来单位**：编码唯一校验、角色位掩码（客户 / 供应商 / 两者）、主体类型（公司 / 个人）、
  国家/地区、主联系人、软删除。有产品引用时不能删除，也不能摘掉供应商角色

**系统**

- **鉴权**：登录、登出、身份校验、登录失败限流（15 分钟内 5 次）、令牌滑动续期
- **概览**：跨域聚合统计，结果缓存 30 秒

**未实现**：ERP 侧的采购订单、销售订单、库存、出货批次；CRM 侧的联系人、
跟进记录、商机。名称已在 [docs/glossary.md](docs/glossary.md) 定好，
扩展方式见 [docs/architecture.md](docs/architecture.md) 第六节。

## 安装 task

```bash
# 任选其一
go install github.com/go-task/task/v3/cmd/task@latest   # 装到 $(go env GOPATH)/bin
brew install go-task/tap/go-task                        # macOS
sudo snap install task --classic                        # Ubuntu
```
