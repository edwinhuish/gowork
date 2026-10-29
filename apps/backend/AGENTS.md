# apps/backend —— Go 服务（面向 AI 协作者）

**职责**：后端手册——代码怎么组织、改东西动哪、为什么这么写会报错。**接口清单全项目只有第 5 节这一份。**
**不管**：项目目标与整体架构（根 `AGENTS.md`）、取舍理由（`docs/architecture.md`）、
业务对象命名（`docs/glossary.md`）、前端（`apps/frontend/AGENTS.md`）。
**相关**：`docs/roadmap.md` 第三节（每个任务收尾要跑的命令）。

---

## 1. 职责边界

**负责**：HTTP API、鉴权、共享主数据（产品 / 供应商 / 客户）、统计聚合、
数据库与缓存访问。

**ERP 与 CRM 是这个服务的两个域，不是两个服务。** 已落地共享主数据、CRM（联系人 / 跟进 /
线索 / 商机）、ERP 交易（报价 / 销售订单 / 采购订单）、供应链（出货、报关、库存流水与快照、
出入库单）与财务（应收应付、收付款与核销）、报表（账龄、单票毛利、汇兑损益）、
订单确认时的信用额度拦截；销售看板与对账单等报表尚未开始，差距清单见 `docs/gap-analysis.md`。
域归属见 `docs/glossary.md` 第二节，判断标准是"这张表会不会被两个域
同时写"——会，就放 `md_` 下。

**不负责**：

| 不属于这里 | 归哪 |
|---|---|
| 页面渲染、静态资源 | `apps/frontend` |
| 权限矩阵、多租户 | 当前不做（见根 `AGENTS.md` 第 1 节） |
| 库存 / 出入库单据 | **已实现**（`inv_` 前缀）：流水是事实、快照是缓存；调拨待仓库维度（T-16b）。差距见 `docs/gap-analysis.md` |
| 账龄、汇兑损益、报表 | 未实现；口径与名称已在 `docs/glossary.md`、`docs/roadmap.md` 定好 |

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
        ├── pdf/                 PDF 渲染：模板加载 + gopdf 绘制
        │   └── templates/       版式（JSON，可被 pdf.template_dir 覆盖）；
        │                        带 label 段的是箱唛，其余按单据模板解析
        └── response/            统一响应体
```

测试与实现同目录：`config/config_test.go`、`infra/cache/cache_test.go`、
`repository/storage_test.go`、`migrate/legacy_merge_test.go`、`migrate/rename_legacy_test.go`、
`migrate/rename_shipment_docno_test.go`，
以及 `service/` 下的 `*_test.go`——它们用 `service/testdb_test.go` 起的**真实 SQLite 临时库**，
不 mock 仓储；`partner` / `product` / `dashboard` 等 service 还没有测试。

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
| `repository` | 泛型 `Base[T]` 提供 Create / UpdateFields / DeleteByID / FindByID / Exists / Count / Paginate / PaginateOrdered / PaginateOrderedWithPreload，以及 `DB()` 与 `WithTx`；查询条件用 `Scope` 解耦 | 新增实体嵌 `Base[T]` 复用，只写自己独有的查询；复杂查询用 `r.DB()`；`WithTx` 目前没有调用点（接入计划见 T-42） |
| `service` | 每个领域一个接口 + 私有实现，构造函数用 **Option 模式**注入 logger 等可选依赖 | 业务规则（唯一性、状态、金额）只写在这一层 |
| `handler` | 只做 `ShouldBind*` + 调 service + `response.OK/Fail` | 不要在这里写 if 业务分支 |
| `pkg/errs` | 错误码前三位对齐 HTTP 状态码；`Wrap` 保留底层错误用于日志 | 新增码沿用规则，`response.Fail` 自动映射状态码 |
| `pkg/response` | 全站唯一响应体 `{code,message,data,request_id}` | 加字段要考虑前端 `api/client.ts` 的拆包逻辑 |
| `bootstrap` | 所有 `New` 集中在此，装配顺序：config → logger → db → 表改名迁移 → auto_migrate → migrate → cache → repo → service → handler → router → seed | 新模块只改这一个文件就能接上线 |

**分页有三处同名但不同的东西**，别混：`dto.PageQuery`（HTTP 入参）、
`repository.Pagination`（数据层入参）、`dto.PageResult[T]`（响应体）。转换在 service 层做。

## 5. 接口清单

统一前缀 `/api/v1`。除 `POST /auth/login` 与 `/healthz` 外都要 `Authorization: Bearer <token>`。

机器可读的完整清单是 `docs/openapi.json`（从路由表生成、CI 校验，`task backend:openapi` 重新生成）。
下面这张表是给人看的：它额外记了每个接口背后的 service 方法与行为约束，那些生成不出来。

域归属见 `docs/glossary.md` 第三节：`products` 与 `partners` 会被 ERP、CRM 两侧引用。
新增接口时先确认它落在哪个域，再决定表前缀。

**本表写代码现状**：编号一栏是当前实现，与目标口径（蓝图 §五.4）不一致的地方已就地标注（如 shipments 的 `SH-`）。T-47 落地前**以本表为准**，落地时再删掉标注。

| 方法 | 路径 | handler | service |
|---|---|---|---|
| GET | `/healthz` | `handler.Health` | — |
| POST | `/api/v1/auth/login` | `Auth.Login` | `AuthService.Login` |
| GET | `/api/v1/auth/profile` | `Auth.Profile` | 取中间件注入的 `Principal` |
| POST | `/api/v1/auth/logout` | `Auth.Logout` | `AuthService.Logout` |
| GET | `/api/v1/dashboard/summary` | `Dashboard.Summary` | `DashboardService.Summary`（缓存 30s） |
| GET | `/api/v1/reports/aging` | `Report.Aging` | `ReportService.Aging`：按「主体 + 币种」分桶，到期当天不算逾期，合计只给本币 |
| GET | `/api/v1/reports/shipment-profits` | `Report.ShipmentProfits` | `ReportService.ShipmentProfits`：收入 − 采购 − 运杂费（本币）；`?shipment_id=` 取单票；缺采购标 `cost_missing` |
| GET | `/api/v1/reports/exchange-gains` | `Report.ExchangeGains` | `ReportService.ExchangeGains`：逐笔核销的到账汇率 vs 单据汇率；应收记收益、应付记损失；`from`/`to` 按到账日闭区间 |
| GET | `/api/v1/reports/collection` | `Report.Collection` | `ReportService.Collection`：回款率 = 期间**到账并核销**（按到账日）÷ 期间**到期**（按到期日），两批单据可以不同批，比率可超 100%；超期明细逐笔到单据号，按逾期天数倒序，未到期不计入明细；分桶复用账龄规则；`limit` 截断时合计与分桶仍是全量 |
| GET | `/api/v1/reports/sales-dashboard` | `Report.SalesDashboard` | `ReportService.SalesDashboard`：`dimension=customer\|product\|country\|month`，收入 / 成本 / 运杂费先摊到**出货行**再分组，四个维度合计必然相等且等于单票毛利之和；订单额按下单日、毛利按出货日；`salesperson` 回 400（等 T-32 的 `owner_id`）；合计不受 `limit` 截断 |
| GET / POST | `/api/v1/products` | `Product.List` / `.Create` | `ProductService.List` / `.Create`：`keyword` 同时匹配 SKU、中英文品名与**全部编号**（子查询 `md_product_code` 与 `md_product_source.supplier_sku_norm`），命中时列表项带 `matched_codes` |
| GET / PUT / DELETE | `/api/v1/products/:id` | `Product.Get` / `.Update` / `.Delete` | 同名方法；改 SKU 时旧 SKU 自动沉淀为一条编号（`source=sku`），新 SKU 不得等于其他产品用过的编号（409）；删除产品在同一事务里清掉它的编号，旧号可立即复用 |
| POST | `/api/v1/products/:id/restore` | `Product.Restore` | 把软删标记写回 0；`?include_deleted=1` 的列表里能看到已删行。SKU 已被别的产品占用时 409，对未删除的记录调用是 400 |
| GET / POST | `/api/v1/product-codes` | `ProductCode.List` / `.Create` | 产品编号（旧 SKU / 客户物料号 / 供应商货号 / 工厂号）：`?product_id=` 必填；归一化后同产品重复 409、跨产品可共用；`code_norm` 由服务端算 |
| DELETE | `/api/v1/product-codes/:id` | `ProductCode.Delete` | **硬删**：软删会让「删掉再加回来」撞唯一索引，而列表里又看不见那行 |
| GET / POST | `/api/v1/partners` | `Partner.List(nil)` / `.Create(nil)` | `PartnerService.List` / `.Create` |
| GET / PUT / DELETE | `/api/v1/partners/:id` | `Partner.Get(nil)` / `.Update(nil)` / `.Delete(nil)` | 同名方法 |
| POST | `/api/v1/partners/:id/restore` | `Partner.Restore` | 恢复已软删的主体；只挂这一个入口，角色已在记录里，不再按 `/customers`、`/suppliers` 各复制一份 |
| GET | `/api/v1/partners/:id/credit` | `Partner.Credit` | `CreditService.Summary`：额度、未结清应收、未开票订单、敞口；额度 0 = 不限 |
| POST | `/api/v1/partners/:id/merge` | `Partner.Merge` | 把 `source_id` 并入 `:id`（主记录）：角色取并集、空字符串字段从被合并方补、引用按 `repository/partnerRefTables` 整体迁移（含 `sys_attachment`），被合并方软删；主记录已有主联系人时把被合并方的降级；货源同产品同货号冲突返回 409；带 `updated_at` 走乐观锁 |
| GET | `/api/v1/partner-duplicates` | `Partner.Duplicates` | 全库查重分组：名称+国家 / 邮箱同域 / 电话尾号三条规则各扫一遍，只返回 ≥2 条的分组。**不挂在 `/partners` 下**——gin 同一层不允许 `:id` 与静态段并存（会 panic），与 `/shipment-candidates` 同一处理 |
| GET | `/api/v1/partner-duplicates/candidates` | `Partner.DuplicateCandidates` | `?name=&country=&email=&phone=&exclude_id=`：录入时的候选提示，一条主体只返回一次（命中几条规则就把原因并起来）；免费邮箱与空值不参与；只提示不阻断 |
| GET / POST | `/api/v1/customers` | `Partner.*(&customerRole)` | 同上，路由注入角色 |
| GET / POST | `/api/v1/suppliers` | `Partner.*(&supplierRole)` | 同上 |
| GET / POST | `/api/v1/forwarders` | `Partner.*(&forwarderRole)` | 同上，角色位 `4`（货代 / 报关行） |
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
| GET / POST | `/api/v1/leads` | `Lead.List` / `.Create` | `LeadService.List` / `.Create`（按创建时间倒序） |
| GET / PUT / DELETE | `/api/v1/leads/:id` | `Lead.Get` / `.Update` / `.Delete` | 同名方法（已转化不能靠改 status 完成） |
| POST | `/api/v1/leads/:id/convert` | `Lead.Convert` | 建主体 + 回填 `partner_id`；重复转化 409，同名同国只 warning |
| GET / POST | `/api/v1/product-sources` | `Source.List` / `.Create` | `ProductSourceService.List` / `.Create`（`?product_id=` 过滤，按本币价升序） |
| GET / PUT / DELETE | `/api/v1/product-sources/:id` | `Source.Get` / `.Update` / `.Delete` | 同名方法（不可改产品与供应商） |
| GET / POST | `/api/v1/quotations` | `Quotation.List` / `.Create` | `QuotationService.List` / `.Create`（单号由案号派生，见「单号」；合计由 service 算） |
| GET / PUT / DELETE | `/api/v1/quotations/:id` | `Quotation.Get` / `.Update` / `.Delete` | 只有草稿可改可删，其余返回 409 |
| PUT | `/api/v1/quotations/:id/status` | `Quotation.UpdateStatus` | 只放行已确认 / 已取消，其余状态 400 |
| POST | `/api/v1/quotations/:id/versions` | `Quotation.NewVersion` | 同单号、版本 +1、回到草稿；过期时须给新有效期 |
| POST | `/api/v1/quotations/:id/convert` | `Quotation.Convert` | 生成销售订单草稿并回填 `sales_order_id`；重复转换 409，条件更新保证幂等 |
| POST | `/api/v1/quotations-preview` | `Quotation.Preview` | 试算金额/成本/毛利，不落库；独立前缀避免与 `/:id` 抢路径 |
| GET / POST | `/api/v1/contracts` | `Contract.List` / `.Generate` | 合同（PI / 销售合同共用一张表）：**只能从报价单或订单生成**，无空白新建；客户、币种、汇率、总额照抄来源；单号 `PI-` / `SC-`；分期合计必须等于总额（400），同来源同类型重复生成 409 |
| GET / PUT / DELETE | `/api/v1/contracts/:id` | `Contract.Get` / `.Update` / `.Delete` | 只有草稿可改可删；来源、客户与金额不在可改字段里，要改就改来源单据后重新生成 |
| PUT | `/api/v1/contracts/:id/status` | `Contract.UpdateStatus` | 只放行已确认 / 已取消（400）；确认前复核来源单据未取消、分期合计与总额相符 |
| GET | `/api/v1/quotations/:id/pdf`、`/api/v1/contracts/:id/pdf` | `PDF.Quotation` / `.Contract` | 返回 `application/pdf` 附件，**不走统一响应体**（失败仍走）；版式在 `internal/pkg/pdf/templates/*.json`，改版式不改代码；中文需 `pdf.font_path` 指向同时含拉丁与汉字的 `.ttf`，缺字体时打印不可用但服务照常启动 |
| GET | `/api/v1/shipments/:id/invoice`、`/api/v1/shipments/:id/packing-list` | `PDF.Invoice` / `.PackingList` | 商业发票与装箱单：内部编号取送货单的套号（`doc_no`），改证后带 `-R{n}`，文件名 `{套号}-CI.pdf` / `-PL.pdf`，**不单独发号**；明细来自分配行（发票金额按出货数量 × 订单单价） |
| GET | `/api/v1/shipments/:id/carton-labels` | `PDF.CartonLabels` | 箱唛：一箱一张（张数按分配的 `cartons`，没填时给 1 张），主标识印销售订单号、套号当发票号，箱号与毛净重留白人工填；文件名 `{套号}-MARK.pdf`；混装箱不生成；单次上限 2000 张 |
| GET / POST | `/api/v1/sales-orders` | `SalesOrder.List` / `.Create` | 合计只由明细汇总（入参无总额字段）；单号 `{案号}-SO{NN}`，从报价转来时沿用来源的案号与序号 |
| GET / PUT / DELETE | `/api/v1/sales-orders/:id` | `SalesOrder.Get` / `.Update` / `.Delete` | 只有草稿可改可删，其余 409 |
| PUT | `/api/v1/sales-orders/:id/status` | `SalesOrder.UpdateStatus` | 走完整状态机，跳步/回退由 `checkDocumentTransit` 拒绝（409）；确认（10→20）时校验信用额度，超额 409，`force_credit` 仅 admin |
| GET | `/api/v1/sales-orders/:id/changes` | `SalesOrder.Changes` | 变更历史，按序号倒序；订单不存在时 404，而不是返回空历史 |
| POST | `/api/v1/sales-orders/:id/changes` | `SalesOrder.Change` | 已确认/备货中的订单改量改价改交期：原因必填、操作人从 ctx 取、改后数量不得低于已出货量（409）、不允许增删行、带 `updated_at` 走乐观锁 |
| GET / POST | `/api/v1/purchase-orders` | `Purchase.List` / `.Create` | 单号 `PO-{YYMM}-{NNN}`；明细可带 `sales_order_item_id` 做以销定采 |
| GET / PUT / DELETE | `/api/v1/purchase-orders/:id` | `Purchase.Get` / `.Update` / `.Delete` | 只有草稿可改可删，其余 409 |
| PUT | `/api/v1/purchase-orders/:id/status` | `Purchase.UpdateStatus` | 与销售订单共用状态机 |
| GET / POST | `/api/v1/shipments` | `Shipment.List` / `.Create` | 单号 `DN-{YYMM}-{NNN}`，同时是这一票的套号；分配挂销售订单**行**，数量不得超过未出数量（409）；一票只能装同一客户、同一币种的订单（400）；分配行可带装箱数据（`cartons` / `gross_weight` / `net_weight` / `measurement`，可留空，只拦负值与净重 > 毛重） |
| GET / PUT / DELETE | `/api/v1/shipments/:id` | `Shipment.Get` / `.Update` / `.Delete` | 只有草稿可改可删，其余 409 |
| PUT | `/api/v1/shipments/:id/status` | `Shipment.UpdateStatus` | 与其它单据共用状态机；取消即把数量还给订单行 |
| POST | `/api/v1/shipments/:id/reissue` | `Shipment.Reissue` | 改证重开：号码不变、`invoice_revision` 自增（无入参）；草稿 409（内容还能直接改）、已取消 409 |
| GET | `/api/v1/shipment-candidates` | `Shipment.Candidates` | 可出货的订单行（已确认 / 备货中且未出 > 0）；编辑页传 `exclude_shipment_id` 把本票占的数量还回来 |
| GET / POST | `/api/v1/shipment-charges` | `Charge.List` / `.Create` | 运费杂费挂到一票货；**不校验出货状态**（账单晚于出货到达）；外币按当日汇率折本币 |
| GET / PUT / DELETE | `/api/v1/shipment-charges/:id` | `Charge.Get` / `.Update` / `.Delete` | 不能改挂到另一票货；有费用的批次拒绝删除（409） |
| GET | `/api/v1/shipment-charges-summary` | `Charge.Summary` | 一票的本币费用合计；独立前缀避免与 `/:id` 抢路径 |
| PUT | `/api/v1/shipments/:id/logistics` | `Shipment.UpdateLogistics` | 提单号 / 船名航次 / ETD / ETA；**任何状态都能改**（船期订舱后才有）；ETA 早于 ETD 拒绝 |
| GET / POST | `/api/v1/customs-declarations` | `Customs.List` / `.Create` | 报关单，一票可多张；报关单号可为空（待申报）；不做状态机 |
| GET / PUT / DELETE | `/api/v1/customs-declarations/:id` | `Customs.Get` / `.Update` / `.Delete` | 不能改挂到另一票货；有报关单的批次拒绝删除（409） |
| GET / POST | `/api/v1/receivables` | `Receivable.List` / `.Create` | 由销售订单生成；币种与汇率从订单**锁定**；同一来源累计不超订单总额（409）；金额留空取余额 |
| GET / PUT / DELETE | `/api/v1/receivables/:id` | `Receivable.Get` / `.Update` / `.Delete` | 有核销后不可改金额、不可删；手改状态只支持作废（90） |
| GET / POST | `/api/v1/payables` | `Payable.List` / `.Create` | 由采购订单生成，规则同应收；采购汇率与销售汇率各锁一份 |
| GET / PUT / DELETE | `/api/v1/payables/:id` | `Payable.Get` / `.Update` / `.Delete` | 同上 |
| GET / POST | `/api/v1/payments` | `Payment.List` / `.Create` | 收付款单 + 核销行，单号 `PM-`；收款只核应收、付款只核应付；汇率取**到账日**（不是应收锁的）；超收在仓储事务内拒绝（400） |
| GET / PUT / DELETE | `/api/v1/payments/:id` | `Payment.Get` / `.Update` / `.Delete` | 只能改日期 / 方式 / 水单号 / 备注；金额与核销明细不可改，删单时同事务回滚目标的已核销金额与状态 |
| GET / POST | `/api/v1/stock-movements` | `Stock.ListMovements` / `.CreateMovement` | 单号 `MV-`；只能手工录期初 / 盘点盈亏 / 其它，采购入库与销售出库由单据产生（400） |
| GET / DELETE | `/api/v1/stock-movements/:id` | `Stock.GetMovement` / `.DeleteMovement` | 删除同事务回滚快照；单据产生的流水改 409，请回来源单据处理 |
| GET | `/api/v1/stock-snapshots` | `Stock.ListSnapshots` | 当前库存，只读；SKU / 品名由 join 产品取当前值 |
| POST | `/api/v1/stock-snapshots/recalc` | `Stock.Recalc` | 用流水重算快照（`product_id` 可选），并报告重算前的不一致 |
| GET / POST | `/api/v1/stock-documents` | `StockDoc.List` / `.Create` | 出入库单：单号前缀 `IN-` / `OUT-` / `CT-`；草稿不动库存 |
| GET / PUT / DELETE | `/api/v1/stock-documents/:id` | `StockDoc.Get` / `.Update` / `.Delete` | 只有草稿可改可删；类型建好后不可改 |
| POST | `/api/v1/stock-documents/:id/confirm` | `StockDoc.Confirm` | 生成流水 + 更新快照（一个事务）；重复确认 409 |
| GET / POST | `/api/v1/attachments` | `Attachment.List` / `.Upload` | 通用附件（`sys_attachment` + `sys_attachment_content`）：`entity_type` + `entity_id` 必填，挂不存在的记录 404；上传走 multipart，只收 PDF / JPG / PNG / WebP，单文件 10MB，扩展名必须与嗅探结果一致；内容存库，与元数据分表 |
| GET / DELETE | `/api/v1/attachments/:id/download`、`/api/v1/attachments/:id` | `Attachment.Download` / `.Delete` | 下载直接吐字节流（`Content-Disposition` 带 `filename*`），删除是软删，内容保留以便追溯 |

`/customers`、`/suppliers` 与 `/forwarders` 是 `/partners` 的角色切片，不是三套实现：同一个
`PartnerHandler`，路由层把默认角色传进去。新增字段只改 `PartnerHandler` 一处。

切片的行为约定：列表强制按角色过滤、创建时补 role、`Get/Update/Delete` 对
角色外的 id 返回 404、并通过这些角色入口编辑时**忽略** role 字段（不让客户页面
有机会抹掉供应商身份）。改这套逻辑前先看 `internal/handler/partner.go` 的注释。

列表通用查询参数：`page`（默认 1）、`page_size`（默认 20，上限 200）、`keyword`；
产品另有 `category` / `supplier_id` / `status`，往来单位另有 `role` / `entity_type` /
`country` / `status`，产品货源另有 `product_id` / `partner_id` / `status`，
报价单与销售订单另有 `partner_id` / `status`，采购订单另有 `supplier_id` / `status`，
出货单另有 `status` / `sales_order_id`（查"这张订单出了几次货"），
库存流水另有 `product_id` / `type` / `from` / `to`（发生日区间，含两端）。

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

### 一次带 Idempotency-Key 的写请求

```
POST /api/v1/sales-orders   Idempotency-Key: 6f1c…
 5 middleware.Auth         身份写入上下文（幂等键按用户隔离，必须排在它之后）
 6 middleware.Idempotency  Incr 抢执行权 → 抢到就放行，没抢到看有没有结果
 7 handler → service → repository
 8 响应写回缓存            {status, content-type, body}，保留 24 小时
```

要点：只对带该头的 **POST** 生效，不强制（客户端自己生成 UUID）；同一键第二次进来直接回放，
并带上 `Idempotent-Replay: true`；前一次还在路上时返回 409 而不是空响应；
**5xx 不缓存**——临时故障时重试应当真的重试；只缓存 JSON 小响应，附件与 PDF 不参与回放。

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
task backend:dev        # 热重载启动（air），SQLite + 内存缓存（默认），:8080
task backend:dev:pg     # PostgreSQL + Redis，:8081（需先 task infra:up）
task backend:dev:mysql  # MySQL + Redis，:8082（需先 infra:up + compose --profile mysql）
task backend:test       # go test ./... -count=1
```

命令的真实来源是 `apps/backend/Taskfile.yml`（`task -d apps/backend -- --list`）；
前置版本、`build` / `test:storage` / `tool:air` 与存储切换见 `README.md`。
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

### 配置文件

四个样例都在 `configs/`，各自的库 / 缓存 / 端口组合见 `README.md`「存储」一节。
新增配置项时三件事一起做：结构体字段、`applyDefaults`、`applyEnv`，并在 `Validate` 里给启动期报错。

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
   **挂完路由跑一次 `task backend:openapi`**：`docs/openapi.json` 是从路由表生成的，
   测试按字节比对，忘了重新生成 CI 会直接红（这是故意的——它拦的就是"加了接口没更新文档"）。

### 软删除

标记是整数：未删除 `0`，删除 Unix 毫秒（`model.DeletedAt`，`gorm.io/plugin/soft_delete`）。
为什么不沿用 `gorm.DeletedAt` 的 NULL 语义、以及它跟唯一索引的关系，见
`docs/architecture.md` 第 7 节；这里只写怎么用。

| 动作 | 写法 | 说明 |
|---|---|---|
| 删除 | `repo.Delete(ctx, id)` | gorm 写 `deleted_at = 毫秒`，不是物理删 |
| 查询 | 不用写条件 | gorm 自动补 `deleted_at = 0` |
| 手写 SQL | `deleted_at = 0` | **不能写 `IS NULL`**：列是整数，NULL 判断会静默查不到任何行 |
| 要看已删的 | `db.Unscoped()` | 迁移读旧表、审计、恢复时用 |
| 恢复 | `Unscoped().Model(&x{}).Where("id = ?", id).Update("deleted_at", 0)` | 同编码已有未删记录时撞唯一索引（409），这是设计 |
| 物理删除 | `Unscoped().Delete(&x{}, id)` | 只在确实没有追溯价值时用 |

**新增带唯一键的模型**时，除了嵌 `Base`，还要在自己的结构体里再声明一次 `DeletedAt`：

```go
DeletedAt DeletedAt `gorm:"softDelete:milli;not null;default:0;uniqueIndex:uni_xxx_code,priority:2" json:"-"`
Code      string    `gorm:"size:32;not null;uniqueIndex:uni_xxx_code,priority:1" json:"code"`
```

外层遮蔽 `Base` 那份，gorm 只认外层，库里仍是一列。漏了这行，唯一约束就退回单列，
删掉的记录会永久占着那个编码——而界面上已经看不到它，等于编码凭空消失。

`ON CONFLICT` 的 `Columns` 要把 `deleted_at` 一起写上（未删时恒为 0，等价于只按业务列），
否则 PG / SQLite 会报"不匹配任何唯一约束"；`repository/stock.go` 的快照 upsert 就是例子。

### 单号

两种形态，别混：

| 形态 | 用于 | 例子 | 发号入口 |
|---|---|---|---|
| `{案号}-{类型}{NN}` | 报价单、销售订单 | `DZ2603-QT01` / `DZ2603-SO01` | `DocNoService.NextDeal` |
| `{类型}-{YYMM}-{NNN}` | 采购订单、送货单 | `PO-2610-001` / `DN-2610-001` | `DocNoService.Next` |
| `{类型}-{YYYYMM}-{NNNN}` | 其余（应收 / 应付 / 收付款 / 报关 / 库存 / 合同） | `PM-202610-0001` | `DocNoService.Next` |

后两种的差别只在 `docNoStyle`：三段式短一号（月内 999 用尽报 409，不进位）。
号段表里 `period` 一律存 **4 位年的 YYYYMM**，只有单号上才截成两位——否则 2100 年
会与 2026 年落到同一行。

送货单号同时是**套号**：商业发票与装箱单共用它，只在文件名与打印件后缀上区分
（`-DN` / `-CI` / `-PL`）；改证重开升 `invoice_revision`，不换号——报关、提单与
客户账上认的都是这个号。存量 `SH-` 已由 `migrate.RenameShipmentDocNos` 重编为
`DN-`，新旧对照留在 `sys_doc_no_rename`，客户拿旧号来问时查得到。

案号 = `{客户编码}{YY}{NN}`，标的是「一笔生意」：报价与它转出的订单共用同一个，
序号也不变（QT03 转出必得 SO03）。`Next` 会明确拒绝报价与销售订单——留两条路径，
同一张表里迟早长出两种格式。

存量报价单（T-47 之前）没有 `base_no`：转单时订单取一笔新生意，不回改报价单的
`doc_no`——那个号早就发给客户了，改号等于换了身份。

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
- **单号分三层，都走 `DocNoService`**，规则见 `docs/trade-erp-crm-blueprint.md` §五.4：
  案号 `base_no` = `{客户编码}{YY}{NN}`（`DZ2603`，**不唯一**，只覆盖报价与销售订单）；
  报价与订单的 `doc_no` = `{案号}-{类型}{NN}`（`DZ2603-SO01`）；采购与送货单 = `{类型}-{YYMM}-{NNN}`
  （`PO-2610-001` / `DN-2610-001`，按月重置、上限 999、用尽报错不扩位）；同一票的送货单 / 发票 /
  装箱单共用送货单这一个号（套号）。不要在业务代码里拼"前缀 + 时间 + 查最大号 + 1"：并发下两个请求
  会拿到同一个号，撞的是唯一索引，表现为保存失败而不是重试。号段表是 `sys_doc_seq`（`sys_` 前缀，
  不嵌 `Base`），`period` 存 `YYYYMM`、单号里只展示 `YYMM`。
  **代码现状仍是 `SO-202610-0001` / `SH-202610-0001`**（没有案号、没有套号），T-47 之前以代码为准。
- **商机阶段（`OpportunityStage`）不是单据状态**，不要复用 `DocumentStatus`：
  商机阶段可以回退（客户预算冻结时真实地退回去了），单据状态只能前进与取消。
  两者的"终态不可再改"是同一条规则，其余全不同。
- **占用数量的单据，可用量必须在服务端算**：出货分配行的上限是
  `订单行数量 − 已占用数量`，它既不是订单行上的 qty，也不能由前端提交。
  两条既有实现要遵守的细节：编辑时要排除自己那一票（`excludeShipmentID`），
  同一次请求里同一行出现两遍要累计（`used` map）——少了任何一条，
  "把 100 改成 90"或"拆成两行各一半"都能绕过校验。
- **取消的单据不许再占数量**：统计已占用数量时一律带 `status <> 已取消`，
  否则客户取消一票再补一票会永远报"超出未出数量"，而实际什么都没出。
- **库存快照只能由流水推出来**：没有任何接口能直接写 `inv_stock_snapshot`。
  写入流水与更新快照必须在同一事务里，快照的增量用 `ON CONFLICT` 原子加，
  不要"先查再 insert/update"（两个人同时入库会撞唯一索引或互相吃掉增量）。
  口径对不上时只做一件事：用 `Stock.Recalc` 按流水重算，并把重算前的差异报出来。
- **流水的方向单独存**：`StockMovement.Qty` 永远为正，出入看 `Direction`（±1）。
  允许存负数会让"两条 +5"和"一条 −10"同样合法，对账时分不清是录错了还是故意的。
- **出入库单确认才产生流水**：草稿改多少次都不动库存。确认要在一个事务里做完
  「改状态 + 回写账面数 + 插流水 + 更新快照」，中间任何一步单独提交都会留下
  "单据已确认但库存没动"的状态，而它只能靠人工对账发现。
- **已确认的库存单据不撤销**：不可改、不可删、不可取消，要纠正就做反向单。
  取消一张已确认的入库单会让流水凭空消失，而"消失"这件事在账上没有任何痕迹。
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
| 并发重复提交返回 500 | 前置的 Exists 校验挡不住并发，第二个请求撞上唯一索引，而驱动给的原始错误没被翻译 | `database.Open` 已开 `TranslateError`，并在 Create/Update 回调里统一翻成 40901；不要在调用点比错误字符串，三个库的文本各不相同 |
| 登出后旧令牌还能用 | 绕过了会话校验 | `AuthService.Verify` 里那次 `sessions.Get` 不能删 |
| 重启后登录态全没了 | memory 缓存没落盘 | `cache.memory.persist: true` 且 `Close()` 被调用（`main` 里靠 defer 保证） |
| 生产误用了内存缓存 | `cache.driver` 配错 | 生产 `fallback_to_memory: false`，连不上 Redis 直接启动失败 |
| 第 3 节的自检命令计数不为 0 | 新代码引入了反向依赖 | 按分层把逻辑挪回对应层 |
| 改了结构体字段名之后那一列数据全变空 | 没写 `gorm:"column:..."`，gorm 按新字段名建了新列，旧数据留在旧列里 | 列名与字段名不一致时必须显式写 `column:`，例：`model.Partner.ContactName` 绑的是 `contact` 列 |
| 筛选查不到记录，但库里明明有 | 把 `DocumentStatus` 的值传给了 `EnableStatus` 字段（或反过来） | 两者都是小整数，编译器不拦；先按 `docs/glossary.md`「状态」一节确认这个字段属于哪套 |
| 明明同时是客户和供应商，却在供应商列表里找不到 | 用 `role = ?` 判角色 | 角色是位掩码，必须按位与（`role.Has(...)`）；列表筛选走 `RoleValuesContaining` 展开成 IN，既覆盖 `role=3` 又能命中索引 |
| 某主体在两个列表里都显示成"未知角色" | 前端写了 `role === 1` / `role === 2` | 同样按位与：`(role & PARTNER_ROLE.supplier) !== 0` |
| 合并主体之后，某张单据的客户变成空白 | 新加的外键没进 `repository/partnerRefTables` | 任何指向 `md_partner` 的外键都要在 `partnerRefTables` 与 `service/partner_merge_test.go` 的 `partnerRefTablesForTest` 各补一行；后者故意不引用前者，就是为了独立兜底 |
| 盘点单的差异跟心里算的不一样 | 账面数取的是**建单时**的库存，而不是确认时的 | `resolvePosting` 里现查 `CurrentQty`；盘点单搁了半天，中途别的单据动过货 |
| 确认了入库单，库存没变 | 单据与流水没在同一个事务里，或确认时漏了 `addToSnapshot` | 走 `StockDocumentRepository.Post`，它是「状态 + 账面 + 流水 + 快照」的唯一入口 |
| 入库之后库存列表里看不到那个产品 | 快照行被软删了，upsert 撞唯一索引更新到那条已删行上 | 快照永不软删：重算时写 `qty=0` 保留行（`ReplaceSnapshot` 的注释说了原因） |
| 库存数与流水算出来的对不上 | 有人直接改了 `inv_stock_snapshot`，或某次写入没走同一事务 | 跑 `POST /stock-snapshots/recalc`，它会把差异报出来并按流水修正 |
| 订单明明没出完，出货却总报"超出未出数量" | 统计已出数量时把已取消的出货也算进去了（或编辑时没排除本票） | 统计一律带 `biz_shipment.status <> 90`，编辑传 `excludeShipmentID` |
| 出货单里的"来源订单"列全是空 | 只 Preload 了 `Allocations.SalesOrderItem`，没再 `Preload("Allocations.SalesOrderItem.SalesOrder")`，或没走 `annotateDocNo` 摊平 | 仓储三处 Preload 要齐；返回前调 `annotateDocNo`（`sales_order_doc_no` 是 `gorm:"-"` 投影） |
| 新记录提交"停用"，存进去却是"启用" | 字段带 `gorm:"default:1"` 且值恰为零值，gorm 会省略该列让数据库填默认值 | 新表不要再给 `EnableStatus` 加 `default` tag；老表改状态走 `UpdateFields(map)`（map 显式指定列，不受影响） |
| 迁移后少了若干条旧数据 | 读旧表时漏了 `Unscoped`，软删除条件由目标 struct 的 schema 决定，`Table()` 指定表名也绕不过去 | `tx.Table("md_xxx").Unscoped().Find(&rows)`，`internal/migrate/legacy_merge_test.go` 有对应用例 |
| 接口整批 500，`/auth/login` 却正常 | 跑着的是改表名前编译的旧二进制，仍按 `md_party` 查询已被改名的 `md_partner`；登录只碰 `sys_user` 所以不受影响 | 重启 dev 即可，不是代码缺陷。判断版本：`/partners` 404 + `/parties` 401 就是旧二进制。`response.Fail` 已把真实原因写进日志，先看日志再猜 |
