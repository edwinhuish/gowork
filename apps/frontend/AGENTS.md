# apps/frontend —— React 管理端（面向 AI 协作者）

范围：**只讲前端**。项目整体目标见根 `AGENTS.md`，后端机制见 `apps/backend/AGENTS.md`。
本文讲"页面怎么组织、数据怎么流动、加模块照什么套路"。

---

## 1. 职责边界

**负责**：登录、概览、产品 / 供应商 / 客户的增删改查界面、列表分页与筛选、
登录态维持、主题与布局。

**当前只有共享主数据的界面。** 系统定位是 ERP + CRM 集成，但 ERP 侧的
订单/库存/出货、CRM 侧的跟进/商机都还没有页面。加的时候不需要新建导航表——
在 `resources` 里加一条就出现在侧栏（见第 3 节）。业务对象叫什么名、
字段中文怎么写，先查 `docs/glossary.md`。

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
    │   └── auth.ts  dashboard.ts      （CRUD 已改由 dataProvider 直连，不再逐个写 xxxApi）
    ├── components/
    │   ├── layout/AppLayout.tsx   ThemedLayout + 品牌块（GoWork / ERP · CRM 两行）+ 顶栏
    │   └── fields.tsx       StatusTag / RoleTag / EntityTypeTag / Money / CategoryTag / Mono / Empty
    ├── constants/catalog.ts 产品类别字典、启用状态、主体类型选项、角色位常量
    ├── utils/format.ts      时间格式化
    └── pages/
        ├── login/index.tsx
        ├── dashboard/index.tsx
        ├── products/        list / create / edit / show / form-fields / index
        ├── suppliers/       同上
        └── customers/       同上
```

**`suppliers/` 与 `customers/` 是同一个数据模型的两种角色视图**，类型都是 `Partner`。
后端只有一张 `md_partner` 表，`/suppliers` 与 `/customers` 是它按角色切的入口。
所以两个目录里的 list 只差默认角色与列，改字段时两边都要看。

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
   │ 3  filters → 后端查询参数（page / page_size / keyword / category / status）
   ▼
api/client.ts
   │ 4  加 Authorization、注入令牌、fetch、读 X-Renewed-Token
   │ 5  拆包 {code,message,data}：code!==0 抛 ApiError
   ▼
后端 /api/v1/*（dev 经 Vite proxy，生产由反代接管）
```

**筛选走白名单**：`dataProvider` 只翻译 `keyword` / `category` / `supplier_id` / `status` / `country`，
其余条件直接丢弃。后端没有对应参数时**不要**把它拼进 URL —— 后端会静默忽略，
表现成「点了查询没反应」，比报错难查得多。

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

## 6. 模块与约定

| 位置 | 关键点 | 改动注意 |
|---|---|---|
| `theme.ts` | 颜色、圆角、组件级 token 的唯一来源 | 不要在组件里写死颜色；新增色先加进 `brand` |
| `providers/dataProvider.ts` | 分页与筛选翻译、白名单 | 后端加查询参数时同步 `PASSTHROUGH_FILTERS` |
| `providers/authProvider.ts` | 登录/登出/check/onError/identity | `check` 里别再直接打接口，先看 identity 缓存 |
| `components/fields.tsx` | 业务字段的统一展示件 | 新的通用展示（如 CurrencyTag）加这里，不要各页复制 render |
| `constants/catalog.ts` | 字典类枚举：产品类别、启用状态 | 取值必须与后端 `model` 里的常量一致（`EnableStatusOn = 1`）；后端改枚举时同步这里，不要在页面里另写一份 options |
| `api/client.ts` | 令牌注入、拆包、续期、401 | 所有请求必须经它；页面里直接 `fetch` 会漏掉全部横切逻辑 |
| `pages/*/form-fields.tsx` | 新建与编辑共用的表单字段 | 加字段只改这一处，两个入口自动同步 |

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
| 侧边栏菜单文字在深色底上看不清 | 只设了 `Menu.itemSelectedBg`，没设 `darkItem*` | 深色 sider 必须配 `Menu.darkItemBg/darkItemColor/darkItemSelectedBg` |
| 刷新页面被踢回登录 | 令牌过期或后端会话被清（登出、重启且内存缓存未落盘） | 预期行为；检查后端 `cache.memory.persist` |
| 续期在生产不生效，状态码却都是 200 | 跨域下 `X-Renewed-Token` 未在 `Access-Control-Expose-Headers` 暴露 | 见后端 `AGENTS.md`「令牌续期」；开发期同源复现不出来 |
| 每个页面跳转都多一次 `/auth/profile` | `authProvider.check` 里的 identity 缓存被清掉了 | 检查 `login` / `check` / `onError` 三处对 `identity` 的赋值 |
| `npm install` 报 `[gowork-frontend] 本项目使用 pnpm` | `package.json` 的 `preinstall` 守卫 | 用 pnpm；`package-lock.json` 被忽略，删掉即可 |
| 某个下拉的选项和后端对不上 | 页面里自己写了一份 options，没走 `constants/catalog.ts` | 字典只留在 catalog.ts 一处，页面 import 它 |
| 改了 `pnpm-workspace.yaml` 不生效 | pnpm 设置只能放这里 | 见根 `README.md`「前端包管理」 |
| 客供两用的主体在列表里显示成"未知角色" | 写了 `role === 1` / `role === 2` | `Partner.role` 是位掩码，必须按位与：`(role & PARTNER_ROLE.supplier) !== 0`，展示统一用 `RoleTag` |
| 新加的表单字段提交后没生效 | 后端 `PartnerInput` 里没有该字段，或字段名与 JSON tag 不一致 | 表单一律提交 `snake_case`，与后端 JSON tag 对齐；`role` / `entity_type` 为 `nil` 表示"不改"而不是"改成 0" |
