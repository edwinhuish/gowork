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
│   └── docker-compose.yml    # MySQL + Redis
└── docs/
    └── architecture.md       # 分层约定与扩展指引
```

## 技术栈

| 层 | 选型 |
|---|---|
| HTTP | gin v1 |
| ORM | gorm（MySQL / SQLite 双驱动） |
| 缓存 | go-redis v9，未配置时自动降级为进程内缓存 |
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

### 默认零依赖启动

`configs/config.yaml` 默认用 **SQLite + 进程内缓存**，不需要先装数据库就能跑。
`data/gowork.db` 是本地库文件，已在 `.gitignore` 中忽略。

要切到 MySQL + Redis：

```bash
task infra:up                                        # 起容器
export GOWORK_DB_DRIVER=mysql
export GOWORK_DB_DSN='gowork:gowork@tcp(127.0.0.1:3306)/gowork?charset=utf8mb4&parseTime=True&loc=Local'
export GOWORK_REDIS_ADDR=127.0.0.1:6379
cd apps/backend && go run ./cmd/server
```

配置优先级：`configs/config.yaml` < `GOWORK_*` 环境变量。生产模板见 `configs/config.prod.yaml`。

## Taskfile 命令

根目录：

| 命令 | 作用 |
|---|---|
| `task` | 列出全部任务 |
| `task install` | 安装前后端依赖 |
| `task dev` | 并行启动前后端 |
| `task build` | 构建后端二进制 + 前端静态产物 |
| `task test` | 后端单元测试 |
| `task lint` | `go vet` + 前端类型检查 |
| `task fmt` | gofmt |
| `task clean` | 清理 `bin/` 与 `dist/` |
| `task infra:up` / `infra:down` | 启停 MySQL + Redis |

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
