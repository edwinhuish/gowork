# 术语表

**本文件是项目术语的唯一事实来源。** 其他文档提到业务对象时只引用这里的名字，
不重复定义——同一概念写两遍必然漂移。

---

## 一、定位

GoWork 是外贸采购代理的 **ERP 与 CRM 集成系统**：一套主数据，两侧业务。
采购代理的日常是"替客户买、替客户发、盯住客户关系"，这三件事分别落在
ERP 的采购/出货链路、CRM 的客户关系链路上，共用同一份产品、供应商、客户数据。

**不是"两个系统对接"**——没有同步任务、没有第二份客户表。集成发生在数据层：
CRM 记的跟进对象与 ERP 开发票的对象是同一行 `md_customer` 记录。

## 二、三层域模型

```
        ┌──────────────── 共享主数据 md_ ────────────────┐
        │  产品 Product · 供应商 Supplier · 客户 Customer  │
        └───────┬─────────────────────────────┬──────────┘
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
| ERP | 交易与供应链：采购、库存、出货、订单 | `biz_`（单据）`inv_`（库存） | 产品、供应商、客户 | 未实现 |
| CRM | 客户关系：联系人、跟进、商机 | `crm_` | 客户（主）、产品 | 未实现 |
| 系统 | 账号、角色 | `sys_` | — | 已实现 |

**为什么 `md_customer` 不放进 `crm_`**：客户是 ERP 开发票、CRM 做跟进的同一个主体。
把它归给任何一侧，另一侧就得跨域引用，域边界立刻失效。判断标准是
"这张表会不会被两个域同时写"——会，就留在 `md_`。

## 三、术语表

"主要使用方"表示哪一侧以它为主；标记两侧的即为共享数据。

### 主数据（已实现）

| 中文 | 英文标识 | 定义 | 主要使用方 |
|---|---|---|---|
| 产品 | `Product` | 可售卖的最小实体，以 SKU 编码唯一标识。价格、规格、包装是它的属性，不是它的身份 | ERP + CRM |
| SKU 编码 | `SKUCode` | 产品唯一编码，格式 `类别(2) + 子类(2) - 流水(4)`，如 `BH01-0023`。全库唯一、永不复用 | ERP + CRM |
| 产品类别 | `Category` | 两字母类别码（`HT` / `PT` / `BH` …），与 SKU 编码前两位一致 | ERP + CRM |
| 供应商 | `Supplier` | 提供货物的工厂或 1688 店铺 | ERP |
| 客户 | `Customer` | 采购方主体：外贸链条里的进口商或终端门店 | ERP + CRM |
| 联系人姓名 | `ContactName` | 客户/供应商上的**单个**主联系人姓名。完整联系人（多个、各有电话邮箱）是 CRM 的 `Contact` 实体，尚未实现 | ERP + CRM |
| 系统账号 | `User` | 登录系统的操作者，与客户/供应商的联系人无关 | 系统 |

### 业务事务（未实现，命名先定下来）

| 中文 | 英文标识 | 定义 | 域 |
|---|---|---|---|
| 联系人 | `Contact` | 客户下的具体自然人，一个客户可有多个；各自有电话、邮箱、职位 | CRM |
| 跟进记录 | `Activity` | 一次交互（电话/邮件/拜访/报价），挂在客户或商机上 | CRM |
| 商机 | `Opportunity` | 有成交可能的销售机会，有阶段与预计金额 | CRM |
| 销售订单 | `SalesOrder` | 客户下达的订单，含明细行 | ERP |
| 采购订单 | `PurchaseOrder` | 向供应商下达的订单，含明细行 | ERP |
| 出货批次 | `Shipment` | 一次实际发运。与订单是多对多（分批出货、合并出货） | ERP |
| 产品货源 | `ProductSource` | 某产品在某供应商处的采购来源与货号。一个产品可有多个货源 | ERP |
| 库存流水 | `StockMovement` | 每一次出入库记录，库存的权威来源 | ERP |
| 库存快照 | `StockSnapshot` | 当前库存，可由流水重算，用于对账 | ERP |

### 状态（最容易混的一对）

| 中文 | 英文标识 | 取值 | 含义 |
|---|---|---|---|
| 启用状态 | `EnableStatus` | `1` 启用 / `0` 停用 | 这条**主数据或账号**还能不能被业务引用。停用不删除，历史单据仍指得到 |
| 单据状态 | `DocumentStatus` | 待实现 | **业务单据**的流转阶段（待确认/已确认/已发货/已取消） |

两者是不同类型，不要共用字段、不要共用一个 `status` 列。

## 四、易混词对照

左列是**本项目不允许出现**的说法，右列才是对的。

| 不要用 | 用 | 原因 |
|---|---|---|
| Active / 在售 / 有效 | Enabled / 启用 | 同一个 `status=1` 曾有三种叫法，代码、文档、界面各说各的 |
| Contact（指人名时） | `ContactName` | `Contact` 是 CRM 实体，不是字段 |
| `code`（指 SKU 时） | `sku_code` / `SKUCode` | 客户和供应商也有 `code`，三者语义不同 |
| Status（脱离上下文的裸写法） | `EnableStatus` / `DocumentStatus` | 不带限定词的 status 无法判断是哪套状态 |
| vendor | Supplier | 项目统一用 supplier |
| client | Customer | 项目统一用 customer |
| 商品 / 货品 | 产品 | `Product` |
| 货号 / 型号 | SKU 编码 | 货号是供应商侧的叫法，属于 `ProductSource` |
| 单据（泛指任何记录） | 明确说是主数据还是单据 | 单据特指有状态流转的业务凭证 |

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
| 类型/接口 PascalCase，不加 `I` 前缀 | `Customer`、`PageResult<T>` |
| 字段名与后端 JSON 完全一致（snake_case） | `sku_code`、`created_at`，不要在这里转 camelCase |

### 数据库与 API

| 对象 | 规则 | 例子 |
|---|---|---|
| 表名 | `前缀_单数_snake` | `md_customer`、`sys_user`、`biz_sales_order` |
| 列名 | snake_case，与 JSON tag 对齐 | `sku_code`、`supplier_id` |
| 主键 | 一律 `id`，自增 `uint` | 见 `docs/architecture.md` 第五节 |
| API 路径 | 复数、kebab-case | `/api/v1/products`、`/api/v1/sales-orders` |
| JSON 字段 | snake_case | `product_total`、`enabled_products` |

**列名与 Go 字段可以不同，但必须有理由。** 例：`Customer.ContactName` 的列名是
`contact`，因为改名会让 AutoMigrate 新增列、把既有数据留在旧列里。
这类偏离要在结构体上写明原因，不能只靠 commit message。
