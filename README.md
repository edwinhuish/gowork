# GoWork

外贸采购代理的 **ERP + CRM 集成系统**：Go 后端 + React 管理端，Taskfile 统一管理指令。
一套主数据（产品 / 往来单位）供 ERP 与 CRM 两侧共用，客户与供应商靠 `role` 位掩码合并在同一张表。

**本文只讲「怎么跑起来」。** 目标、架构与模块划分见 [`AGENTS.md`](AGENTS.md)；
业务对象叫什么见 [`docs/glossary.md`](docs/glossary.md)；已实现到哪一步见
[`docs/roadmap.md`](docs/roadmap.md) 的进度表。

## 前置

| 依赖 | 版本 | 说明 |
|---|---|---|
| Go | ≥ 1.26 | 见 `apps/backend/go.mod` |
| Node | ≥ 20 | 前端 |
| pnpm | ≥ 11 | 版本锁在 `apps/frontend/package.json` 的 `packageManager` |
| `task` | 任意近期版本 | 安装方式见文末 |

```bash
corepack enable pnpm        # 之后 pnpm 按 packageManager 自动取对应版本
```

## 快速开始

```bash
task install            # 装依赖（后端 go mod + 前端 pnpm install）
task backend:tool:air   # 一次性：装后端热重载工具 air（go install，不进 go.mod）
task dev                # 并行起后端 :8080（air 热重载）与前端 :5173（Vite HMR）
```

打开 <http://localhost:5173>，默认账号 **admin / admin123**。
首次启动自动建表并写入演示数据（含一个客供两用主体与一个个人主体）。
两个开关在配置里：`database.auto_migrate`（建表）与 `database.seed`（演示数据），**生产两者都应为 false**。

没有 `task` 时的等价操作：

```bash
cd apps/backend && air -c .air.toml              # 不想热重载就 go run ./cmd/server
cd apps/frontend && pnpm install && pnpm run dev
```

`task backend:dev` 报找不到 air，就是上面那条 `tool:air` 没执行（版本锁在
`apps/backend/Taskfile.yml` 的 `AIR_VERSION`，配置见 `apps/backend/.air.toml`）。

## 存储：默认零依赖

开发默认 **SQLite + 内存缓存**，不装任何数据库就能跑：`data/gowork.db`（WAL）与
`data/cache.snapshot`（登录态与限流计数的落盘快照），两者都在 `.gitignore` 里。

| 想跑的组合 | 执行 |
|---|---|
| SQLite + 内存（默认） | `task backend:dev` |
| PostgreSQL + Redis | `task infra:up`，再 `task backend:dev:pg` |
| MySQL + Redis | `task infra:up` + `docker compose --profile mysql up -d`，再 `task backend:dev:mysql` |

切换只改配置，不改代码。也可以用环境变量整段覆盖：

```bash
GOWORK_DB_DRIVER=postgres GOWORK_DB_HOST=127.0.0.1 GOWORK_DB_USER=gowork \
GOWORK_DB_PASSWORD=gowork GOWORK_DB_NAME=gowork \
GOWORK_CACHE_DRIVER=redis GOWORK_REDIS_ADDR=127.0.0.1:6379 \
task backend:run
```

优先级：配置文件 < `GOWORK_*` 环境变量。生产模板见 `configs/config.prod.yaml`；
驱动差异与一致性保障（同一套断言跑三库两缓存）见 [`docs/architecture.md`](docs/architecture.md) 第五节。

```bash
task test:storage   # postgres/mysql/redis 没起来时对应子用例自动跳过
```

## 命令

| 命令 | 作用 |
|---|---|
| `task` | 列出全部任务 |
| `task install` | 安装前后端依赖 |
| `task install:ci` | 按锁文件严格安装（前端 `--frozen-lockfile`） |
| `task dev` | 并行启动前后端（air + Vite HMR） |
| `task backend:dev:once` | 后端单次启动、不监听变更 |
| `task backend:tool:air` | 安装 air（一次性） |
| `task build` | 后端二进制 + 前端静态产物 |
| `task test` | 后端单元测试 |
| `task test:storage` | 三库两缓存一致性测试 |
| `task lint` | `go vet` + 前端类型检查 |
| `task fmt` | gofmt |
| `task clean` | 清理 `bin/` 与 `dist/` |
| `task infra:up` / `infra:down` | 启停 PostgreSQL + Redis（MySQL 加 `--profile mysql`） |

子目录也有各自任务：`task -d apps/backend -- --list`、`task -d apps/frontend -- --list`。

## 前端只认 pnpm

| 约定 | 说明 |
|---|---|
| 版本 | `packageManager: pnpm@12.10.1`，corepack 与 CI 都读这一处 |
| 锁文件 | 只有 `pnpm-lock.yaml`；`package-lock.json`、`yarn.lock` 已 gitignore |
| 安装 | 开发 `pnpm install`；CI / 发布 `pnpm install --frozen-lockfile` |
| 守卫 | 用 npm / yarn 会被 `preinstall` 直接拦下，这是有意的 |

一个反直觉行为：pnpm 默认 `minimumReleaseAge=1440`（24 小时），刚发布的版本会被跳过，
所以同样的 `^` 范围解析出的版本可能比 npm 低一版——这是供应链保护，不是锁文件坏了。
其余 pnpm 细节见 [`apps/frontend/AGENTS.md`](apps/frontend/AGENTS.md)。

## 安装 task

```bash
go install github.com/go-task/task/v3/cmd/task@latest   # 装到 $(go env GOPATH)/bin
brew install go-task/tap/go-task                        # macOS
sudo snap install task --classic                        # Ubuntu
```
