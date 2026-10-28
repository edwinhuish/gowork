# GoWork —— 项目总览（面向 AI 协作者）

这份文件是入口。读完它应当能回答三个问题：**这个项目要解决什么、代码在哪、改某类东西该动哪一层。**
某一层的细节不在本文重复，去看对应目录的文档：

- 后端：`apps/backend/AGENTS.md`
- 前端：`apps/frontend/AGENTS.md`
- 设计取舍的长文：`docs/architecture.md`

---

## 1. 项目目标

外贸 ERP 的最小可用骨架。先把「供应商 → 产品 → 客户」这段主数据做扎实，
再按同一套分层往里加单据（订单 / 采购 / 库存 / 出货）。

**当前阶段：主数据。** 已实现鉴权、产品、供应商、客户、概览统计。
订单、采购、库存、出货批次**尚未实现**，扩展套路见 `docs/architecture.md` 第六节。

**刻意不做的事**（不要"顺手"加上）：

| 不做 | 原因 |
|---|---|
| 多租户 / 权限矩阵 | 只有 `admin` 与普通用户两种角色，够用；先做会把每条查询都染色 |
| 微服务拆分 | 单进程足够；分层已经让边界清楚，拆进程是部署问题不是代码问题 |
| 把订单号 / 批次号 / 价格编进 SKU | SKU 只承载身份。订单与出货是多对多（分批 / 合并），塞进编码必崩 |
| GraphQL / tRPC | REST + 统一响应体已经够薄，换协议收益不抵学习成本 |

## 2. 仓库布局

```
GoWork/
├── AGENTS.md                    # 本文件：总览与导航
├── Taskfile.yml                 # 根编排：只调子项目任务，不写具体命令
├── apps/
│   ├── backend/                 # Go 服务（见 apps/backend/AGENTS.md）
│   │   ├── cmd/server/          # 启动入口，只做参数→配置→装配→启停
│   │   ├── configs/             # 4 份配置样例：默认 / prod / postgres / mysql
│   │   └── internal/            # 全部业务代码，不对外暴露
│   └── frontend/                # React 单页应用（见 apps/frontend/AGENTS.md）
│       ├── src/                 # pages / hooks / api / components / store / types
│       └── pnpm-workspace.yaml  # pnpm 设置（不是工作区划分）
├── deployments/
│   └── docker-compose.yml       # 本地 PostgreSQL + Redis（MySQL 走 profile）
├── docs/architecture.md         # 分层约定、存储可替换性、扩展指引
└── .github/workflows/ci.yml     # CI：go build/vet/test + pnpm install/typecheck/build
```

**两端的耦合方式只有一条：HTTP。** 前端不 import 后端代码，后端不产前端产物，
`deployments/` 里也没有把两者打包进同一镜像——部署时由网关或反代拼起来。

## 3. 整体架构

```
浏览器 (React SPA :5173)
   │  fetch /api/v1/*        dev 时由 Vite 代理，生产由反代接管
   ▼
gin HTTP 层 (Go :8080)
   middleware: RequestID → AccessLog → Recovery → CORS → Auth
   handler  →  service  →  repository  →  model
   │                │            │
   │                │            └── gorm ──→ PostgreSQL | MySQL | SQLite（配置切换）
   │                └──────────────────────→ Cache 接口 ──→ Redis | memory(可落盘)（配置切换）
   └── pkg/errs + pkg/response：统一错误码与响应体
```

三条贯穿全局的设计：

1. **依赖单向**：`handler → service → repository → model`。反过来 import 了就说明分层错了。
2. **存储可替换**：数据库与缓存都靠配置里的 `driver` 切换，业务代码零改动。
   开发默认 SQLite + memory，不装任何外部服务就能跑。
3. **装配集中**：所有 `New` 只出现在 `internal/bootstrap/app.go`，其余地方不做依赖拼装。

## 4. 技术栈

版本以 `apps/backend/go.mod` 与 `apps/frontend/package.json` 为准，此处只写选型理由。

| 位置 | 选型 | 为什么是这个 |
|---|---|---|
| HTTP | gin v1.12 | 生态成熟，中间件与分组路由够用，不需要更重的框架 |
| ORM | gorm v1.31 + postgres/mysql/glebarez-sqlite | 三种驱动统一 API；sqlite 用纯 Go 实现，免 cgo，交叉编译和容器都省事 |
| 缓存 | go-redis v9 / 进程内实现（可落盘） | 开发不想拉 Redis；两套实现语义对齐后可按环境换 |
| 鉴权 | JWT HS256 + 服务端会话 | 只有签名不够——登出要能立即失效，所以会话必须落在缓存里 |
| 配置 | yaml + `GOWORK_*` 环境变量 | 12-factor；敏感值走环境变量，不进仓库 |
| 日志 | 标准库 `log/slog` | 结构化日志不需要第三方库 |
| 前端 | React 19 + TypeScript + Vite 6 + react-router 7 | 无状态管理库、无 UI 库：当前页面量级自己写更省心 |
| 包管理 | pnpm 12（`packageManager` 锁定） | 严格 node_modules，未声明的依赖当场报错 |
| 任务编排 | Taskfile | 跨平台，比 shell 脚本可读；命令下沉到子目录避免重复维护 |

## 5. 模块划分

**后端**（细节见 `apps/backend/AGENTS.md`）：

| 模块 | 路由前缀 | 说明 |
|---|---|---|
| auth | `/api/v1/auth` | 登录、当前用户、登出；登录失败限流 |
| product | `/api/v1/products` | 产品主数据，SKU 唯一 |
| supplier | `/api/v1/suppliers` | 供应商 |
| customer | `/api/v1/customers` | 客户 |
| dashboard | `/api/v1/dashboard` | 聚合统计，结果缓存 30 秒 |

**前端**（细节见 `apps/frontend/AGENTS.md`）：登录页 + 概览 / 产品 / 供应商 / 客户四个列表页，
共用一套列表状态机（分页、关键字、加载、错误、竞态）。

## 6. 跨层约定

这几条是"改代码前必须知道"的，违反任何一条都会立刻在别处露出来：

1. **响应体固定** `{"code":0,"message":"ok","data":...,"request_id":"..."}`；
   `code != 0` 时前三位对齐 HTTP 状态码（40001 → 400）。新增错误码沿用这个规则，
   前端就能按前三位做统一分支，不必枚举每个码。
2. **handler 不 import gorm，service 不 import gin。** 前者意味着 SQL 泄漏到 HTTP 层，
   后者意味着事务/重试逻辑没法脱离 HTTP 测试。
3. **换存储不改代码。** 见到 `if driver == "mysql"` 这种分支就是信号：应该收进 `infra/` 或
   `repository/dialect.go`。三库行为一致性由 `task test:storage` 保证。
4. **前端只认 pnpm。** 锁文件只有 `pnpm-lock.yaml`；`npm install` 会被 `preinstall` 守卫拦掉。
5. **新增业务模块照五步走**：model → repository → dto → service → handler+router，
   最后在 `bootstrap/app.go` 装配一次。检查清单在 `docs/architecture.md` 第六节末尾。

## 7. 运行

```bash
task install      # 装依赖（go mod + pnpm install）
task dev          # 并行起后端 :8080 与前端 :5173，Ctrl-C 一并退出
task build        # 后端二进制 + 前端静态产物
task test         # 后端单元测试
task test:storage # 三库两缓存一致性测试（没起的服务自动跳过）
task lint         # go vet + tsc --noEmit
```

默认账号 `admin / admin123`，首次启动自动建表并写入演示数据。
零依赖跑法：默认配置就是 SQLite + 内存缓存，不需要任何容器。

## 8. 文档地图

| 文件 | 讲什么 | 什么时候读 |
|---|---|---|
| `AGENTS.md`（本文） | 目标、架构、技术栈、模块划分 | 第一次接触项目 |
| `apps/backend/AGENTS.md` | 后端分层、核心模块、接口清单、数据流、约定 | 改 Go 代码 |
| `apps/frontend/AGENTS.md` | 前端分层、数据流、页面套路、约定 | 改 React 代码 |
| `docs/architecture.md` | 取舍的**理由**、存储可替换性、扩展指引 | 要动架构或质疑某个设计 |
| `README.md` | 人看的快速开始与命令表 | 第一次跑起来 |

新加文档时遵循同一条规则：**范围互斥，同一事实只写一处**。重复描述必然漂移，
所以本文只给导航，机制细节一律交给子文档。
