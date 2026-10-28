# GoWork —— 项目总览（面向 AI 协作者）

**职责**：入口。读完它应当能回答三个问题——这个项目要解决什么、代码在哪、改某类东西该动哪一层。
**不管**：机制细节与操作步骤（`apps/backend/AGENTS.md`、`apps/frontend/AGENTS.md`）、
取舍理由（`docs/architecture.md`）、术语（`docs/glossary.md`）、进度（`docs/roadmap.md`）。
**约定**：同一事实只写一处；本文只给导航与跨层约束，细节一律交给第 8 节列出的文档。

---

## 1. 项目目标

外贸采购代理的 **ERP + CRM 集成系统**。一条主数据（产品 / 往来单位），
两侧业务：ERP 管采购、库存、出货与订单，CRM 管联系人、跟进与商机。

**集成发生在数据层，不是两个系统对接。** CRM 里跟进的客户与 ERP 里开票的客户
是同一行 `md_partner` 记录——没有同步任务，也没有第二份客户表。
域边界与判断标准见 `docs/glossary.md` 第二节。

**客户与供应商也是同一张表。** 它们是"往来单位"的两种角色，用 `role` 位掩码区分，
所以"既是客户又是供应商"的主体只有一条记录、一套联系方式；货代（含报关行）同样
落在这张表上，只是角色位不同。`/customers`、`/suppliers` 与 `/forwarders` 是同一资源
按角色切的入口，不是两套实现。

**当前阶段：沿业务主链推进。** 已实现鉴权、共享主数据（产品 / 货源 / 往来单位 /
币种汇率）、CRM（联系人 / 跟进 / 线索 / 商机）、ERP 交易（报价单 / 销售订单 /
采购订单）、出货批次与报关单、库存流水快照与出入库单（采购入库 / 销售出库 / 盘点）、
应收应付与收付款核销、报表（账龄分桶 / 单票毛利 / 汇兑损益）、订单确认时的客户信用额度拦截。
合同单据与仓间调拨、销售看板**均未实现**，进度与下一步见 `docs/roadmap.md`，
扩展套路见 `docs/architecture.md` 第六节。

**改代码前先查 `docs/glossary.md`。** 它规定了每个业务对象该叫什么名、
归哪个域、哪些词不许用——那里面每一条都是踩过的坑。

**刻意不做的事**（不要"顺手"加上）：

| 不做 | 原因 |
|---|---|
| 多租户 / 字段级权限矩阵 | 单租户单库，不做租户隔离；字段级权限留到有真实角色诉求时再做。但**单租户内的数据范围（`owner_id`）要做**，见 `docs/trade-erp-crm-blueprint.md` 的 I2 |
| 微服务拆分 | 单进程足够；分层已经让边界清楚，拆进程是部署问题不是代码问题 |
| 给 CRM 建独立的客户表 | 客户被两侧同时写，拆表就要双向同步，集成从数据层退化成消息层 |
| 把客户与供应商拆回两张表 | 分开时"同一主体两头都有"就是两行数据、两套联系方式、两个编码，对账要人工确认是同一家 |
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
│       ├── src/                 # pages / providers / api / components / constants / utils / types
│       └── pnpm-workspace.yaml  # pnpm 设置（不是工作区划分）
├── deployments/
│   └── docker-compose.yml       # 本地 PostgreSQL + Redis（MySQL 走 profile）
├── docs/
│   ├── glossary.md              # 术语表：业务对象叫什么、归哪个域（唯一事实来源）
│   └── architecture.md          # 分层约定、存储可替换性、扩展指引
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
| 鉴权 | JWT HS256 + 服务端会话，滑动续期 | 只有签名不够——登出要能立即失效，所以会话必须落在缓存里；剩余有效期不足一半时自动换发新令牌 |
| 配置 | yaml + `GOWORK_*` 环境变量 | 12-factor；敏感值走环境变量，不进仓库 |
| 日志 | 标准库 `log/slog` | 结构化日志不需要第三方库 |
| 前端 | React 19 + TypeScript + Vite 6 + refine 5 + Ant Design 5 + react-router 7 | 管理端 CRUD 的表单、表格、鉴权路由都由 refine 统一，手写会在多模块间走形 |
| 包管理 | pnpm 12（`packageManager` 锁定） | 严格 node_modules，未声明的依赖当场报错 |
| 任务编排 | Taskfile | 跨平台，比 shell 脚本可读；命令下沉到子目录避免重复维护 |

## 5. 模块划分

**后端**（细节见 `apps/backend/AGENTS.md`）：

| 模块 | 域 | 路由前缀 | 说明 |
|---|---|---|---|
| auth | 系统 | `/api/v1/auth` | 登录、当前用户、登出；登录失败限流 |
| dashboard | 系统 | `/api/v1/dashboard` | 跨域聚合统计，结果缓存 30 秒 |
| product | 共享主数据 | `/api/v1/products` | 产品，SKU 编码唯一 |
| product-source | 共享主数据 | `/api/v1/product-sources` | 产品货源：某供应商处的货号、采购价、交期 |
| partner | 共享主数据 | `/api/v1/partners` | 往来单位：客户与供应商的统一主体，角色用位掩码；`/:id/credit` 读额度占用 |
| partner（角色切片） | 同上 | `/api/v1/customers`、`/api/v1/suppliers`、`/api/v1/forwarders` | 同一份数据按角色切的入口，不是两套实现 |
| currency / exchange-rate | 共享主数据 | `/api/v1/currencies`、`/api/v1/exchange-rates` | 币种与历史汇率；汇率不可改，录错删掉重录 |
| contact / activity | CRM | `/api/v1/contacts`、`/api/v1/activities` | 联系人、跟进记录（`?due=1` 查已到期未跟进） |
| lead / opportunity | CRM | `/api/v1/leads`、`/api/v1/opportunities` | 线索可转化为主体；商机带阶段机与加权预测 |
| quotation | ERP · 交易 | `/api/v1/quotations` | 报价单：成本快照、版本、转订单（`/quotations-preview` 试算不落库） |
| sales-order | ERP · 交易 | `/api/v1/sales-orders` | 销售订单：合计只由明细汇总，状态走完整状态机；确认时校验信用额度（超额 409，admin 可强制放行） |
| purchase-order | ERP · 交易 | `/api/v1/purchase-orders` | 采购订单：明细可挂销售订单行做以销定采 |
| shipment | ERP · 供应链 | `/api/v1/shipments` | 出货批次：一票装哪些销售订单行、各多少；可出行的来源走 `/shipment-candidates` |
| shipment-charge | ERP · 供应链 | `/api/v1/shipment-charges` | 运费杂费：归集到一票货的成本，单票毛利的减项；合计走 `/shipment-charges-summary` |
| customs-declaration | ERP · 供应链 | `/api/v1/customs-declarations` | 报关单：一票可多张（分票申报 / 退单重报）；物流节点走 `PUT /shipments/:id/logistics` |
| receivable / payable | ERP · 财务 | `/api/v1/receivables`、`/api/v1/payables` | 应收应付：由订单生成、一期一张单、汇率随单据锁定 |
| payment | ERP · 财务 | `/api/v1/payments` | 收付款单与核销行：收款核应收、付款核应付；汇率取到账日，差额是汇兑损益 |
| report | 系统 · 报表 | `/api/v1/reports/*` | 只读聚合：账龄（按主体分桶）、单票毛利（收入 − 采购 − 运杂费）、汇兑损益（逐笔核销）；不跟随单据状态机 |
| stock | ERP · 库存 | `/api/v1/stock-movements`、`/api/v1/stock-snapshots` | 流水是库存的唯一事实来源；快照只由流水推出来，接口只有读与重算 |
| stock-document | ERP · 库存 | `/api/v1/stock-documents` | 出入库单（采购入库 / 销售出库 / 盘点）：确认那一刻才生成流水 |

**前端**（细节见 `apps/frontend/AGENTS.md`）：主数据、CRM、单据各自成模块，
每个模块是 refine 的 `list / create / edit / show` 四页，新建与编辑共用
`form-fields.tsx` 的字段定义；单据类另有明细 / 分配行编辑器。
客户与供应商两个模块的类型都是 `Partner`，只是默认角色不同。

**已实现**：`SalesOrder` / `PurchaseOrder` / `Shipment`（出货批次，含分配行与物流节点）、
`ShipmentCharge`（运费杂费）、`CustomsDeclaration`（报关单）、
`Receivable` / `Payable`（应收应付，一期一张单）、
`Payment` / `PaymentAllocation`（收付款与核销，核销行只增不改）、
`StockMovement` / `StockSnapshot` / `StockDocument`（库存流水、快照与出入库单），
CRM 侧的 `Contact` / `Activity` / `Opportunity` / `Lead`。

**未实现但已定名的模块**（先看术语表再加，别另起名字）：
`Contract`（PI 与合同）。

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
   最后在 `bootstrap/app.go` 装配一次。步骤与硬性规则见 `apps/backend/AGENTS.md` 第 8 节，
   为什么这么排见 `docs/architecture.md` 第六节。
   **起名和定表前缀之前先查 `docs/glossary.md`**：未实现的模块名已经定好了，
   自己换一个叫法会让前后端和文档立刻分叉。

## 7. 运行

```bash
task dev     # 后端 :8080（air 热重载）+ 前端 :5173（Vite HMR）
task test    # 后端单元测试
task lint    # go vet + 前端 tsc --noEmit
```

默认账号 `admin / admin123`；默认配置是 SQLite + 内存缓存，零外部依赖。
前置版本、完整命令表、存储切换（PostgreSQL / MySQL / Redis）见 `README.md`。

## 8. 文档地图

九份文档各有一个**唯一职责**：同一事实只允许在对应的那一份里展开，其余处只写一句话加指向。
新加文档时先在下面这张表里占一行，再写正文——没有归口的内容，写下来也活不过两次改动。

| 文件 | 唯一职责 | 什么时候读 |
|---|---|---|
| `AGENTS.md`（本文） | 目标、整体架构、模块划分、跨层约定 | 第一次接触项目 |
| `README.md` | 怎么跑起来：前置版本、命令、存储切换 | 第一次跑起来 |
| `docs/glossary.md` | **术语与域模型**：业务对象叫什么、归哪个域、哪些词不许用、命名规则 | 写任何新代码或文档之前 |
| `docs/architecture.md` | 分层与取舍的**理由**、存储可替换性 | 要动架构或质疑某个设计 |
| `docs/trade-erp-crm-blueprint.md` | **能力地图**：要做什么、为什么做、优先级判据（不含任务编号与进度） | 决定"做什么、先做哪个"时 |
| `docs/roadmap.md` | **唯一的任务表**：编号、交付物、验收标准、依赖、优先级、进度 | 每开一个新任务之前 |
| `docs/gap-analysis.md` | **差距与整改**：与主流 ERP / CRM 的能力差距证据、架构评估、为什么建议补某条 | 要补功能、评估重构或质疑优先级时 |
| `apps/backend/AGENTS.md` | 后端手册：分层、接口清单（唯一处）、数据流、约定、陷阱 | 改 Go 代码 |
| `apps/frontend/AGENTS.md` | 前端手册：分层、数据流、页面套路、约定、陷阱 | 改 React 代码 |

**格式约定**（三份人读手册与业务文档分开编号，避免 `## 3.` 同时表示两种意思）：

- `README.md` 与两份 `AGENTS.md` 用阿拉伯数字：`## 1. 职责边界`；
- `docs/` 下的业务文档用中文数字：`## 一、分层与依赖方向`；
- 每份文档开头三行固定写「**职责** / **不管** / **相关**」，正文里不再重复这三件事。

