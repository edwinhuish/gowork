# apps/frontend —— React 管理端（面向 AI 协作者）

范围：**只讲前端**。项目整体目标见根 `AGENTS.md`，后端机制见 `apps/backend/AGENTS.md`。
本文讲"页面怎么组织、数据怎么流动、加页面照什么套路"。

---

## 1. 职责边界

**负责**：登录、概览、产品 / 供应商 / 客户的增删改查界面、列表分页与搜索、登录态维持。

**不负责**：

| 不属于这里 | 归哪 |
|---|---|
| 业务规则（唯一性、状态流转、金额计算） | 后端 service 层；前端只做必填与格式校验 |
| 权限判断 | 后端；前端最多按 `user.role` 隐藏入口 |
| 服务端会话 | 后端；前端只持有令牌 |
| 状态管理库、UI 组件库 | 刻意不引入，见根 `AGENTS.md` 第 1 节 |

技术栈：React 19 + TypeScript 5.9（`strict`）+ Vite 6 + react-router 7，包管理 pnpm 12。
**没有** ESLint / Prettier / Tailwind：类型检查靠 `tsc --noEmit`，样式是 `src/index.css` 里的普通 CSS 类。

## 2. 目录结构

```
apps/frontend/
├── index.html
├── vite.config.ts          # 端口 5173；/api 代理到 :8080；resolve.alias 的 @ -> src
├── tsconfig.json           # strict + noUnusedLocals/Parameters；paths 的 @/* -> src/*
├── package.json            # packageManager: pnpm@12.10.1；scripts: dev/build/typecheck/preview
├── pnpm-workspace.yaml     # pnpm 设置（allowBuilds 放行 esbuild），不是工作区划分
├── .npmrc                  # engine-strict 等 pnpm 侧配置
└── src/
    ├── main.tsx            应用挂载（StrictMode）
    ├── App.tsx             路由表 + 登录守卫 RequireAuth
    ├── index.css           全部样式（一个文件，525 行）
    ├── types/index.ts      与后端 dto 对齐的类型（PageResult / UserInfo / Product / ...）
    ├── api/
    │   ├── client.ts       统一 fetch：令牌注入、响应拆包、ApiError、401 广播、续期令牌替换
    │   ├── auth.ts  product.ts  supplier.ts  customer.ts  dashboard.ts
    ├── hooks/
    │   ├── usePagedResource.ts   列表页通用状态机（分页/搜索/加载/错误/竞态）
    │   └── useFormDraft.ts       新建表单草稿（sessionStorage，被打断后可找回）
    ├── store/
    │   └── auth.tsx        AuthContext：当前用户 + login/logout，刷新时用令牌换身份
    ├── components/
    │   ├── Layout.tsx      侧边导航 + 顶栏 + <Outlet />
    │   ├── Modal.tsx  Field.tsx  Pagination.tsx  StatusBadge.tsx   无业务语义的通用件
    └── pages/
        ├── Login.tsx
        ├── Dashboard.tsx   概览统计
        └── Products.tsx  Suppliers.tsx  Customers.tsx   列表 + 弹窗增改删
```

## 3. 数据流

```
pages/Products.tsx
   │ 1  usePagedResource(productApi.list)        传入 fetchPage 闭包
   ▼
hooks/usePagedResource.ts
   │ 2  管理 page / keyword / loading / error / tick；用 ref 存闭包避免 stale closure
   │ 3  cancelled 标记丢弃过期响应（防竞态：快速翻页时旧请求后到不覆盖新数据）
   ▼
api/product.ts
   │ 4  只做 path + 参数拼装，返回 Promise<PageResult<Product>>
   ▼
api/client.ts
   │ 5  统一加 Content-Type、从 localStorage 取令牌加 Authorization
   │ 6  fetch('/api/v1' + path)
   │ 7  响应头带 X-Renewed-Token 就替换本地令牌（服务端滑动续期）
   │ 8  拆包 {code,message,data}：code!==0 → 抛 ApiError；40101/40102 → 清令牌 + 广播 AUTH_EXPIRED_EVENT
   ▼
后端 /api/v1/*（dev 时经 Vite 代理，生产由反代接管）
```

**错误处理只有一条路径**：任何失败都变成 `ApiError`（带 `code`），由页面 catch 后写入
`usePagedResource` 的 `error` 或表单的局部错误展示。页面里不出现 `try/catch` 的兜底空实现。

### 登录态失效时发生什么

```
任意请求返回 401
   │
   ├─ client.ts 清 localStorage 的令牌
   ├─ client.ts 广播 window 事件 gowork:auth-expired
   ▼
store/auth.tsx 订阅该事件 → setUser(null)
   ▼
App.tsx RequireAuth 看到 user 为空 → <Navigate to="/login" state={{from}}>
   ▼
Login 登录成功 → navigate(from) 回到被打断的那一页
   ▼
新建弹窗里的草稿由 useFormDraft 从 sessionStorage 取回
```

**为什么用事件而不是直接 import**：`client -> store -> api/auth -> client` 会形成循环依赖，
所以清理动作走广播。**注意必须同时清 localStorage 和内存里的 `user`**——
只清前者时 `RequireAuth` 判的还是 `user`，用户会卡在页面上反复报错，直到手动刷新。
这是曾经的真实 bug，改动这一块时先看 `store/auth.tsx` 里那个订阅。

**令牌续期不需要前端做什么**：剩余有效期不足一半时服务端自动换发，前端只是在
`client.ts` 里把响应头里的新令牌写回 localStorage。

**登录态**（`store/auth.tsx`）：

```
首次挂载 → 有令牌？ → 调 authApi.profile() 换身份
                      ├─ 成功 → setUser
                      └─ 失败 → setToken(null)（静默清除，等守卫送回登录页）
登录成功 → setToken(token) + setUser(user) + navigate(from)
登出     → 调 /auth/logout（失败也不阻塞）+ 清本地令牌 + setUser(null)
收到 AUTH_EXPIRED_EVENT → setUser(null)（令牌已被 client 清掉）
```

`RequireAuth` 在 `loading` 期间显示占位，避免刷新时先闪一下登录页；
未登录时跳 `/login` 并带上 `state.from`，登录后原路返回。

## 4. 核心模块

| 模块 | 关键点 | 改动注意 |
|---|---|---|
| `api/client.ts` | `BASE_URL = '/api/v1'`；令牌键 `gowork_token`；`toQuery` 自动跳过空值 | 所有请求必须走这里。页面里直接 `fetch` 会漏掉令牌注入、续期替换与 401 广播 |
| `api/*.ts` | 每个领域一个对象（`productApi` 等），方法与后端路由一一对应 | 返回值类型必须来自 `types/index.ts`，不要就地写匿名类型 |
| `hooks/usePagedResource.ts` | 分页（`PAGE_SIZE = 15`）、关键字、`reload()`、竞态保护 | 额外筛选条件放进各页面自己的 `fetchPage` 闭包，不要改 hook 签名 |
| `hooks/useFormDraft.ts` | 新建表单实时写 sessionStorage；`take()` 取回、`clear()` 清掉 | 只在新建态挂载（`modalOpen && !editing`）；主动关闭要调 `clear()`，否则下次新建会冒出旧内容 |
| `store/auth.tsx` | Context + `useMemo`，只放跨页面共享的登录态 | 页面级状态不要塞进来 |
| `components/` | `Modal`（受控显隐）、`Field`（label + 控件 + 错误文案）、`Pagination`、`StatusBadge` | 保持无业务语义；出现"产品专用"逻辑就该留在 page 里 |
| `pages/` | 三个列表页结构一致：筛选区 + 表格 + 分页 + 弹窗表单 | 新增页面照 `Products.tsx` 抄骨架最快 |
| `App.tsx` | 路由表；`*` 兜底跳 `/`；`/login` 在守卫外层 | 新增页面要同时加 `Layout` 的 `navItems` 与 `titleMap` |

## 5. 调用的后端接口

后端完整清单在 `apps/backend/AGENTS.md` 第 5 节，这里只列前端实际用到的。

| 前端 | 方法 + 路径 | 返回类型 |
|---|---|---|
| `authApi.login` | `POST /auth/login` | `LoginResponse` |
| `authApi.profile` | `GET /auth/profile` | `UserInfo` |
| `authApi.logout` | `POST /auth/logout` | — |
| `dashboardApi.summary` | `GET /dashboard/summary` | `DashboardSummary` |
| `productApi.list / get / create / update / remove` | `GET`+`POST` `/products`；`GET`+`PUT`+`DELETE` `/products/:id` | `PageResult<Product>` / `Product` |
| `supplierApi.list / create / update / remove` | `/suppliers` 同构（**没有 get**，列表数据够用） | `PageResult<Supplier>` / `Supplier` |
| `customerApi.list / create / update / remove` | `/customers` 同构（同样没有 get） | `PageResult<Customer>` / `Customer` |

列表查询参数：`page`、`page_size`、`keyword`，产品额外 `category` / `status`。

## 6. 运行方式

```bash
task frontend:dev         # Vite 开发服务器 :5173，/api 代理到 :8080
task frontend:build       # tsc --noEmit && vite build → dist/
task frontend:typecheck   # 只做类型检查
task frontend:preview     # 预览 dist（会自动先 build）
task frontend:install     # pnpm install
task frontend:install:ci  # pnpm install --frozen-lockfile（CI 用）
```

不用 task 时：

```bash
pnpm install
pnpm run dev
```

前置：Node ≥ 20、pnpm ≥ 11（版本锁在 `package.json` 的 `packageManager`，`corepack enable pnpm` 即可对齐）。
**用 npm / yarn 装依赖会被 `preinstall` 守卫拦下**，这是有意的。

开发和后端分离跑时，前端只认 `/api` 相对路径，靠 `vite.config.ts` 的 proxy 转发；
生产部署不需要改代码，由网关把 `/api` 指到后端。

## 7. 开发约定

### 新增一个页面（照 Products 抄）

1. `src/types/index.ts` 加实体类型，字段与后端 json tag 一致（snake_case）。
2. `src/api/xxx.ts` 写 `xxxApi`（list/get/create/update/remove），复用 `http` 与 `toQuery`。
3. `src/pages/Xxx.tsx`：`usePagedResource(xxxApi.list)` + 表格 + `Modal` 表单；
   表单校验只做必填与格式，业务校验交给后端，按 `ApiError.code` 展示后端文案。
4. `src/App.tsx` 挂路由；`src/components/Layout.tsx` 的 `navItems` 与 `titleMap` 各加一项。
5. `pnpm run typecheck` 必须无错（`noUnusedLocals` 会拦住没用到的 import）。

### 硬性规则

- **请求只走 `api/client.ts`**，不在页面里 `fetch`。
- **类型只写在 `types/index.ts`**，api 层与页面都从那里取。
- **跨目录导入用 `@/` 前缀，同目录用 `./`**：`@/api/product`、`@/components/Modal`；
  `./App`、`./client`、`./index.css` 这类同目录引用不必绕 alias。
  别名在 `tsconfig.json` 的 `paths` 与 `vite.config.ts` 的 `resolve.alias` **两处**同时声明——
  tsc 只读前者，vite/rollup 只读后者，缺一边就是「typecheck 过、build 报 Could not resolve」。
  两处必须一起改，改完 `pnpm run build` 才能证明对齐。
- **表单用受控组件**，`Field` 负责 label 与错误文案，错误状态存在页面 state 里。
- **列表页一律用 `usePagedResource`**，不要各写一套 loading/error 逻辑。
- **样式写在 `src/index.css`**，复用已有类名（见下），不在组件里堆 inline style。
  现有类分四组：布局 `app-shell` / `sidebar` / `main` / `topbar` / `content`；
  列表 `toolbar` / `card` / `table-wrap` / `table-empty` / `pagination` / `cell-*`；
  表单 `form-grid` / `form-item` / `form-label` / `input` / `select` / `form-error`；
  弹窗与按钮 `modal*` / `btn` / `btn-primary` / `btn-danger` / `btn-sm` / `link-btn`。
  新增样式直接追加到该文件，不引入 CSS Modules 或预处理器。
- **加依赖用 `pnpm add`**，并把 `pnpm-lock.yaml` 一起提交；不要手改锁文件。
- 删除确认目前三个页面都用 `window.confirm`（一行搞定，行为一致）。
  如果哪天要统一视觉，再换成 `Modal`——别一个页面一个写法。

## 8. 已知陷阱

| 现象 | 原因 | 处理 |
|---|---|---|
| 用 `@/xxx` 导入报 `Could not resolve` | 只改了 `tsconfig.json` 的 `paths` 或只改了 `vite.config.ts` 的 `resolve.alias` | 两处必须同时存在，见第 7 节「开发约定」的导入约定；改完跑 `pnpm run build` 验证 |
| 刷新页面被踢回登录 | 令牌过期或后端会话被清（登出、重启且内存缓存未落盘） | 预期行为；`client.ts` 已自动清令牌，检查后端 `cache.memory.persist` |
| 401 后页面还在，只是每条请求都报错 | 只清了 localStorage 没清内存里的 `user`，`RequireAuth` 判的还是 `user` | 检查 `store/auth.tsx` 里对 `AUTH_EXPIRED_EVENT` 的订阅是否还在 |
| 续期在生产环境不生效，状态码却都是 200 | 跨域下 `X-Renewed-Token` 未在 `Access-Control-Expose-Headers` 里暴露，JS 读不到 | 见后端 `AGENTS.md`「令牌续期」；开发期走 vite proxy 同源，复现不出来 |
| 新建弹窗里冒出上次没提交的旧内容 | 上次是主动关闭，但没调 `draft.clear()` | 关闭/取消统一走页面里的 `close()`；只有被登录态打断时才该保留草稿 |
| 列表每页 15 条，和后端默认 20 不一致 | `usePagedResource` 的 `PAGE_SIZE = 15` 显式传参 | 有意为之；改这里就同时看后端 `MaxPageSize=200` 上限 |
| 快速翻页后显示的是旧数据 | 组件卸载/切换时旧请求后到 | `usePagedResource` 已用 `cancelled` 标记丢弃；自己写的请求同样要处理 |
| `npm install` 报 `[gowork-frontend] 本项目使用 pnpm` | `package.json` 的 `preinstall` 守卫 | 用 pnpm；npm 已生成的 `package-lock.json` 被 `.gitignore` 忽略，删掉即可 |
| 改了 `pnpm-workspace.yaml` 不生效 | pnpm 设置只能放这里，放 `.npmrc` 或 `package.json` 的 `pnpm` 字段都会被忽略 | 保持现状，见根 `README.md`「前端包管理」一节 |
