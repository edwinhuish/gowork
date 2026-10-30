# apps/frontend —— React 管理端（面向 AI 协作者）

**职责**：前端手册——页面怎么组织、数据怎么流动、加模块照什么套路。
**不管**：业务规则与接口契约（`apps/backend/AGENTS.md`）、项目整体目标（根 `AGENTS.md`）、
取舍理由（`docs/architecture.md`）、业务对象命名（`docs/glossary.md`）。
**相关**：`README.md`（命令与 pnpm 约定）、`docs/roadmap.md`（已实现哪些模块）。

---

## 1. 职责边界

**负责**：登录、概览，以及全部业务模块的界面——共享主数据（产品 / 货源 / 往来单位 /
币种汇率）、CRM（联系人 / 跟进 / 线索 / 商机）、ERP 交易（报价 / 合同 / 销售订单 / 采购订单）、
供应链（出货 / 库存 / 出入库单）。列表分页与筛选、登录态维持、主题与布局也在这里。

**界面跟着后端模块走，不自己造概念。** 加一个模块不需要新建导航表——
在 `resources` 里加一条就出现在侧栏（见第 3 节）。业务对象叫什么名、
字段中文怎么写，先查 `docs/glossary.md`；哪些模块已实现，见 `docs/roadmap.md` 的进度表。

**不负责**：

| 不属于这里 | 归哪 |
|---|---|
| 业务规则（唯一性、状态流转、金额计算） | 后端 service 层；前端只做必填与格式校验 |
| 权限判断 | 后端；前端最多按 `user.role` 隐藏入口 |
| 服务端会话 | 后端；前端只持有令牌 |
| 业务术语的定义 | `docs/glossary.md`；前端只引用，不改写 |

技术栈：React 19 + TypeScript 5.9（`strict`）+ Vite 6 + **refine 5**（`@refinedev/core` / `@refinedev/antd` / `@refinedev/react-router`）+ **Ant Design 5** + TanStack Query 5，包管理 pnpm 12。

**没有** ESLint / Prettier / Tailwind：类型检查靠 `tsc --noEmit`，结构性样式在 `src/index.css`，其余全部由 antd + 设计 token 提供。

> 历史说明：本项目早期刻意不引入 UI 组件库，手写了 525 行 CSS 与一套分页/弹窗组件。
> 2026-10 按需求整体重做为 refine + antd，原因是手写件在表单校验、表格交互、响应式上
> 都要自己造轮子，且多模块之间容易走形。结论：**组件库已引入，不要再手写基础控件**。

## 2. 目录结构

```
apps/frontend/
├── index.html
├── vite.config.ts          # 端口 5173；/api 代理到 :8080；@ 别名；手动分包
├── tsconfig.json           # strict + noUnusedLocals/Parameters；paths 的 @/* -> src/*
├── package.json            # packageManager: pnpm@12.10.1
├── pnpm-workspace.yaml     # pnpm 设置（allowBuilds 放行 esbuild），不是工作区划分
└── src/
    ├── main.tsx            挂载；先引 antd reset.css 再引 index.css
    ├── App.tsx             Refine 装配 + 三段式路由 + resources 声明
    ├── theme.ts            设计 token（颜色/圆角/组件级配置），全站唯一色彩来源
    ├── index.css           只放 antd 覆盖不到的结构样式
    ├── types/index.ts      与后端 dto 对齐的类型（snake_case）
    ├── providers/
    │   ├── dataProvider.ts REST 适配：{code,message,data} 拆包、分页与筛选翻译
    │   └── authProvider.ts 登录/登出/身份缓存/401 处理
    ├── api/
    │   ├── client.ts       统一 fetch：令牌注入、拆包、ApiError、续期令牌替换
    │   └── auth / dashboard / leads / opportunities / product-sources /
    │       purchase-orders / quotations / sales-orders / shipments /
    │       stock / stock-documents   单个资源的非 CRUD 动作（转单、确认、重算…）
    ├── components/
    │   ├── layout/AppLayout.tsx   ThemedLayout + 品牌块（GoWork / ERP · CRM 两行）+ 顶栏
    │   ├── fields.tsx       StatusTag / RoleTag / EntityTypeTag / AccentTag / HeatTag /
    │   │                    Money / CategoryTag / Mono / Empty（标签的三档用法见第 6 节）
    │   ├── partner-avatar.tsx     主体头像 / Logo：真图 → 名称占位标识，列表与卡片共用
    │   ├── partner-cards.tsx      往来单位卡片视图 + 卡片行操作 + 分页转换（三张角色表共用）
    │   ├── line-items-editor.tsx  单据明细行（数量 × 单价），create / edit 共用
    │   ├── product-select.tsx     选产品的下拉：按关键词走后端搜，客户报的编号也能选到
    │   ├── product-source-select.tsx  明细行的成本货源：按产品现查，不再全量取数
    │   └── authed-image.tsx      带令牌取图（fetch → blob → object URL），图片接口不能用 <img src>
    ├── constants/catalog.ts 产品类别字典、账号启用状态与币种归档状态、主体类型选项、角色位常量
    ├── hooks/
    │   └── use-idempotency-key.ts  写操作的幂等键：同一次提交的重试复用，成功后轮换
    ├── utils/format.ts      时间格式化
    └── pages/
        ├── login/  dashboard/
        ├── products/         list / create / edit / show / form-fields / source-panel / code-panel / image-panel
        ├── suppliers/ customers/ forwarders/   同上（同一 `Partner` 模型的三种角色视图）
        ├── contacts/ activities/ leads/ opportunities/   list / create / edit / form-fields
        ├── quotations/       同上 + show / preview（试算页）
        ├── contracts/        list / create（只能从来源单据生成）/ edit / show + terms-editor
        ├── orders/ purchases/   list / create / edit / show（+ 明细行编辑器；订单带额度提示、
        │                       admin 强制放行、已确认后的变更弹窗与变更历史）
        ├── shipments/        同上（+ 分配行编辑器、物流节点卡片与报关单 / 费用 / 毛利三个面板；
        │                      详情页可下载商业发票 / 装箱单 / 箱唛，只能改证重开、不能另开一票）
        ├── settlements/      应收应付共用的 list / form-fields / show
        ├── receivables/ payables/  四页薄封装，只传 resource 与方向
        ├── payments/         收付款 list / create / edit / show（+ 核销行编辑器，create 里自带）
        ├── stock/            只读当前库存 + 重算（没有 create / edit）
        ├── stock-movements/  流水列表 + 手工录入（没有 edit）
        ├── stock-documents/  出入库单 + items-editor + 确认
        ├── aging/            账龄报表（只读聚合，没有 create / edit）
        ├── exchange-gains/   汇兑损益报表（同上，逐笔明细 + 币种小计）
        ├── sales-dashboard/  销售看板（同上，四维度切换 + 合计行）
        ├── collection/       回款与超期（同上，期间指标 + 催款清单）
        ├── audit-logs/       操作日志（只读，管理员可见，无 create / edit）
        ├── todos/            待办清单（三类到期的事，每行可点进源单据）
        ├── currencies/       list / create / edit / form-fields（代码编辑时只读）
        └── exchange-rates/   list / create / form-fields（没有 edit）
```

**`suppliers/`、`customers/` 与 `forwarders/` 是同一个数据模型的三种角色视图**，类型都是 `Partner`。
三者都支持**列表 / 卡片双视图**（切换方式与产品页一致，见第 6 节），
并且共用 `components/partner-cards.tsx`——卡片该显示哪些信息只有一份实现。
后端只有一张 `md_partner` 表，三个路径是它按角色切的入口（角色位 `2` / `1` / `4`）。
所以三个目录里的 list 只差默认角色与列，改字段时三边都要看 —— 唯一的实质差异是
货代表单不展示商务条件（贸易术语、账期、信用额度），那几列对货代没有意义。

## 3. 应用装配

`App.tsx` 是唯一的装配点，顺序：`BrowserRouter` → `ConfigProvider`（locale + theme）→ `AntApp` → `Refine` → `Routes`。

```tsx
<Refine
  dataProvider={dataProvider}
  authProvider={authProvider}
  routerProvider={routerProvider}
  resources={[...]}
  options={{ syncWithLocation: true, warnWhenUnsavedChanges: true }}
/>
```

**resources 是导航的唯一来源**：侧边栏菜单、面包屑、页面标题、编辑/详情的返回链接都由它推导。
加一个模块只需要在 `resources` 加一条 + 在 `Routes` 加一组路由，不需要再维护 `navItems` 之类的映射表。

**侧栏分组同样走 resources**：`meta.parent` 指向一条只有 `name` + `meta.label/icon` 的资源
（如 `master-data`、`finance`），`useMenu` 会把它变成 `Menu.SubMenu`，选中项的父组自动展开，
面包屑也多一层组名。加模块时写清 `meta.parent` 就自动落到对应分组，**不要**为分组再写一份配置。
组与组内顺序都由 `resources` 数组顺序决定。

三段式路由：

| 段 | 作用 |
|---|---|
| 受保护段 | `<Authenticated fallback={<CatchAllNavigate to="/login" />}>`，未登录直接踢走 |
| 登录段 | `<Authenticated fallback={<Outlet />}><NavigateToResource resource="dashboard" /></Authenticated>`，已登录访问 `/login` 会被送回首页 |
| 兜底 | `path="*"` → `<Navigate to="/" replace />` |

## 4. 数据流

```
pages/products/list.tsx
   │ 1  useTable({ resource: 'products', onSearch })
   ▼
@refinedev/antd 的 useTable
   │ 2  把分页/筛选/排序规范化成 CrudFilters
   ▼
providers/dataProvider.ts
   │ 3  filters → 后端查询参数（page / page_size / keyword / category / include_deleted）
   ▼
api/client.ts
   │ 4  加 Authorization、注入令牌、fetch、读 X-Renewed-Token
   │ 5  拆包 {code,message,data}：code!==0 抛 ApiError
   ▼
后端 /api/v1/*（dev 经 Vite proxy，生产由反代接管）
```

**筛选走白名单**：`dataProvider` 的 `PASSTHROUGH_FILTERS` 只翻译名单里的条件，其余直接丢弃。
后端没有对应参数时**不要**把它拼进 URL —— 后端会静默忽略，表现成「点了查询没反应」，
比报错难查得多。

名单以 `providers/dataProvider.ts` 为准，**不要在这里抄一份**：抄了就会过期
（本手册曾写着 `from` / `to` / `sales_order_id` 是"已知缺口"，实际前两个早就加进去了，
第三个也随 T-43 补上）。要确认某个条件通不通，grep 一眼 `PASSTHROUGH_FILTERS` 比读文档快。

**错误只有一条路径**：`client.ts` 抛 `ApiError`（带 `code`），refine 捕获后调 `authProvider.onError`；
非 401 错误交回页面，由表单/表格的局部状态展示。页面里不写 `try/catch` 兜底空实现。

## 5. 鉴权与登录态

```
登录：useLogin → authProvider.login → POST /auth/login → setToken + 缓存 identity
进入任一受保护路由：Authenticated → authProvider.check
  ├─ 无令牌        → { authenticated: false, redirectTo: '/login' }
  ├─ 有 identity   → { authenticated: true }（不打接口）
  └─ 无 identity   → GET /auth/profile（刷新页面后的首次校验）
任意请求 401（40101/40102）：client 清令牌 → authProvider.onError 登出并跳 /login
```

**identity 缓存是有意的**：refine 的 `check` 在每次路由切换都会跑，不缓存会让每个页面跳转
都多一次 `/auth/profile`。缓存在模块作用域，登出与 401 时清空。

**令牌续期对页面透明**：剩余有效期不足一半时后端换发新令牌，`client.ts` 从
`X-Renewed-Token` 响应头读出来写回 localStorage。页面与 provider 都不需要感知。

**令牌放 localStorage 是有意的取舍**：换成 httpOnly Cookie 能挡住 XSS 窃取令牌，
但要另做一整套 CSRF 防护（SameSite、双提交、跨站表单），而这一版是内网单页应用。
代价是"一旦有 XSS，令牌就是对方的"——所以**这里没有 XSS 注入面**是硬要求，
2026-10 的审查结论：

- 全仓没有 `dangerouslySetInnerHTML`、`innerHTML` / `insertAdjacentHTML` /
  `document.write`、`<iframe>` / `srcdoc`、`eval` / `new Function`；
- 没有把用户数据拼进 `href` 或 `window.open`（全仓 `href=` 一处都没有，
  下载走 `download()` 的 fetch + `<a download>`，文件名取响应头）；
- 后端返回的错误文案一律以文本渲染（antd `message` / `Alert`），React 默认转义。

**新增富文本、Markdown 渲染、外链预览、或任何"把后端字符串当 HTML/URL 用"的功能时，
先回来读这一条**：那会是第一个注入点，而令牌就在 localStorage 里。

## 6. 模块与约定

| 位置 | 关键点 | 改动注意 |
|---|---|---|
| `theme.ts` | 颜色、圆角、组件级 token 的唯一来源 | 不要在组件里写死颜色；新增色先加进 `brand`。改色号前按 WCAG 正文 4.5:1 校一遍——口径说明、未填占位都是 12.5px 的正文，"次要"不等于"看不清" |
| `index.css` 的 `--gap-*` / `--pad-block` | 结构性间距的密度标尺 | 新加结构间距引用这些别名，不要再写一条 `[data-density='compact']` 覆写；表单项下边距归 `theme.ts` 的 `formItemGap`，不要两边都写 |
| `providers/dataProvider.ts` | 分页与筛选翻译、白名单 | 后端加查询参数时同步 `PASSTHROUGH_FILTERS` |
| `providers/authProvider.ts` | 登录/登出/check/onError/identity | `check` 里别再直接打接口，先看 identity 缓存 |
| `components/fields.tsx` | 业务字段的统一展示件 | 新的通用展示（如 CurrencyTag）加这里，不要各页复制 render；标签别自己挑颜色，按"标签的三档用法"选件 |
| `utils/color.ts` | `mixHex`：两个颜色之间插值 | 只有有序量（逾期分桶那种梯度）用得到；不要拿它给新模块造色号，色号归 `theme.ts` |
| `components/page/page-list.tsx` | 列表页外壳：筛选条 + 表格 + 分页 + 空态，**窄屏自动降级成卡片** | 页面只给「哪些筛选项、哪些列」；`scrollX` 不传按内容自适应，别各页手写宽度 |
| `components/page/record-cards.tsx` | 窄屏的通用卡片视图 + 从 `<Table.Column>` 读列定义（`collectCardColumns`） | 卡片内容由列定义推导，不要为某个模块再写一套卡片；推导不出来会退回表格，不会白屏 |
| `components/page/responsive-table.tsx` | 「宽屏表格 / 窄屏卡片」的同一个开关 | 列表页走 `PageList`；**报表页、详情页、面板与弹窗、以及不走 PageList 的列表页**（待办、首页看板）直接用它 —— 判据是列数：**≥ 4 列就降级**，3 列以内的横滑一屏看得完、卡片化反而更啰嗦，保留表格（客户 360 的出货、导入失败清单）。列定义两种写法都能推导：`<Table.Column>` children 与 `columns={[...]}`。合计要给两份：`summary`（`Table.Summary`）与 `cardSummary`（合计卡），形状不同、数是同一份。操作列是猜的（最后一列 + 无 dataIndex + 只有 render），猜错了用 `cardActionKeys` 覆盖，`[]` 表示没有操作列 |
| `constants/catalog.ts` | 字典类枚举：产品类别、启用状态（**只对账号用**，另有 `CURRENCY_STATUS_OPTIONS` 给币种的归档） | 取值必须与后端 `model` 里的常量一致（`EnableStatusOn = 1`）；后端改枚举时同步这里，不要在页面里另写一份 options |
| `api/client.ts` | 令牌注入、拆包、续期、401 | 所有请求必须经它；页面里直接 `fetch` 会漏掉全部横切逻辑 |
| `pages/*/form-fields.tsx` | 新建与编辑共用的表单字段 | 加字段只改这一处，两个入口自动同步 |
| `components/line-items-editor.tsx` | 报价单 / 订单 / 采购的明细行（数量 × 单价） | 金额只显示不提交，后端算 |
| `components/product-select.tsx` | 任何「选一个产品」的下拉（明细行、库存流水、出入库单明细） | 搜索走后端 `/products?keyword=`，不要退化成 `useSelect` 取满 200 条——本地过滤筛不出客户报的编号，产品过两百条之后后半段还会选不到 |
| `components/product-source-select.tsx` | 明细行的成本货源（报价单） | 按 `product_id` 现查（货源没有启用状态了）；已选的那条被删时仍占位显示，别让它变成一个光秃秃的 id |
| `hooks/use-idempotency-key.ts` | 任何会**重复建单**的新建页（报价 / 订单 / 采购 / 出货 / 合同 / 应收应付 / 出入库单），以及**不在表单里、但每次调用都会新建记录**的接口（订单变更 `POST /sales-orders/:id/changes`） | 表单页挂到 `useForm` 的 `mutationMeta` 上、成功后 `onMutationSuccess` 里轮换；非表单的调用把 `key` 作为 `http.post` 的第三个参数传（`{ 'Idempotency-Key': key }`），成功后手动 `rotate()`。键必须在「用户意图」这层生成：让 http 层每次自动生成等于每次都是新操作，幂等无从谈起 |
| `pages/shipments/allocation-editor.tsx` | 出货分配行（选销售订单行 + 数量 + 装箱数据） | 选行来源是 `/shipment-candidates`，不是订单列表；编辑时传 `excludeShipmentId`；装箱数据（箱数 / 毛净重 / 尺寸）可留空，净重大于毛重由后端拒 |
| `pages/products/profit-panel.tsx` | 产品详情页的「逐票毛利」（T-31 下钻） | 一行一票、点进那一票（毛利对不上要看的是那一票的费用）；口径来自 `/reports/product-profit`，前端不重算；`cost_hidden` 显示「—」 |
| `pages/products/code-panel.tsx` | 产品详情页的「其他编号」面板 | 行内表单录入（一次要补三五个号，弹窗比敲键盘还慢）；没有编辑，改写法就删掉重录；归一化与唯一性由服务端判 |
| `components/doc-chain.tsx` | 单据链（报价 → 合同 → 订单 → 出货 → 应收 → 收款），挂在 5 个详情页顶部 | 每一环三态：可跳转 / 已关联但无页面 / 还没这一环（虚线）。订单页的上下游靠 `useList` 查（`sales_order_id`、`source_type=10`），**不要**为此加接口；应付那侧与采购链不要套这条销售链 |
| `components/attachment-panel.tsx` | 通用附件面板（上传 / 下载 / 删除） | 挂在任何详情页，传 `entityType` + `entityId`（取值见 `AttachmentEntity`）；上传走 `http.postForm`——multipart 的 `Content-Type` 必须由浏览器生成，手写会让服务端解析不出字段；已挂在出货详情与客户 / 供应商详情 |
| `components/authed-image.tsx` + `pages/products/image-panel.tsx` | 产品图片（多图上传 / 排序换主图 / 替换 / 删除） | 取图接口要 `Authorization`，`<img src>` 用不了：一律经 `AuthedImage`（fetch → blob → object URL，按路径缓存）。**主图 = 第一张**，前端不另存标记，「设为主图」就是移到第一位；排序提交完整 ids，409 时提示刷新而不是猜着合并；写操作后同时失效 `products` 的 `list` 与 `detail`，否则列表页会继续显示旧主图。替换要用 `http.putForm`，`http.put` 会把 FormData 序列化成 `{}` |
| `pages/stock/list.tsx` | 当前库存（只读）+ `stockApi.recalc` | 库存没有 create / edit：要调数就补一条流水，不要给这个页面加编辑框 |
| `pages/stock-movements/` | 库存流水列表 + 手工录入 | 只给期初 / 盘盈亏 / 其它（`MANUAL_STOCK_MOVEMENT_OPTIONS`），其余由单据产生 |
| `pages/stock-documents/` | 出入库单（列表 / 新建 / 编辑 / 详情 + 确认） | 类型切换会换往来单位的下拉源（供应商 / 客户）；盘点明细填实盘数，账面由后端确认时取 |
| `pages/contracts/` | 合同（PI / 销售合同）+ `terms-editor.tsx` 付款分期 | 新建页只能"从来源单据生成"：选了来源后用它的总额预填一期全额；金额与汇率没有输入框，改金额请改来源单据 |
| `pages/orders/change-modal.tsx` | 已确认订单的变更（改量 / 改价 / 改交期 + 必填原因），详情页另有变更历史卡片 | 只提交真的变了的行；交期没变就**不带** `delivery_date`（字段省略 = 不动它，空串 = 清掉）；不能增删明细行，要加减货请作废重下 |
| `components/partner-duplicate-hint.tsx` | 客户 / 供应商 / 货代三个表单的「可能已存在同一家」提示 | 挂在 `form-fields` 里、四个参与查重的字段之后；防抖 500ms 调 `/partner-duplicates/candidates`；**只提示不阻断**，编辑时靠 `useWatch('id')` 自动排除自身；失败静默，编码唯一索引仍兜底 |
| `components/partner-merge-modal.tsx` | 主体详情页的「合并到其他主体」 | 方向固定为「当前这条被并走」，即接口的路径 id 是选中的那条、`source_id` 是当前这条——两个 id 是换位的；三道闸（说明、勾选确认、成功后跳到留下的那条）不要省，合并不逆 |
| `components/partner-360-panel.tsx` | 客户详情页的「客户 360」（T-30） | 抬头数字是全量、五块明细各 10 条且每行都能点回源单据；只挂客户侧——供应商那侧这几块基本是空的。断言"有数据"不够，字段名与列名对不上时接口会静默给空串 |
| `pages/customers/`、`pages/suppliers/`、`pages/forwarders/` 列表 | 三张列表都有「负责人」列 | 空值显示灰字「未分配」而不是空白——空白会被读成"接口没返回"，而未分配是一种正常状态（公共池） |
| `components/partner-avatar.tsx` | 往来单位的头像 / Logo（列表的名称列、卡片头部、以及任何需要"认人"的位置） | 只有这一个实现，两个视图调用同一个组件、同一个默认尺寸（`PARTNER_AVATAR_SIZE`）——换个视图就换个长相，会被当成两条数据。降级顺序：`logo_url` 取到字节 → 名称占位标识（中文取首字、西文取前两个词首字母，配色按名称哈希）；有图但加载失败也降级，不要显示浏览器破图。真图走 `useAuthedUrl`，与公司 logo 同通道 |
| `components/partner-cards.tsx` | 三张主体列表的卡片视图 + 卡片里的删除 / 恢复（`PartnerRowActions`）+ 分页转换（`toCardPager`） | 卡片该显示什么只有一份实现，客户卡多的「分组 / 区域 / 结算 / 账期」由 `showCustomerFacts` 开关——货代表单原本就不给商务条件，这里是同一个结论。`PartnerRowActions` 同时用在表格操作列与卡片右下角，两边按钮必须一样 |
| `pages/todos/` | 待办清单（T-34） | 每块副标题都写着判定口径（"早于今天（当天不算）"/"到期日 <= 今天（当天就算）"）——这类页面的争议永远在"为什么它算在这里"；口径里的 `<` 必须写成 `{'…'}`，JSX 文本里会被当成标签开头 |
| `components/partner-owner-modal.tsx` | 三个主体详情页的「转移负责人」 | 选人列表来自 `GET /users`（只列启用账号），下拉第一项是「未分配（公共池）」——离职与暂时没人跟都要能把归属清掉；弹窗里必须写清"这个客户的单据会一起跟着走" |
| `pages/clauses/`、`pages/contracts/clause-picker.tsx` | 条款模板库（T-27）+ 合同新建时的条款多选 | 模板是**主数据**，合同上存的是快照：改模板不影响已生成的合同（页面文案要写这句）。选择器只提交 `clause_ids`，**不要连正文一起传**——传了就没有"快照"这回事了；多选顺序即合同上的顺序 |
| `pages/audit-logs/` | 操作日志（T-33） | 只读：没有编辑与删除按钮，后端也只有 List。`body` 是**提交内容**（已脱敏、超长截断），不是前后 diff；筛选里那个关键字是"手里只有单号"时的入口 |
| `pages/collection/` | 回款与超期（T-29） | 上半是**期间流量**（到期 / 回款 / 回款率）、下半是**截至截止日的存量**（超期清单），两者不互相校验；回款率超 100% 是正常现象（在还上个月的账），不要在页面上"修正"它 |
| `pages/purchases/milestone-panel.tsx` | 采购详情页的「跟单节点」（T-38） | 行是采购明细、列是五节点，一格两行（计划 / 实际）；**一次只提交一格**——补录是部分的，整体回传会覆盖别人刚填的；逾期上色只用于看，统计一律以后端为准；新增筛选参数要同步 `PASSTHROUGH_FILTERS`（`overdue_node` 已加） |
| `pages/customers/import-modal.tsx` | 客户列表的「导入 CSV」（T-35） | 三步：选文件 → 校验（`dry_run=true`）→ 确认导入；「确认导入」在校验前禁用，因为它会真的写库；失败行可下载 CSV（同样带 BOM）；模板就用导出的那份文件 |
| `api/exports.ts` + `http.download` | 客户 / 产品列表的「导出 CSV」（T-35） | 下载必须走 `http.download`（fetch blob）而不是 `window.open`——后者带不上 Authorization 头，只会拿到 401；按钮文案要写明"忽略当前筛选、上限 10000 行"，否则用户以为导出的是筛选后的那一批 |
| `pages/sales-dashboard/`、`pages/shipments/profit-card.tsx`、`pages/products/` | 成本与毛利（T-36） | 后端把成本类金额清成 0 并带 `cost_hidden`，界面必须显示「—」而不是 0——0 会被读成"没成本 / 没赚钱"，与"你看不到"正好相反；`cost_hidden` 时顶部的「缺少采购成本」警告不再弹，两者原因不同 |
| `pages/sales-dashboard/` | 销售看板（T-28）+ 产品行下钻（T-31） | 金额与毛利率一律用后端给的数，**不在前端重算**——重算一遍就有两个毛利率；订单额（下单日）与收入（出货日）分两列标时间口径；`truncated` 时提示"合计是全量"，别让人拿当前几十行自己加 |

### 一个主数据模块的标准四件套

```
pages/xxx/
├── list.tsx          useTable + 筛选条 + Table + CreateButton
├── create.tsx        useForm({ action: 'create' }) + Create 外壳
├── edit.tsx          useForm({ action: 'edit' }) + Edit 外壳
├── show.tsx          useShow + Descriptions
├── form-fields.tsx   字段与校验规则（create/edit 共用）
└── index.ts          对外只导四个页面组件
```

**单据类模块比这多一层**：表头之外还有明细 / 分配行，行编辑器单独成文件
（`line-items-editor.tsx`、`pages/shipments/allocation-editor.tsx`），
并由 `create.tsx` 与 `edit.tsx` 共用。行的隐藏主键（`product_id`、`sales_order_item_id`）
必须用 hidden 的 `Form.Item` 渲染出来——不渲染就提交，这一行到后端就是空的。

### 列表页的双视图（表格 / 卡片）

产品列表与三张往来单位列表（客户 / 供应商 / 货代）都提供表格与卡片两种视图，
开关在 `hooks/use-list-view.ts`，页面侧只用
`const { view, setView } = useListView('gowork:xxx:view')`。

切换控件统一用 antd `Segmented`（`BarsOutlined` / `AppstoreOutlined`），放在
`PageList` 的 `extra` 里、其它操作按钮左侧，与产品列表保持同一位置——同一个功能在不同页
换个地方放，等于每次都要重新找。storageKey 按资源分开写（`gowork:customers:view` 等），
一个人习惯卡片视图不该连累了另一个列表。

要点只有三条，其余都是排版：

- **两种视图共用同一个 `useTable`**。切换只换渲染方式，不换数据源；组件不卸载，
  所以筛选、排序、页码天然保留。各自建一套 `useTable` 会变成"切一次重新请求一次"。
- **视图存在两处**：URL 的 `?view=card` 优先（发链接能看到同一屏），其次是
  `localStorage`（个人习惯），都没有才用默认。优先级写在 hook 里，页面不要自己读。

  **窄屏（< 768px）强制卡片**，压过上面三个来源。hook 额外返回 `narrow`，
  页面用它把 `Segmented` 藏起来 —— 一个点了没反应的开关比没有开关更让人困惑。
  强制只在渲染层生效，不改 URL 也不改 localStorage，窗口拉宽后原来选的视图还在。
- **卡片视图要自己渲染分页**：`Table` 自带的分页条在卡片模式下根本不存在，
  用 `tableProps.pagination` 转成 `<Pagination>`。滚动位置由 hook 在切换后按新的
  文档高度夹一次，不这么做会停在空白处（卡片比表格矮的时候）。

卡片栅格用 `auto-fill + minmax(…, 1fr)`，不为每个断点写列数；断点只调间距。
最小宽度按卡片内容定：产品卡一张图加两行字用 200px，往来单位卡有六到九行
「标签：值」，用 240px——再窄就只剩省略号，看不出谁是谁的值。
排序控件与表头箭头必须驱动同一个 `sorters`，否则"点了表头、下拉框没变"。

### 标签的三档用法

颜色是**语义**，不是装饰。加标签前先问它属于哪一档，用对应的件，不要自己挑色号：

| 档 | 用什么 | 例子 |
|---|---|---|
| **状态**（有"进行中 / 完成 / 出错"） | `OptionTag` + 字典的 `tone`，或 `<Tag color="success\|processing\|warning\|error">` | 订单状态、结算状态、报关状态 |
| **强调**（"这一条是特殊的那一个"） | `AccentTag`（主色淡底） | 默认地址、默认仓库、本币、主联系人、主图、以销定采、个人主体 |
| **有序量**（同一个量在变大） | `HeatTag`：warning → error 四档插值 | 逾期天数分桶 |

- **中性**（身份 / 类别）：直接 `<Tag>`，不给颜色。客户 / 供应商 / 货代、公司主体、
  币种、出入库类型都在这里 —— 角色是身份不是状态，三个分不出的蓝只会让人以为
  存在三种需要不同处理的状态。
- **不要**用 antd 的固定调色板（`blue` / `geekblue` / `cyan` / `gold` / `orange` /
  `volcano` / `red` / `green`）：它们不跟 `theme.ts` 走，暗色下是 antd 自己算的另一套。
- **不要**在页面里写 `statusColor()`：字典的 `tone` 就是颜色的唯一来源，
  再写一份必然出现"同一个状态在两页上两种颜色"（合同页曾把「已取消」画成灰色）。
- 五档语义色的字色是压深过的（亮色 800 档）：状态标签是 12px 正文，
  压在自己的浅底上要 ≥ 4.5:1；改 `theme.ts` 里那几档之前先算一遍对比度。

### 新增一个模块的完整步骤

1. `src/types/index.ts` 加实体类型，字段与后端 json tag 一致（snake_case）。
2. 复制 `pages/suppliers/` 整个目录改名，替换字段与列定义。
3. `App.tsx` 的 `resources` 加一条（含 `meta.label` 与 `meta.icon`），`Routes` 加一组四条路由。
4. `pnpm run typecheck` 必须无错（`noUnusedLocals` 会拦住没用的 import）。

### 硬性规则

- **跨目录导入用 `@/` 前缀，同目录用 `./`**。别名在 `tsconfig.json` 的 `paths` 与
  `vite.config.ts` 的 `resolve.alias` **两处**同时声明：tsc 只读前者，vite/rollup 只读后者，
  缺一边就是「typecheck 过、build 报 Could not resolve」。改完必须跑 `pnpm run build`。
- **不要手写基础控件**：按钮、表格、表单、分页、弹窗、下拉一律用 antd；
  refine 的 `List` / `Create` / `Edit` / `Show` 负责页面骨架、面包屑、返回与保存按钮。
- **表单校验只做必填与格式**，业务校验（唯一编码等）交给后端，按 `ApiError` 的文案展示。
- **加依赖用 `pnpm add`**，锁文件一起提交；不要手改 `pnpm-lock.yaml`。
- **删除确认用 antd 的 `Popconfirm`**（`DeleteButton` 已内置），不要 `window.confirm`。

## 7. 运行方式

```bash
task frontend:dev         # Vite 开发服务器 :5173，/api 代理到 :8080
task frontend:build       # tsc --noEmit && vite build → dist/
task frontend:typecheck   # 只做类型检查
task frontend:preview     # 预览 dist（会自动先 build）
task frontend:install     # pnpm install
task frontend:install:ci  # pnpm install --frozen-lockfile（CI 用）
```

不用 task 时：`pnpm install && pnpm run dev`。

前置：Node ≥ 20、pnpm ≥ 11（版本锁在 `packageManager`，`corepack enable pnpm` 对齐）。
**用 npm / yarn 装依赖会被 `preinstall` 守卫拦下**，这是有意的。

前端只认 `/api` 相对路径，靠 `vite.config.ts` 的 proxy 转发；生产不需要改代码，由网关接管。

## 8. 构建产物与体积

| chunk | gzip | 说明 |
|---|---|---|
| antd | ~281KB | antd 5 + rc-* + dayjs（含 `@ant-design/pro-layout` 约 200KB 的连带依赖） |
| react | ~89KB | react 19 + react-dom + react-router |
| vendor | ~119KB | refine core 的传递依赖（lodash / qs / papaparse 等） |
| refine | ~30KB | `@refinedev/*` |
| index | ~8KB | 业务代码 |

按依赖族用函数式 `manualChunks` 拆包（`vite.config.ts`）：三族版本不同步，分开缓存后
升级一族不会让另外两族的缓存一起失效。**不要为了"少几个文件"改回单包**，那会让首屏
必须等整个 vendor 下载完。

可优化方向（未做）：`@refinedev/antd` 会连带打包 `@ant-design/pro-layout`，砍掉它大约能省 200KB，
代价是放弃 refine 的 `List`/`Form` 组件改用手写骨架 —— 除非体积成为硬指标，不建议。

## 9. 已知陷阱

| 现象 | 原因 | 处理 |
|---|---|---|
| 用 `@/xxx` 导入报 `Could not resolve` | 只改了 `tsconfig.json` 的 `paths` 或只改了 `vite.config.ts` 的 `resolve.alias` | 两处必须同时存在；改完跑 `pnpm run build` 验证 |
| 从 `@refinedev/antd` 导入 `ThemedLayoutV2` 报「没有此导出」 | v5 已改名为 `ThemedLayout` | 以 `node_modules/@refinedev/antd` 的实际导出为准，别照 v4 文档写 |
| `useSelect` 解构 `selectProps` 得到 undefined | v5 移除了 `selectProps`，直接返回 `options` | 自己接 `options` + `loading` + `filterOption` |
| 下拉里永远只有十几条选项 | `useSelect` 没配 `pagination: { mode: 'off' }`，或 dataProvider 没把 off 翻译成一次取满 | 见 `dataProvider.buildListQuery`：off → `page_size=200` |
| 筛选条件填了但结果没变 | filter 的 field 不在 dataProvider 白名单里，被丢弃 | 后端加参数时同步 `PASSTHROUGH_FILTERS` |
| 配了 `Menu.darkItem*` 却一点变化都没有 | `@refinedev/antd` 的 `ThemedSider` 把 `backgroundColor: token.colorBgElevated` 内联写进 `Layout.Sider`，`Menu` 也没传 `theme="dark"`，所以侧栏实际是白底 | 侧栏定为**浅色**（token 见 `theme.ts`）；真要深色得自己实现 Sider 组件，不要在 token 里配 `darkItem*` 骗自己 |
| 刷新页面被踢回登录 | 令牌过期或后端会话被清（登出、重启且内存缓存未落盘） | 预期行为；检查后端 `cache.memory.persist` |
| 续期在生产不生效，状态码却都是 200 | 跨域下 `X-Renewed-Token` 未在 `Access-Control-Expose-Headers` 暴露 | 见后端 `AGENTS.md`「令牌续期」；开发期同源复现不出来 |
| 每个页面跳转都多一次 `/auth/profile` | `authProvider.check` 里的 identity 缓存被清掉了 | 检查 `login` / `check` / `onError` 三处对 `identity` 的赋值 |
| `npm install` 报 `[gowork-frontend] 本项目使用 pnpm` | `package.json` 的 `preinstall` 守卫 | 用 pnpm；`package-lock.json` 被忽略，删掉即可 |
| 某个下拉的选项和后端对不上 | 页面里自己写了一份 options，没走 `constants/catalog.ts` | 字典只留在 catalog.ts 一处，页面 import 它 |
| 改了 `pnpm-workspace.yaml` 不生效 | pnpm 设置只能放这里 | 见根 `README.md`「前端只认 pnpm」 |
| 客供两用的主体在列表里显示成"未知角色" | 写了 `role === 1` / `role === 2` | `Partner.role` 是位掩码，必须按位与：`(role & PARTNER_ROLE.supplier) !== 0`，展示统一用 `RoleTag` |
| 新加的表单字段提交后没生效 | 后端 `PartnerInput` 里没有该字段，或字段名与 JSON tag 不一致 | 表单一律提交 `snake_case`，与后端 JSON tag 对齐；`role` / `entity_type` 为 `nil` 表示"不改"而不是"改成 0" |
