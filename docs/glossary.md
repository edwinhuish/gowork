# 术语表

**本文件是项目术语的唯一事实来源。** 其他文档提到业务对象时只引用这里的名字，
不重复定义——同一概念写两遍必然漂移。

---

## 一、定位

GoWork 是外贸采购代理的 **ERP 与 CRM 集成系统**：一套主数据，两侧业务。
采购代理的日常是"替客户买、替客户发、盯住客户关系"，这三件事分别落在
ERP 的采购/出货链路、CRM 的客户关系链路上，共用同一份产品与往来单位数据。

**不是"两个系统对接"**——没有同步任务、没有第二份客户表。集成发生在数据层：
CRM 记的跟进对象与 ERP 开发票的对象是同一行 `md_partner` 记录。

而且**客户与供应商本身也是同一张表**。它们是同一个"往来单位"的两种角色，
用 role 位掩码区分，因此"既是客户又是供应商"的主体只有一条记录、一套联系方式。

## 二、三层域模型

```
        ┌──────────────── 共享主数据 md_ ─────────────────────┐
        │ 产品 Product · 往来单位 Partner（客户/供应商/两者） │
        └───────┬─────────────────────────────┬───────────────┘
                │                             │
    ┌───────────▼───────────┐     ┌───────────▼───────────┐
    │  ERP 域 biz_ / inv_   │     │  CRM 域 crm_          │
    │  采购 · 库存 · 出货    │     │  联系人 · 跟进 · 商机  │
    │  订单 · 对账           │     │                       │
    └───────────────────────┘     └───────────────────────┘
```

| 域 | 管什么 | 表前缀 | 引用的主数据 | 当前状态 |
|---|---|---|---|---|
| 共享主数据 | 被两侧反复引用的实体 | `md_` | — | 已实现 |
| ERP | 交易与供应链：采购、库存、出货、订单 | `biz_`（单据）`inv_`（库存） | 产品、往来单位（供应商角色） | 未实现 |
| CRM | 客户关系：联系人、跟进、商机 | `crm_` | 往来单位（客户角色）、产品 | 未实现 |
| 系统 | 账号、角色、单号号段 | `sys_` | — | 已实现 |

**为什么往来单位不放进 `crm_`**：客户是 ERP 开发票、CRM 做跟进的同一个主体。
把它归给任何一侧，另一侧就得跨域引用，域边界立刻失效。判断标准是
"这张表会不会被两个域同时写"——会，就留在 `md_`。

**为什么客户与供应商不拆成两张表**：拆开的代价在"同一主体既是客户又是供应商"时
立刻显现——两行数据、两套联系方式、两个编码，对账时还要人工确认它们是同一家公司。
合并后一笔业务引用同一个 id，往来余额可以直接按主体归集。

## 三、术语表

"主要使用方"表示哪一侧以它为主；标记两侧的即为共享数据。

### 主数据（已实现）

| 中文 | 英文标识 | 定义 | 主要使用方 |
|---|---|---|---|
| 产品 | `Product` | 可售卖的最小实体，以 SKU 编码唯一标识。价格、规格、包装是它的属性，不是它的身份 | ERP + CRM |
| SKU 编码 | `SKUCode` | 产品唯一编码，格式 `类别(2) + 子类(2) - 流水(4)`，如 `BH01-0023`。全库唯一、永不复用 | ERP + CRM |
| 产品类别 | `Category` | 两字母类别码（`HT` / `PT` / `BH` …），与 SKU 编码前两位一致 | ERP + CRM |
| 往来单位 | `Partner` | 客户与供应商的统一主体。一个主体可同时承担两种角色，此时只有一条记录、一套联系方式 | ERP + CRM |
| 主体角色 | `PartnerRole` | 位掩码：`1` 客户 / `2` 供应商 / `3` 两者 | ERP + CRM |
| 主体类型 | `PartnerEntityType` | `0` 公司 / `1` 个人。个人主体的报关资料与发票抬头和公司不同 | ERP + CRM |
| 供应商角色 | `PartnerRoleSupplier` | 角色位 `2`。代码里说"供应商"指的是**这个角色**，不是一张表 | ERP |
| 客户角色 | `PartnerRoleCustomer` | 角色位 `1`。同上 | ERP + CRM |
| 联系人姓名 | `Partner.ContactName` | 主体上的**单个**主联系人姓名。完整联系人（多个、各有电话邮箱）是 CRM 的 `Contact` 实体，尚未实现 | ERP + CRM |
| 系统账号 | `User` | 登录系统的操作者，与往来单位的联系人无关 | 系统 |

### 底座（未实现，命名先定下来）

这几张表本身不是业务单据，但被后面每张单据引用，所以表名要先定死。

| 中文 | 英文标识 | 表名 | 定义 | 主要使用方 |
|---|---|---|---|---|
| 币种 | `Currency` | `md_currency` | ISO 4217 三位代码（`USD` / `EUR` / `CNY`）+ 小数位。全库有且只有一个本币 | ERP + CRM |
| 本币 | `BaseCurrency` | — | 记账本位币，`Currency` 里 `is_base` 的那一条。不是配置项：它出现在每张单据的本币金额里，必须可查 | ERP |
| 汇率 | `ExchangeRate` | `md_exchange_rate` | 某币种在某生效日相对本币的折算率，`1 外币 = rate 本币`。按日追加历史，不覆盖 | ERP + CRM |
| 单据编号 | `DocNo` | — | 单据的业务编号，格式 `类型(2)-年月(6)-流水(4)`，如 `SO-202610-0001`。与主键 `id` 是两回事 | ERP |
| 单号号段 | `DocSeq` | `sys_doc_seq` | 每个「单据类型 + 年月」一条，记录该号段已发出的最大序号。不嵌 `Base`：号段若可软删除，号码就能重发 | ERP |

### 业务事务 · CRM（未实现，命名先定下来）

| 中文 | 英文标识 | 表名 | 定义 |
|---|---|---|---|
| 联系人 | `Contact` | `crm_partner_contact` | 往来单位下的具体自然人，一个主体可有多个；各有电话、邮箱、职位、语言。`md_partner.contact` 只是主联系人姓名的冗余 |
| 跟进记录 | `Activity` | `crm_activity` | 一次交互（电话/邮件/拜访/报价），挂在往来单位**或**商机上 |
| 商机 | `Opportunity` | `crm_opportunity` | 有成交可能的销售机会，有阶段、金额、币种、赢单概率 |
| 线索 | `Lead` | `crm_lead` | 尚未建档的询盘主体，有来源渠道与国别；转化后指向新建的往来单位 |

### 业务事务 · ERP 交易（`ProductSource` / `Quotation` / `SalesOrder` / `PurchaseOrder` 已实现，其余命名先定下来）

| 中文 | 英文标识 | 表名 | 定义 |
|---|---|---|---|
| 产品货源 | `ProductSource` | `md_product_source` | 某产品在某供应商处的采购来源与货号、价格、交期。一个产品可有多个货源 |
| 报价单 | `Quotation` | `biz_quotation`（明细 `biz_quotation_item`） | 向客户发出的价格方案，含贸易术语、币种、有效期与多轮版本 |
| 销售订单 | `SalesOrder` | `biz_sales_order`（明细 `biz_sales_order_item`） | 客户下达的订单，含明细行；金额由明细汇总 |
| 采购订单 | `PurchaseOrder` | `biz_purchase_order`（明细 `biz_purchase_order_item`） | 向供应商下达的订单，可挂到销售订单行上 |
| 出货批次 | `Shipment` | `biz_shipment`（分配 `biz_shipment_allocation`） | 一次实际发运。与订单是多对多（分批出货、合并出货） |
| 库存流水 | `StockMovement` | `inv_stock_movement` | 每一次出入库记录，库存的权威来源 |
| 库存快照 | `StockSnapshot` | `inv_stock_snapshot` | 当前库存，只能由流水重算，用于对账与性能 |
| 应收单 | `Receivable` | `biz_receivable` | 应向客户收取的一笔款项，由订单或出货生成 |
| 应付单 | `Payable` | `biz_payable` | 应向供应商支付的一笔款项 |
| 收付款单 | `Payment` | `biz_payment`（核销 `biz_payment_allocation`） | 实际发生的一笔收款或付款，可核销多张应收/应付 |
| 运费杂费 | `ShipmentCharge` | `biz_shipment_charge` | 归集到出货批次的海运费、报关费、拖车费等 |
| 报关单 | `CustomsDeclaration` | `biz_customs_declaration` | 一票货物的报关记录，含报关单号与监管方式 |

### 单据与协作（未实现，命名先定下来）

| 中文 | 英文标识 | 表名 | 定义 |
|---|---|---|---|
| 形式发票 / 销售合同 | `Contract` | `biz_contract` | PI 与销售合同同一张表，用 `type` 区分。**先于订单成立**：客户凭它付定金或开信用证 |
| 合同付款条款 | `ContractTerm` | `biz_contract_term` | 合同的付款分期（30% 订金 / 70% 见提单副本），合计须等于合同总额 |
| 附件 | `Attachment` | `sys_attachment` | 挂在主体或单据上的文件：提单、报关单、验货报告。与业务无关，故用 `sys_` |
| 审计日志 | `AuditLog` | `sys_audit_log` | 谁在什么时候把哪张单改成了什么。只追加，不提供修改与删除接口 |

「PI」是 Proforma Invoice 的日常叫法，文档与代码里统一写 **形式发票 `Contract(type=PI)`**，
不要另起 `ProformaInvoice` 这个类型——同一份东西两个名字，最后会有两个接口。

### 交易条件（不是实体，是单据上的字段）

| 中文 | 英文标识 | 取值 | 说明 |
|---|---|---|---|
| 贸易术语 | `TradeTerm` | `EXW` / `FOB` / `CIF` / `CFR` / `DDP` | 决定运费与保险由谁承担，直接影响报价的构成 |
| 结算方式 | `SettlementMethod` | `TT` 电汇 / `LC` 信用证 / `DP` 付款交单 / `DA` 承兑交单 / `OA` 赊销 | 决定账期与风险，报价时的价格基础不同 |

### 状态（最容易混的一对）

| 中文 | 英文标识 | 取值 | 含义 |
|---|---|---|---|
| 启用状态 | `EnableStatus` | `1` 启用 / `0` 停用 | 这条**主数据或账号**还能不能被业务引用。停用不删除，历史单据仍指得到 |
| 单据状态 | `DocumentStatus` | `10` 草稿 / `20` 已确认 / `30` 备货中 / `40` 已出货 / `50` 已完成 / `90` 已取消 | **业务单据**的流转阶段 |

两者是不同类型，不要共用字段、不要共用一个 `status` 列。

`DocumentStatus` 的取值**故意留空档**（10 / 20 / 30 …）：将来要在"已确认"和"备货中"之间
插一个"已收订金"，只需给一个新值，不用把已有记录的 30 改成 40。连续编号看着整齐，
但它的整齐是拿历史数据换的。


### 角色的位掩码（合并客户与供应商的产物）

角色用位掩码而不是 `1=客户 / 2=供应商 / 3=两者` 的连续枚举。理由是角色天然是集合：
加入第三种角色（货代、报关行）时，连续枚举要补到 7 个值，而且每加一种都得回头
改所有判断分支；位掩码只加一个常量。

代价是**判断必须按位**：

```go
role.Has(model.PartnerRoleSupplier)  // 对
role == model.PartnerRoleSupplier    // 错：值为 3 的主体被判成"不是供应商"
role&model.PartnerRoleSupplier != 0  // 对，Has 内部就是这么写的
```

```ts
(role & PARTNER_ROLE.supplier) !== 0  // 对
role === 1                            // 错，理由同上
```

还有一处只有位掩码才会遇到的细节：按角色**筛选列表**时不能用
`WHERE role = ?`，那样会把"客供两用"的主体漏掉。查询要展开成
`role IN (...)`（Go 侧是 `model.RoleValuesContaining`），既覆盖全部组合，
又能命中 role 上的普通索引——直接写位运算条件在 PostgreSQL 和 MySQL 上都走不了 btree。

## 四、易混词对照

左列是**本项目不允许出现**的说法，右列才是对的。

| 不要用 | 用 | 原因 |
|---|---|---|
| Active / 在售 / 有效 | Enabled / 启用 | 同一个 `status=1` 曾有三种叫法，代码、文档、界面各说各的 |
| Contact（指人名时） | `ContactName` | `Contact` 是 CRM 实体，不是字段 |
| `code`（指 SKU 时） | `sku_code` / `SKUCode` | 往来单位也有 `code`，语义不同 |
| Status（脱离上下文的裸写法） | `EnableStatus` / `DocumentStatus` | 不带限定词的 status 无法判断是哪套状态 |
| vendor | Supplier（角色名） | 角色仍叫 supplier，承载它的实体叫 partner |
| client | Customer（角色名） | 角色仍叫 customer，承载它的实体叫 partner |
| 客户表 / 供应商表 | `md_partner` + 角色过滤 | 合并后只有一张表；`/customers`、`/suppliers` 是同一资源按角色切的入口 |
| 客供两用 / 双向往来（作为角色取值） | `role = 3`（客户\|供应商） | 它是两个角色位的组合，不是第三个角色 |
| 个人客户 / 公司客户（作为两个类型） | `PartnerEntityType` | 主体类型与角色是两个正交维度，别混成一个字段 |
| 商品 / 货品 | 产品 | `Product` |
| 货号 / 型号 | SKU 编码 | 货号是供应商侧的叫法，属于 `ProductSource` |
| 供应商货号（简写「货号」） | `supplier_sku`（`ProductSource.SupplierSKU`） | 与 SKU 编码不是一回事：前者是对方目录里的叫法，后者是我方的身份 |
| `lead_time` / 货期（作为字段名） | `delivery_days`（交期天数） | 本库里 `Lead` 已经是 CRM 的线索，`lead_time` 在搜索与讨论里会被读成「线索时间」 |
| 单据（泛指任何记录） | 明确说是主数据还是单据 | 单据特指有状态流转的业务凭证 |
| 订单（裸写） | `SalesOrder` / `PurchaseOrder` | 两个方向都有订单，裸写「订单」时无法判断谁向谁下 |
| 发货 / 出运 / 发货单 | 出货批次 `Shipment` | 三个叫法指同一件事；「出运」只在报关语境里用 |
| 已出数量（不说明含不含已取消的票） | 有效出货已占数量（`Shipment.Status <> 已取消`） | 把取消的票也算进去，可用量会凭空少一截，而货一件都没出 |
| 剩余数量 / 还有多少（指出货口径时） | 未出数量 = 订单行数量 − 已占数量 | 它和库存不是一回事：这张订单还没出的量，可能连货都还没采购 |
| 报价 / 价格表 / 报价方案 | 报价单 `Quotation` | 价格表是产品属性，报价单是有有效期与版本的凭证 |
| 改单 / 修正（对已确认的报价） | 另存新版本（`version` 递增） | 已确认的报价单是发给客户的承诺，改它等于改历史；同一份报价共用 `DocNo` |
| 成本价（不说明口径） | 货源本币价 / 报价单明细上的成本快照 | 前者是会变的主数据，后者是写下单据那一刻的锁定值 |
| 汇率 / 汇价 / 兑换率 | `ExchangeRate` | 三处各写一个名字，最后会有人以为它们是三个概念 |
| 币种 / 货币 / 结算币 | `Currency` | 用「币种」；「结算币」是单据上的字段语义，不是实体 |
| 金额（不说明计价口径） | 原币金额 / 本币金额 | 跨币种单据上两者相差一个汇率，裸写「金额」必然被误读 |
| 客户（指线索时） | `Lead` | 线索还没建档，没有 `md_partner` 记录，不算客户 |
| 联系人（指主体上的主联系人） | `ContactName` | 同第 122 行的规则，只是这次要区分「多人中的主联系人」 |
| 库存（指快照） | `StockSnapshot` | 「库存」既可指流水也可能指快照，查询与修正走的是两套规则 |
| 核销 / 冲销 | 核销 `PaymentAllocation` | 「冲销」是财务的红字冲回，与把收款对应到应收不是一回事 |
| Party / party | `Partner` / `partner` | Party 在合同语境里指「当事方」，日常又容易被读成「聚会」，做主数据实体名两头都不贴切；Partner 才是「业务伙伴」的行业说法。表名同理：`md_party` → `md_partner` |

## 五、命名规则

### Go

| 规则 | 例子 |
|---|---|
| 缩写词全大写（Go 官方 Code Review Comments） | `SKUCode`、`UserID`、`TokenID`、`HTTP`、`DSN`、`CORSOrigins`、`JWT`。不写 `SkuCode`、`UserId` |
| 导出标识符 PascalCase，包内 camelCase | `ProductService`、`normEnableStatus`、`enableStatusPtr` |
| 接口以职责命名，不加 `I` 前缀 | `ProductRepository`，不是 `IProductRepository` |
| 构造函数 `New + 类型名`，可选依赖走 Option | `NewProductService(repo, WithProductLogger(log))` |
| 仓储查重方法带列语义 | `ExistsBySKUCode` / `ExistsByCode`，不写含糊的 `Exists` |
| 布尔方法用 `Is` / `Has` / `Can` 开头 | `IsEnabled`、`IsAdmin` |

### TypeScript

| 规则 | 例子 |
|---|---|
| 组件 PascalCase，文件名与导出一致 | `ProductList`、`form-fields.tsx` 导出多个字段组件 |
| hook 以 `use` 开头 | `useTable`（refine 提供）、`useForm` |
| 常量 UPPER_SNAKE_CASE | `ENABLE_STATUS_OPTIONS`、`PAGE_SIZE` |
| 类型/接口 PascalCase，不加 `I` 前缀 | `Partner`、`PageResult<T>` |
| 字段名与后端 JSON 完全一致（snake_case） | `sku_code`、`created_at`，不要在这里转 camelCase |

### 数据库与 API

| 对象 | 规则 | 例子 |
|---|---|---|
| 表名 | `前缀_单数_snake` | `md_partner`、`sys_user`、`biz_sales_order` |
| 列名 | snake_case，与 JSON tag 对齐 | `sku_code`、`supplier_id`、`entity_type` |
| 主键 | 一律 `id`，自增 `uint` | 见 `docs/architecture.md` 第五节 |
| API 路径 | 复数、kebab-case | `/api/v1/products`、`/api/v1/partners`、`/api/v1/sales-orders` |
| JSON 字段 | snake_case | `product_total`、`enabled_products` |
| 单据编号列 | 一律 `doc_no`，加 `uniqueIndex` | `biz_sales_order.doc_no` = `SO-202610-0001` |
| 币种列 | `currency_code`，存 ISO 4217 三位代码 | 不要存币种 `id`：单据要能脱离主数据读懂 |
| 金额列 | 跨币种单据必须成对出现 | `amount`（原币）+ `amount_base`（本币），另存 `exchange_rate` |
| 单据状态列 | 一律叫 `status`，类型 `model.DocumentStatus` | 与主数据的 `status`（`EnableStatus`）同名不同型，靠类型而非列名区分 |

**列名与 Go 字段可以不同，但必须有理由。** 例：`Partner.ContactName` 的列名是
`contact`，因为改名会让 AutoMigrate 新增列、把既有数据留在旧列里。
这类偏离要在结构体上写明原因，不能只靠 commit message。
