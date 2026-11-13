# GoWork

外贸采购代理的 **ERP + CRM 集成系统**：Go 后端 + React 管理端，Taskfile 统一管理指令。
一套主数据（产品 / 往来单位）供 ERP 与 CRM 两侧共用，客户与供应商靠 `role` 位掩码合并在同一张表。

**职责**：怎么跑起来——前置版本、命令、默认账号、存储切换。
**不管**：目标与架构（[`AGENTS.md`](AGENTS.md)）、分层与接口（`apps/*/AGENTS.md`）、
术语（[`docs/glossary.md`](docs/glossary.md)）、进度（[`docs/roadmap.md`](docs/roadmap.md)）。
**相关**：`Taskfile.yml` 与 `apps/*/Taskfile.yml` 是命令的真实来源，本文只是索引。

## 前置

| 依赖 | 版本 | 说明 |
|---|---|---|
| Go | ≥ 1.26 | 见 `apps/server/go.mod` |
| Node | ≥ 20 | 前端 |
| pnpm | ≥ 11 | 版本锁在 `apps/web/package.json` 的 `packageManager` |
| `task` | 任意近期版本 | 安装方式见文末 |

```bash
corepack enable pnpm        # 之后 pnpm 按 packageManager 自动取对应版本
```

## 快速开始

```bash
task install            # 装依赖（后端 go mod + 前端 pnpm install）
task server:tool:air   # 一次性：装后端热重载工具 air（go install，不进 go.mod）
task dev                # 并行起后端 :8080（air 热重载）与前端 :5173（Vite HMR）
```

打开 <http://localhost:5173>，默认账号 **admin / admin123**。
空库启动会自动建表，并写入**基础数据**（默认管理员与币种汇率）——这一步没有开关，
关掉它只会让空库连登录都进不去。

**演示数据**由 `database.demo_seed` 控制：示例往来单位 / 产品 / 货源 / 条款模板
（含一个客供两用主体与一个个人主体），再往下是整条业务链
（报价 → 合同 → 订单 → 出货 → 应收 → 收付款 → 库存）。
它只在**空库**时写入，生产应为 `false`。想重来一遍：

```bash
task demo:reset    # 删掉 apps/server/data/gowork.db，重启后端即重新灌数
```

`demo_seed` 单独一个开关，是因为单元测试断言的是它们自己造的数据的合计 ——
库里预置几十张单会让它们全部对不上。

没有 `task` 时的等价操作：

```bash
cd apps/server && air -c .air.toml              # 不想热重载就 go run ./cmd/server
cd apps/web && pnpm install && pnpm run dev
```

`task server:dev` 报找不到 air，就是上面那条 `tool:air` 没执行（版本锁在
`apps/server/Taskfile.yml` 的 `AIR_VERSION`，配置见 `apps/server/.air.toml`）。

## 存储：默认零依赖

开发默认 **SQLite + 内存缓存**，不装任何数据库就能跑：`data/gowork.db`（WAL）与
`data/cache.snapshot`（登录态与限流计数的落盘快照），两者都在 `.gitignore` 里。

| 想跑的组合 | 执行 |
|---|---|
| SQLite + 内存（默认） | `task server:dev` |
| PostgreSQL + Redis | `task infra:up`，再 `task server:dev:pg` |
| MySQL + Redis | `task infra:up` + `docker compose -f docker-compose.dev.yml --profile mysql up -d`，再 `task server:dev:mysql` |

切换只改配置，不改代码。也可以用环境变量整段覆盖：

```bash
GOWORK_DB_DRIVER=postgres GOWORK_DB_HOST=127.0.0.1 GOWORK_DB_USERNAME=gowork \
GOWORK_DB_PASSWORD=gowork GOWORK_DB_NAME=gowork \
GOWORK_CACHE_DRIVER=redis GOWORK_REDIS_ADDR=127.0.0.1:6379 \
task server:run
```

优先级：配置文件 < `GOWORK_*` 环境变量。生产模板见 `configs/config.prod.yaml`
（单机部署用的是 `deployments/config.prod.yaml`）；
`database` 那段每个字段的含义、以及什么时候该改用整段 `dsn`，
见 [`docs/architecture.md`](docs/architecture.md) 第五节。

```bash
task test:storage   # postgres/mysql/redis 没起来时对应子用例自动跳过
```

## 生产部署（单机 Compose）

`deployments/docker-compose.yml` 起**两个容器**：`server`（Go API，只绑 `127.0.0.1`）
与 `web`（nginx，托管前端 `dist` 并把 `/api` 反代到 `server`，是这个栈唯一的对外入口）。
数据库与缓存用云托管或机房已有的 PostgreSQL / Redis，不跟应用一起起落；
TLS 证书与限流交给同一台机器外面的那层网关。

两个镜像由两份 Dockerfile 构建，随同一次发布一起推：后端是 `apps/server/Dockerfile`、
前端是 `apps/web/Dockerfile`，**构建上下文都是各自的子项目目录**，都不从仓库里读版本号。
后端的版本号只经 `--build-arg VERSION` 进去，不传就是 `dev`；本机想让它能自证就传
`git describe --always --long --dirty` 的结果（`task server:image` 已代劳）。两个镜像
不合并成一个，因此可以单独回滚。
前后端同源之后浏览器不发跨域请求，`config.prod.yaml` 的 `cors_origins` 只影响直连后端的场景。

配置分两处：应用自身的配置在 `deployments/config.prod.yaml`（库与缓存地址、CORS、
建表开关、日志格式），只读挂进容器覆盖镜像里的同名模板，改完重启即可；`.env` 只放
密钥、两个镜像名、对外端口与运行身份。

```bash
cp deployments/.env.example deployments/.env      # 填库与缓存的密码
docker compose -f deployments/docker-compose.yml up -d
docker compose -f deployments/docker-compose.yml logs -f server web
```

本机要自己构建这两个镜像（不拉 ghcr.io）时，在 `up` 后面加 `--build`；只重建其中一个：
`docker compose -f deployments/docker-compose.yml build server`（或 `web`）。

和开发环境（根目录 `docker-compose.dev.yml`，`task infra:up`）的区别：

| | 开发 | 生产 |
|---|---|---|
| 起哪些服务 | 只起库与缓存，应用在宿主机上跑 | 前端 + 后端两个容器，库与缓存是外部服务 |
| 端口 | 库 / 缓存都映射到本机，方便连工具 | 前端对外（默认 80），后端只绑 `127.0.0.1` |
| 配置 | `apps/server/configs/config.yaml`（可再用 `GOWORK_*` 覆盖） | `deployments/config.prod.yaml`，密钥走 `.env` |
| 运行身份 | 当前登录用户 | 默认 root；设 `PUID`/`PGID` 则降权并让 `/app` 属主对齐 |
| 建表 / 种子 | 每次启动按 `config.yaml` 决定 | 在 `config.prod.yaml` 里首启时打开，之后改回 `false` |

空库第一次启动要建表并创建 `admin` 账号：`config.prod.yaml` 的 `auto_migrate` 与 `seed`
已按首次部署打开，登录后在「账号维护」里改掉默认密码，再把这两项改回 `false` 重启。

## 命令

| 命令 | 作用 |
|---|---|
| `task` | 列出全部任务 |
| `task install` | 安装前后端依赖 |
| `task install:ci` | 按锁文件严格安装（前端 `--frozen-lockfile`） |
| `task dev` | 并行启动前后端（air + Vite HMR） |
| `task server:dev:once` | 后端单次启动、不监听变更 |
| `task server:tool:air` | 安装 air（一次性） |
| `task server:image` | 构建后端镜像（版本号缺省 `git describe --always --long --dirty`） |
| `task build` | 后端二进制 + 前端静态产物 |
| `task test` | 后端单元测试 |
| `task test:storage` | 三库两缓存一致性测试 |
| `task lint` | `go vet` + 前端类型检查 |
| `task fmt` | gofmt |
| `task release` | 发版：问版本号 → 同步前后端版本号 → 提交打 tag → 问是否推送 |
| `task clean` | 清理 `bin/` 与 `dist/` |
| `task infra:up` / `infra:down` | 启停开发用的 PostgreSQL + Redis（`docker-compose.dev.yml`；MySQL 加 `--profile mysql`） |

子目录也有各自任务：`task -d apps/server -- --list`、`task -d apps/web -- --list`。

## 发版

版本号正本是仓库根目录的 `VERSION.txt`（一行数字），`apps/web/package.json` 的 `version`
跟着一起改。产物不读这个文件，而是**构建期注进去** —— 二进制要能在任何目录、任何机器上
用 `gowork-server --version` 自证：

```
gowork-server 0.1.0
  commit: 7b2b231
  built:  2026-11-06T10:18:49Z
  go:     go1.27.3
```

| 优先级 | 来源 | 用在哪 |
|---|---|---|
| 1（最高） | `-X main.version=…` 链接参数 | 发布流水线把 tag 名注进去；`docker build --build-arg VERSION=` 同理，不传就是 `dev` |
| 2 | `task server:build APP_VERSION=1.2.3` | 本地试打某个号 |
| 3 | 根目录 `VERSION.txt` | `task server:build` 没显式给号时的缺省值（**只管本地编译，镜像不读它**） |
| 4（兜底） | 代码里的 `"dev"` | 裸 `go build` / air 热重载 / 没传 `VERSION` 的镜像构建，一眼看出不是发布产物 |

发版走 `task release`，它按顺序做完这几步，任一步失败就停下并说明停在哪：

| 步骤 | 做什么 | 出问题时的表现 |
|---|---|---|
| 前置校验 | 工作区干净、不在 detached HEAD、远程（若配置了）可达且没有同名 tag | 列出原因后退出，不留半成品 |
| 问版本号 | 回车 = 当前版本 +1 patch，也可以直接敲 `1.3.0` / `1.3.0-rc.1` | 输错当场重问 |
| 同步版本号 | 写 `VERSION.txt` 与 `apps/web/package.json` 的 `version` | 两处本来就是目标版本时跳过提交，只打 tag |
| 提交 + 打 tag | 一个「发布 vX.Y.Z」提交，加一个带注释的 `vX.Y.Z` tag | — |
| 问推送 | 回车即 `git push --atomic`，分支与 tag 一起推 | 选不推就只留本地提交与 tag，并把补推命令打在屏幕上 |

```bash
task release                      # 不带参数：先问版本号（回车 = 当前版本 +1 patch）
task release VERSION=1.3.0        # 指定版本号（带不带 v 前缀都认），不再问版本号
task release VERSION=1.3.0-rc.1   # 预发布，tag 仍是 v 前缀
task release REMOTE=upstream      # 推到别的远程（默认 origin；没配远程就跳过推送）
task release PUSH=false           # 只做本地部分（提交 + tag），不问也不推
task release DRY_RUN=true         # 只打印将要发生什么，不改文件、不提交、不推
```

推 tag 之后由 CI 出产物，本机不再参与：

| 工作流 | 触发时机 | 做什么 |
|---|---|---|
| `ci.yml` | 任意 `push`、`pull_request` | 后端 `build` / `vet` / `gofmt` / `test`，前端 `typecheck` / `build`，外加三库两缓存的一致性测试 |
| `release.yml` | 推 `v*` tag | 五个平台的后端包 + 前端 `dist` + `checksums.txt` + `ghcr.io/<repo>/server` 与 `ghcr.io/<repo>/web` 两个镜像 + Release |

两条流水线都不重写构建命令，一律调用 `Taskfile.yml` 里已有的任务。发布说明由提交历史
现算（`scripts/generate-changelog.sh`），不依赖 GitHub 的自动生成。

## 前端只认 pnpm

| 约定 | 说明 |
|---|---|
| 版本 | `packageManager: pnpm@12.10.1`，corepack 与 CI 都读这一处 |
| 锁文件 | 只有 `pnpm-lock.yaml`；`package-lock.json`、`yarn.lock` 已 gitignore |
| 安装 | 开发 `pnpm install`；CI / 发布 `pnpm install --frozen-lockfile` |
| 守卫 | 用 npm / yarn 会被 `preinstall` 直接拦下，这是有意的 |

一个反直觉行为：pnpm 默认 `minimumReleaseAge=1440`（24 小时），刚发布的版本会被跳过，
所以同样的 `^` 范围解析出的版本可能比 npm 低一版——这是供应链保护，不是锁文件坏了。
其余 pnpm 细节见 [`apps/web/AGENTS.md`](apps/web/AGENTS.md)。

## 安装 task

```bash
go install github.com/go-task/task/v3/cmd/task@latest   # 装到 $(go env GOPATH)/bin
brew install go-task/tap/go-task                        # macOS
sudo snap install task --classic                        # Ubuntu
```
