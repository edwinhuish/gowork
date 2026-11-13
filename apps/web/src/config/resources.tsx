import {
  AccountBookOutlined,
  BankOutlined,
  BookOutlined,
  AlertOutlined,
  AppstoreOutlined,
  BellOutlined,
  BarChartOutlined,
  LineChartOutlined,
  CommentOutlined,
  ContactsOutlined,
  DashboardOutlined,
  DatabaseOutlined,
  DollarOutlined,
  FileDoneOutlined,
  FileProtectOutlined,
  FileSearchOutlined,
  FileTextOutlined,
  FundOutlined,
  GlobalOutlined,
  HistoryOutlined,
  ImportOutlined,
  InboxOutlined,
  MoneyCollectOutlined,
  PieChartOutlined,
  ProfileOutlined,
  SendOutlined,
  SettingOutlined,
  ShoppingCartOutlined,
  ShoppingOutlined,
  ShopOutlined,
  SolutionOutlined,
  UserAddOutlined,
  UserOutlined,
  SwapOutlined,
  TeamOutlined,
  TransactionOutlined,
  WalletOutlined,
} from '@ant-design/icons'
import type { ResourceProps } from '@refinedev/core'

/**
 * 资源清单：导航、面包屑、页面标题、返回链接与标签页标题的唯一来源。
 *
 * 从 App.tsx 抽出来独立成文件，是因为它现在有第二个读者 ——
 * 标签页要按同一份清单把 pathname 翻译成人话（`/products/create` → 「新建产品」），
 * 若是留在 App.tsx 里，标签页组件就得反过来 import App，形成循环。
 *
 * 加模块仍然只是在这里加一条：侧栏分组、菜单选中态、标签页标题都会自动跟上，
 * 不需要另维护 `navItems` 或标签标题映射表。
 */
export const resources: ResourceProps[] = [
  // 概览与待办不归组：它们是每天进来的第一屏，藏在分组里要多点一次。
  {
    name: 'dashboard',
    list: '/',
    meta: { label: '概览', icon: <DashboardOutlined /> },
  },
  {
    // 待办：待跟进、应收待催、缺当日汇率。三类各有明确口径，页面上写清。
    name: 'todos',
    list: '/todos',
    meta: { label: '待办', icon: <BellOutlined /> },
  },

  /*
   * 分组标题是「没有 list 路径的资源」：useMenu 按 meta.parent 把子项挂到
   * 父资源下，ThemedSider 见到 children 就渲染成 SubMenu，选中项的父组自动展开。
   * 所以加模块仍然只是加一条资源再写上 meta.parent —— 不另维护导航映射表。
   * 分组的顺序由它在下面数组里的位置决定，组内顺序同理。
   */
  {
    name: 'master-data',
    meta: { label: '主数据', icon: <DatabaseOutlined /> },
  },
  {
    name: 'products',
    list: '/products',
    create: '/products/create',
    edit: '/products/edit/:id',
    show: '/products/show/:id',
    meta: { label: '产品', icon: <AppstoreOutlined />, parent: 'master-data' },
  },
  {
    name: 'suppliers',
    list: '/suppliers',
    create: '/suppliers/create',
    edit: '/suppliers/edit/:id',
    show: '/suppliers/show/:id',
    meta: { label: '供应商', icon: <ShopOutlined />, parent: 'master-data' },
  },
  {
    name: 'customers',
    list: '/customers',
    create: '/customers/create',
    edit: '/customers/edit/:id',
    show: '/customers/show/:id',
    meta: { label: '客户', icon: <TeamOutlined />, parent: 'master-data' },
  },
  {
    name: 'forwarders',
    list: '/forwarders',
    create: '/forwarders/create',
    edit: '/forwarders/edit/:id',
    show: '/forwarders/show/:id',
    meta: { label: '货代', icon: <GlobalOutlined />, parent: 'master-data' },
  },
  {
    name: 'currencies',
    list: '/currencies',
    create: '/currencies/create',
    edit: '/currencies/edit/:id',
    meta: { label: '币种', icon: <DollarOutlined />, parent: 'master-data' },
  },
  {
    // 汇率没有 edit：历史汇率不可改，所以 resources 里也不声明编辑路径。
    name: 'exchange-rates',
    list: '/exchange-rates',
    create: '/exchange-rates/create',
    meta: { label: '汇率', icon: <SwapOutlined />, parent: 'master-data' },
  },
  {
    // 条款模板库（T-27）：主数据，合同生成时抄成快照。
    name: 'clauses',
    list: '/clauses',
    meta: { label: '条款库', icon: <BookOutlined />, parent: 'master-data' },
  },
  {
    // 仓库（T-56）：主数据维护，建仓与设默认。
    name: 'warehouses',
    list: '/warehouses',
    meta: { label: '仓库', icon: <BankOutlined />, parent: 'master-data' },
  },

  {
    name: 'crm',
    meta: { label: '客户与商机', icon: <SolutionOutlined /> },
  },
  {
    name: 'leads',
    list: '/leads',
    create: '/leads/create',
    edit: '/leads/edit/:id',
    meta: { label: '线索', icon: <UserAddOutlined />, parent: 'crm' },
  },
  {
    name: 'opportunities',
    list: '/opportunities',
    create: '/opportunities/create',
    edit: '/opportunities/edit/:id',
    show: '/opportunities/show/:id',
    meta: { label: '商机', icon: <FundOutlined />, parent: 'crm' },
  },
  {
    name: 'contacts',
    list: '/contacts',
    create: '/contacts/create',
    edit: '/contacts/edit/:id',
    meta: { label: '联系人', icon: <ContactsOutlined />, parent: 'crm' },
  },
  {
    name: 'activities',
    list: '/activities',
    create: '/activities/create',
    edit: '/activities/edit/:id',
    meta: { label: '跟进记录', icon: <CommentOutlined />, parent: 'crm' },
  },

  {
    name: 'sales',
    meta: { label: '销售', icon: <FileDoneOutlined /> },
  },
  {
    // 报价单是 ERP 侧第一张单据：单号、状态机、成本快照都从它开始。
    name: 'quotations',
    list: '/quotations',
    create: '/quotations/create',
    edit: '/quotations/edit/:id',
    show: '/quotations/show/:id',
    meta: { label: '报价单', icon: <FileTextOutlined />, parent: 'sales' },
  },
  {
    // 合同：PI 与销售合同共用一张表。没有空白新建，
    // 只能从已确认的报价单或订单生成，金额与汇率照抄来源。
    name: 'contracts',
    list: '/contracts',
    create: '/contracts/create',
    edit: '/contracts/edit/:id',
    show: '/contracts/show/:id',
    meta: { label: '合同', icon: <FileProtectOutlined />, parent: 'sales' },
  },
  {
    // 订单：报价谈成之后要出货的那张凭证，状态走完整状态机。
    name: 'sales-orders',
    list: '/sales-orders',
    create: '/sales-orders/create',
    edit: '/sales-orders/edit/:id',
    show: '/sales-orders/show/:id',
    meta: { label: '销售订单', icon: <ProfileOutlined />, parent: 'sales' },
  },
  {
    // 销售看板：按客户 / 产品 / 国家 / 月汇总，口径与单票毛利同源。
    name: 'sales-dashboard',
    list: '/sales-dashboard',
    meta: { label: '销售看板', icon: <PieChartOutlined />, parent: 'sales' },
  },

  {
    name: 'sourcing',
    meta: { label: '采购与出货', icon: <ShoppingOutlined /> },
  },
  {
    // 采购订单：销售订单的镜像，明细可挂销售单行做「以销定采」。
    name: 'purchase-orders',
    list: '/purchase-orders',
    create: '/purchase-orders/create',
    edit: '/purchase-orders/edit/:id',
    show: '/purchase-orders/show/:id',
    meta: { label: '采购订单', icon: <ShoppingCartOutlined />, parent: 'sourcing' },
  },
  {
    // 出货批次：一票装哪些销售订单行。与订单是多对多（分批出、拼柜出）。
    name: 'shipments',
    list: '/shipments',
    create: '/shipments/create',
    edit: '/shipments/edit/:id',
    show: '/shipments/show/:id',
    meta: { label: '出货单', icon: <SendOutlined />, parent: 'sourcing' },
  },

  {
    name: 'inventory',
    meta: { label: '库存', icon: <InboxOutlined /> },
  },
  {
    // 库存只有读与重算：能改库存的只有流水，所以这里没有 create / edit。
    name: 'stock-snapshots',
    list: '/stock',
    meta: { label: '当前库存', icon: <DatabaseOutlined />, parent: 'inventory' },
  },
  {
    // 出入库单：草稿不动库存，确认那一刻才生成流水。
    name: 'stock-documents',
    list: '/stock-documents',
    create: '/stock-documents/create',
    edit: '/stock-documents/edit/:id',
    show: '/stock-documents/show/:id',
    meta: { label: '出入库单', icon: <ImportOutlined />, parent: 'inventory' },
  },
  {
    name: 'stock-movements',
    list: '/stock-movements',
    create: '/stock-movements/create',
    meta: { label: '库存流水', icon: <HistoryOutlined />, parent: 'inventory' },
  },

  {
    name: 'finance',
    meta: { label: '财务', icon: <MoneyCollectOutlined /> },
  },
  {
    name: 'receivables',
    list: '/receivables',
    create: '/receivables/create',
    edit: '/receivables/edit/:id',
    show: '/receivables/show/:id',
    meta: { label: '应收', icon: <AccountBookOutlined />, parent: 'finance' },
  },
  {
    name: 'payables',
    list: '/payables',
    create: '/payables/create',
    edit: '/payables/edit/:id',
    show: '/payables/show/:id',
    meta: { label: '应付', icon: <WalletOutlined />, parent: 'finance' },
  },
  {
    name: 'payments',
    list: '/payments',
    create: '/payments/create',
    edit: '/payments/edit/:id',
    show: '/payments/show/:id',
    meta: { label: '收付款', icon: <TransactionOutlined />, parent: 'finance' },
  },
  {
    // 账龄是只读聚合：没有 create / edit，只有一页报表。
    name: 'aging',
    list: '/aging',
    meta: { label: '账龄', icon: <BarChartOutlined />, parent: 'finance' },
  },
  {
    name: 'exchange-gains',
    list: '/exchange-gains',
    meta: { label: '汇兑损益', icon: <LineChartOutlined />, parent: 'finance' },
  },
  {
    // 回款与超期：期间回款率是流量，超期明细是存量，两者不互相校验。
    name: 'collection',
    list: '/collection',
    meta: { label: '回款与超期', icon: <AlertOutlined />, parent: 'finance' },
  },

  {
    name: 'system',
    meta: { label: '系统', icon: <SettingOutlined /> },
  },
  {
    // 账号维护同样只有管理员能用；菜单对所有人可见，权限由接口兜底。
    name: 'user-accounts',
    list: '/user-accounts',
    // 不缓存：系统页没有"填一半再回来"的场景，表却最重。切走即卸载、切回重取。
    meta: { label: '账号', icon: <UserOutlined />, parent: 'system', keepAlive: false },
  },
  {
    // 操作日志只读、且只有管理员能查：里面装着全公司单据的提交内容。
    name: 'audit-logs',
    list: '/audit-logs',
    meta: { label: '操作日志', icon: <FileSearchOutlined />, parent: 'system', keepAlive: false },
  },
]

/**
 * pathname → 标签页标题。
 *
 * 遍历上面这份清单里的六个路径字段（list / create / edit / show / clone / 无），
 * 取匹配模板最长的那个 —— `/products/create` 既命中 `/products`（list），
 * 也命中 `/products/create`（create），后者更长，说明它是更精确的那条。
 *
 * 不读 document.title：那是 DocumentTitleHandler 算出来的，带后缀、且要晚一帧；
 * 标签页需要的是稳定的短标题，与侧栏那一项同名才不割裂。
 */
interface RouteEntry {
  path: string
  label: string
  keepAlive: boolean
  /** 这条路径属于哪个资源（侧栏那一项的中文名），面包屑中间那一跳要它。 */
  resourceLabel: string
  /** 分组名（主数据 / 客户与商机…）；概览与待办没有分组。 */
  groupLabel?: string
  /** 该资源的列表路径：面包屑里"产品"那一跳点它。 */
  listPath?: string
  action: 'list' | 'create' | 'edit' | 'show'
}

const ROUTE_LABELS: RouteEntry[] = (() => {
  // 先把"资源名 → 中文名"收一遍：分组标签挂在父资源上，要靠 parent 反查。
  const labelByName = new Map<string, string>()
  for (const resource of resources) {
    const label = resource.meta?.label
    if (typeof label === 'string') labelByName.set(resource.name, label)
  }

  const entries: RouteEntry[] = []
  for (const resource of resources) {
    const label = resource.meta?.label
    if (typeof label !== 'string') continue
    const parent = resource.meta?.parent
    const groupLabel = typeof parent === 'string' ? labelByName.get(parent) : undefined
    /*
     * 默认缓存，只有显式写 `keepAlive: false` 的页面才在切走后卸载。
     * 默认取"缓存"，是因为绝大多数页面的价值就在"回来时还是刚才那样"：
     * 筛选、分页、半填的表单。要关掉的只是少数（系统页、纯报表）。
     */
    const common = {
      keepAlive: resource.meta?.keepAlive !== false,
      resourceLabel: label,
      groupLabel,
      listPath: resource.list,
    }
    if (resource.list) entries.push({ ...common, path: resource.list, label, action: 'list' })
    if (resource.create) {
      entries.push({ ...common, path: resource.create, label: `新建${label}`, action: 'create' })
    }
    if (resource.edit) {
      entries.push({ ...common, path: resource.edit, label: `编辑${label}`, action: 'edit' })
    }
    if (resource.show) {
      entries.push({ ...common, path: resource.show, label: `${label}详情`, action: 'show' })
    }
  }
  return entries
})()

/** 命中模板最长的那个登记项；都没命中返回 null。 */
function matchRoute(pathname: string): RouteEntry | null {
  let matched: RouteEntry | null = null
  for (const entry of ROUTE_LABELS) {
    if (!pathMatches(entry.path, pathname)) continue
    if (!matched || entry.path.length > matched.path.length) matched = entry
  }
  return matched
}

/**
 * pathname 是否对应一个已登记的页面。
 *
 * 外壳用它挡住手输的错路径：未知路径直接送回首页，而不是先开一个标签页、
 * 再由路由表末尾的 `*` 兜底重定向 —— 那样会在标签条上留下一个点一下就跳走的死标签。
 * 代价是新加页面必须同时在 resources 与 router/routes.tsx 里登记（本来也要）。
 */
export function isKnownPath(pathname: string): boolean {
  return ROUTE_LABELS.some((entry) => pathMatches(entry.path, pathname))
}

/** 把 pathname 翻成标签页上那一行字；匹配不到就退回路径本身。 */
export function describeRoute(pathname: string): string {
  return matchRoute(pathname)?.label ?? pathname
}

/** 面包屑的一跳。`to` 为空表示不可点（当前页、或没有对应页面的分组）。 */
export interface TrailItem {
  label: string
  to?: string
}

/**
 * pathname → 面包屑：分组 → 资源列表 → 当前页。
 *
 * 顶栏里那一条就靠它，内容区不再画第二份。规则与标签标题共用同一份清单：
 *   - 列表页两跳（分组、资源），资源那一跳就是当前页，不给链接；
 *   - 新建 / 编辑 / 详情三跳，中间那跳回列表 —— 这是"从单据退回列表"最常走的一步；
 *   - 概览与待办没有分组，只有一跳。
 */
export function describeTrail(pathname: string): TrailItem[] {
  const matched = matchRoute(pathname)
  if (!matched) return []

  const trail: TrailItem[] = []
  if (matched.groupLabel) trail.push({ label: matched.groupLabel })
  if (matched.action === 'list') {
    trail.push({ label: matched.resourceLabel })
    return trail
  }
  trail.push({ label: matched.resourceLabel, to: matched.listPath })
  trail.push({ label: matched.label })
  return trail
}

/**
 * 这一页切走之后，是否保留它的组件实例。
 *
 * 没登记的路径一律不缓存：认不出来的页面留着只会占内存，也说不清它该是什么状态。
 * 缓存边界与"允许丢什么"写在 docs/ui-shell-refactor.md 4.3。
 */
export function isKeepAlivePath(pathname: string): boolean {
  return matchRoute(pathname)?.keepAlive ?? false
}

/**
 * 轻量的路径模板匹配：只支持 `:param` 段与完全相等，够覆盖 resources 里的写法。
 *
 * 不用 react-router 的 matchPath：这里只需要布尔结果，而它返回的 params
 * 在标签标题里用不上；反过来，标签标题在渲染期执行，拉进 router 的匹配器
 * 会让这个文件从"一份数据"变成"一个路由模块"。
 */
function pathMatches(template: string, pathname: string): boolean {
  const templateSegments = template.split('/').filter(Boolean)
  const pathSegments = pathname.split('/').filter(Boolean)
  if (templateSegments.length !== pathSegments.length) return false
  return templateSegments.every((segment, index) =>
    segment.startsWith(':') ? pathSegments[index].length > 0 : segment === pathSegments[index],
  )
}
