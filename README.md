# GoWork

外贸 ERP 的 monorepo 骨架：Go 后端（gin + gorm + redis）+ React 前端，用 Taskfile 统一管理指令。

目录分两块，构建与运行彼此独立，靠 HTTP API 解耦：

```
GoWork/
├── Taskfile.yml              # 根编排：只调用子项目的 Taskfile
├── apps/
│   ├── backend/              # Go 服务
│   └── frontend/             # React 应用
├── deployments/
│   └── docker-compose.yml    # PostgreSQL + Redis（MySQL 走 profile）
└── docs/
    └── architecture.md       # 分层约定、存储可替换性、扩展指引
```

## 技术栈

| 层 | 选型 |
|---|---|
| HTTP | gin v1 |
| ORM | gorm（PostgreSQL / MySQL / SQLite 三驱动，配置切换） |
| 缓存 | go-redis v9 / 进程内实现（可落盘），配置切换 |
| 鉴权 | JWT（HS256）+ 服务端会话，登出即失效 |
| 日志 | 标准库 log/slog，结构化输出 |
| 前端 | React 19 + TypeScript + Vite + react-router |

## 快速开始

前置：Go ≥ 1.24、Node ≥ 20。Taskfile 需要 `task`（安装见文末）。

```bash
task install     # 装依赖（后端 go mod + 前端 npm install）
task dev         # 并行起后端 :8080 与前端 :5173
```

打开 http://localhost:5173，默认账号 **admin / admin123**。

首次启动会自动建表并写入演示数据（2 个供应商、1 个客户、4 个产品）。

没有 `task` 时等价操作：

```bash
cd apps/backend && go run ./cmd/server        # 后端
cd apps/frontend && npm install && npm run dev # 前端
```

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
| `task install` | 安装前后端依赖 |
| `task dev` | 并行启动前后端 |
| `task build` | 构建后端二进制 + 前端静态产物 |
| `task test` | 后端单元测试 |
| `task test:storage` | 三库两缓存的一致性测试（未启动的服务自动跳过） |
| `task lint` | `go vet` + 前端类型检查 |
| `task fmt` | gofmt |
| `task clean` | 清理 `bin/` 与 `dist/` |
| `task infra:up` / `infra:down` | 启停 PostgreSQL + Redis（MySQL 加 `--profile mysql`） |

子目录也有各自的任务：`task -d apps/backend -- --list`、`task -d apps/frontend -- --list`。

## API

统一响应格式：`{"code":0,"message":"ok","data":...,"request_id":"..."}`，`code != 0` 时按前三位对应 HTTP 状态码。

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/healthz` | 健康检查（免鉴权） |
| POST | `/api/v1/auth/login` | 登录 |
| GET | `/api/v1/auth/profile` | 当前用户 |
| POST | `/api/v1/auth/logout` | 登出 |
| GET | `/api/v1/dashboard/summary` | 首页统计 |
| GET/POST | `/api/v1/products` | 产品列表 / 新建 |
| GET/PUT/DELETE | `/api/v1/products/:id` | 产品详情 / 编辑 / 删除 |
| GET/POST | `/api/v1/suppliers` | 供应商列表 / 新建 |
| GET/PUT/DELETE | `/api/v1/suppliers/:id` | 供应商详情 / 编辑 / 删除 |
| GET/POST | `/api/v1/customers` | 客户列表 / 新建 |
| GET/PUT/DELETE | `/api/v1/customers/:id` | 客户详情 / 编辑 / 删除 |

除 `/healthz` 与 `/auth/login` 外，均需 `Authorization: Bearer <token>`。

列表接口通用查询参数：`page`、`page_size`（上限 200）、`keyword`，另有各模块的 `status`、`category` 等。

## 已实现模块

- **鉴权**：登录、登出、身份校验、登录失败限流（15 分钟内 5 次）
- **产品**：SKU 编码唯一校验、分类、供应商关联、成本/售价、软删除
- **供应商 / 客户**：编码唯一校验、关键字搜索、软删除
- **概览**：聚合统计，结果缓存 30 秒

订单、采购、库存、出货批次等模块尚未实现，扩展方式见 [docs/architecture.md](docs/architecture.md)。

## 安装 task

```bash
# 任选其一
go install github.com/go-task/task/v3/cmd/task@latest   # 装到 $(go env GOPATH)/bin
brew install go-task/tap/go-task                        # macOS
sudo snap install task --classic                        # Ubuntu
```
