import { Tooltip } from '@/components/ui/Tooltip'
import {
  BgColorsOutlined,
  BellOutlined,
  CheckOutlined,
  CompressOutlined,
  ExpandOutlined,
  LogoutOutlined,
  MenuFoldOutlined,
  MenuUnfoldOutlined,
  MoonOutlined,
  UserOutlined,
} from '@ant-design/icons'
import { useGetIdentity, useLogout, useMenu, type TreeMenuItem } from '@refinedev/core'
import { useQuery } from '@tanstack/react-query'
import { Avatar,
  Badge,
  Breadcrumb,
  Button,
  ConfigProvider,
  Drawer,
  Dropdown,
  Grid,
  Layout,
  Menu,
  Space, } from 'antd'
import type { MenuProps } from 'antd'
import type { CSSProperties, ReactNode } from 'react'
import { useEffect, useMemo, useState } from 'react'
import { Link, Navigate, useLocation, useNavigate } from 'react-router'

import { reportApi } from '@/api/reports'
import { describeTrail, isKnownPath } from '@/config/resources'
import { TabsProvider } from '@/providers/tabs'
import { useUiPrefs } from '@/providers/uiPrefs'
import {
  buildSiderThemeConfig,
  resolveSiderMode,
  shell,
  shellCssVars,
  siderCssVars,
  type SiderMode,
} from '@/theme'
import type { UserInfo } from '@/types'
import { AppTabs } from './app-tabs'
import { TabPanes } from './tab-panes'
import { UnsavedTabGuard } from './unsaved-tab-guard'

/*
 * 侧栏两档宽度与抽屉宽度不在这里定义：它们属于外壳几何，
 * 统一放在 theme.ts 的 `shell`（与顶栏高度、内容内距同一处）。
 */

const SIDER_MODE_LABELS: Record<SiderMode, string> = {
  follow: '跟随页面',
  light: '侧栏浅色',
  dark: '侧栏深色',
}

/**
 * 侧栏品牌块。
 *
 * 收起态只留标志图形，避免 200px 收成 80px 后文字被裁成半截。
 */
function BrandTitle({ collapsed }: { collapsed: boolean }) {
  return (
    <div className="brand">
      <span className="brand-mark">G</span>
      {!collapsed && (
        <span className="brand-text">
          GoWork
          <em className="brand-sub">ERP · CRM</em>
        </span>
      )}
    </div>
  )
}

/** refine 的菜单树 → antd Menu 的 items（叶子节点才给链接）。 */
function toMenuItems(nodes: TreeMenuItem[]): NonNullable<MenuProps['items']> {
  return nodes.map((node) => {
    const label = node.label ?? node.meta?.label ?? node.name
    const icon = node.icon ?? node.meta?.icon
    // 分组（resources 里带 meta.parent 的那些）：只有标题，点了不跳转。
    if (node.children.length > 0) {
      return { key: node.key, icon, label, children: toMenuItems(node.children) }
    }
    const route = node.route ?? node.list
    // 没有列表页的资源不给链接：点不动比跳到首页强。
    return { key: node.key, icon, label: route ? <Link to={route}>{label}</Link> : label }
  })
}

/**
 * 菜单里的一行"开关项"。
 *
 * 勾选位固定占一格宽度：三行侧栏配色里只有当前项显示勾，
 * 若按有无勾来排版，文字会一行行左右错开。
 */
function MenuChoice({ checked, children }: { checked: boolean; children: ReactNode }) {
  return (
    <span className="app-menu-choice">
      <CheckOutlined className={checked ? 'app-menu-check is-on' : 'app-menu-check'} />
      <span>{children}</span>
    </span>
  )
}

interface AppSiderProps {
  collapsed: boolean
  /** 整体隐藏：顶栏左侧按钮要求"完全收起"时为真，这时侧栏不渲染、主区域占满整行。 */
  hidden: boolean
  mobile: boolean
  mobileOpen: boolean
  onToggleCollapse: () => void
  onMobileClose: () => void
  /** 侧栏自己的 CSS 变量：与页面主题无关的一套配色（见 theme.ts）。 */
  vars: CSSProperties
}

/**
 * 侧栏。
 *
 * 不用 refine 的 ThemedSider：它要的三段结构（品牌块 / 可滚菜单 / 底部开关）
 * 与自带主题都只能靠覆写它的内联样式得到；它用的还是 antd 已废弃的 Menu children
 * 写法（<Menu.Item> / <Menu.SubMenu>），每次加载刷告警，把真问题埋在噪音里。
 * 菜单数据仍来自 refine 的 useMenu（同一份 resources，分组 / 图标 / 选中态都在里面），
 * 只是换成 items 写法自己渲染。
 *
 * 结构是**三段**，而不是一整块跟着页面滚：
 *   品牌块   定高，与顶栏同高，不参与滚动
 *   菜单区   占满剩余高度，自己滚
 *   折叠开关 定高，钉在侧栏底部
 * 所以菜单再长，品牌块和开关都不会被滚走。
 *
 * 窄屏下整块换成抽屉：抽屉不带折叠开关（没有"收起"这回事），
 * 点菜单项即关闭（见 onClick）。
 */
function AppSider({
  collapsed,
  hidden,
  mobile,
  mobileOpen,
  onToggleCollapse,
  onMobileClose,
  vars,
}: AppSiderProps) {
  const { menuItems, selectedKey, defaultOpenKeys } = useMenu()
  const items = useMemo(() => toMenuItems(menuItems), [menuItems])

  // 用 Link 而不是 onClick 导航：中键 / 右键「在新标签打开」是常态操作，
  // 走 onClick 就只有左键能用。窄屏点了顺手关掉抽屉。
  const menu = (
    <Menu
      mode="inline"
      items={items}
      selectedKeys={selectedKey ? [selectedKey] : []}
      defaultOpenKeys={defaultOpenKeys}
      style={{ borderInlineEnd: 'none' }}
      onClick={() => {
        if (mobile) onMobileClose()
      }}
    />
  )

  if (mobile) {
    /*
     * 抽屉是 portal 渲染到 body 的：它不在 .app-shell 的子树里，
     * 拿不到挂在树根上的 --shell-*。必须在这里再给一次 ——
     * 少了这一份，品牌块与折叠钮会退回 CSS 里的兜底值，
     * 改 token 时移动端会静默停在旧尺寸上（外观没变、只是对不上）。
     */
    const drawerBody: CSSProperties = {
      padding: 0,
      display: 'flex',
      flexDirection: 'column',
      // 抽屉自己不许滚：滚的只能是菜单区，否则品牌块会跟着滑走。
      overflow: 'hidden',
      ...shellCssVars,
      ...vars,
    }

    return (
      <Drawer
        placement="left"
        width={shell.mobileSiderWidth}
        open={mobileOpen}
        onClose={onMobileClose}
        closable={false}
        styles={{ body: drawerBody }}
        classNames={{ body: 'app-sider-drawer-body' }}
      >
        <div className="app-sider-brand">
          <BrandTitle collapsed={false} />
        </div>
        <div className="app-sider-menu">{menu}</div>
      </Drawer>
    )
  }

  // 顶栏左侧的按钮要求"完全隐藏"侧栏：这里直接不渲染，主区域自动占满整行。
  if (hidden) return null

  return (
    <Layout.Sider
      className="app-sider"
      collapsed={collapsed}
      collapsedWidth={shell.siderCollapsedWidth}
      width={shell.siderWidth}
      /*
       * 不给 collapsible / trigger：antd 自带的触发器是 position: fixed，
       * 在这套"外壳一屏高、只有内容区滚"的布局里会钉在视口底部、
       * 而不是侧栏底部。下面自己画一个。
       */
      style={vars}
    >
      <div className="app-sider-brand">
        <BrandTitle collapsed={collapsed} />
      </div>
      <div className="app-sider-menu">{menu}</div>
      <button
        type="button"
        className="app-sider-trigger"
        aria-label={collapsed ? '展开导航' : '收起导航'}
        onClick={onToggleCollapse}
      >
        {collapsed ? <MenuUnfoldOutlined /> : <MenuFoldOutlined />}
      </button>
    </Layout.Sider>
  )
}

interface AppHeaderProps {
  mobile: boolean
  /** 窄屏没有常驻侧栏，只有一个抽屉：这里留下的按钮负责把它拉出来。 */
  onOpenNav: () => void
  /** 侧栏是否被整体隐藏（顶栏左侧按钮控制的那个状态）。 */
  siderHidden: boolean
  /** 切换侧栏的整体隐藏 / 展开。 */
  onToggleSider: () => void
}

/**
 * 顶栏左侧的面包屑：分组 → 资源列表 → 当前页。
 *
 * 内容区那份由 CSS 收起来（refine 的 List/Show 自带），全站只留这一条 ——
 * 两份同样的导航并排出现，会让人以为其中一份是"另一层"。
 */
function AppBreadcrumb() {
  const { pathname } = useLocation()
  const trail = describeTrail(pathname)
  if (trail.length === 0) return null

  return (
    <Breadcrumb
      className="app-breadcrumb"
      items={trail.map((item, index) => ({
        // 最后一跳是当前页，不给链接；中间那跳回列表。
        title:
          item.to && index < trail.length - 1 ? <Link to={item.to}>{item.label}</Link> : item.label,
      }))}
    />
  )
}

/**
 * 顶栏。
 *
 * 它在 .app-main 的顶部、不在任何滚动容器里 —— 内容区怎么滚它都不动。
 *
 * 左侧是面包屑：告诉你"现在在哪一页、上一层是什么"。标签条已经搬到它下面的
 * 独立一条（.app-tabbar-row），两者各管一件事 —— 顶栏回答"当前在哪"，
 * 标签条回答"还开着什么"。挤在一行里时，标签每多一个就要从面包屑那里让位，
 * 最后两样都看不全。
 *
 * 左侧（面包屑之前）在宽屏放一个"隐藏 / 展开侧栏"按钮：完全收起侧栏，
 * 让内容区占满整行。它与侧栏底部的开关是两回事——底部那个只是"收成 60px"，
 * 这里才是整个藏起来（见 AppSider 的 hidden 分支）。窄屏没有常驻侧栏（只有抽屉），
 * 那个按钮换成"打开导航"（窄屏也不画面包屑：横排空间先给标题）。
 *
 * 右半边只剩两类东西：待办（每天第一眼要看的数）与自己。
 * 观感类的三个开关（明暗、侧栏配色、密度）收进头像菜单：它们都是"设置一次就放着"的
 * 低频操作，占着顶栏会把待办挤到视线外。菜单项仍走各自的 toggle / set，
 * 状态仍由 uiPrefs 持有，逻辑一点没变 —— 只是入口换了地方。
 */
function AppHeader({ mobile, onOpenNav, siderHidden, onToggleSider }: AppHeaderProps) {
  const { data: identity } = useGetIdentity<UserInfo>()
  const { mutate: logout } = useLogout()
  const { mode, toggleMode, compact, toggleCompact, siderMode, setSiderMode } = useUiPrefs()
  const navigate = useNavigate()

  // 与概览页用同一个 query key：两处共用一次请求，徽标不会比页面慢一拍。
  const { data: todos } = useQuery({
    queryKey: ['report-todos', 5],
    queryFn: () => reportApi.todos({ limit: 5 }),
    staleTime: 60_000,
  })

  const pending =
    (todos?.followup_count ?? 0) +
    (todos?.receivable_count ?? 0) +
    (todos?.inspection_count ?? 0) +
    (todos?.contract_term_count ?? 0) +
    (todos?.missing_rates.length ?? 0)

  const userMenu: MenuProps = {
    items: [
      {
        key: 'mode',
        icon: <MoonOutlined />,
        label: <MenuChoice checked={mode === 'dark'}>深色模式</MenuChoice>,
        onClick: () => toggleMode(),
      },
      {
        // 侧栏配色是整页明暗之外的独立一档：它只影响侧栏，允许一深一浅。
        key: 'sider-mode',
        icon: <BgColorsOutlined />,
        label: '侧栏配色',
        children: (['follow', 'light', 'dark'] as SiderMode[]).map((key) => ({
          key: `sider-${key}`,
          label: <MenuChoice checked={siderMode === key}>{SIDER_MODE_LABELS[key]}</MenuChoice>,
          onClick: () => setSiderMode(key),
        })),
      },
      { type: 'divider' },
      {
        key: 'density',
        icon: compact ? <ExpandOutlined /> : <CompressOutlined />,
        label: compact ? '切换为标准密度' : '切换为紧凑密度',
        onClick: () => toggleCompact(),
      },
      { type: 'divider' },
      {
        key: 'logout',
        icon: <LogoutOutlined />,
        label: '退出登录',
        onClick: () => logout(),
      },
    ],
  }

  return (
    <Layout.Header className="app-header">
      {/*
       * 宽屏：面包屑之前的按钮负责把侧栏整体收起 / 展开（完全藏起来，
       * 让内容区占满整行）。它和侧栏底部的开关是两回事——底部那个只是"收成 60px"，
       * 这里是整个不渲染。窄屏没有常驻侧栏（只有抽屉），那个按钮换成"打开导航"。
       */}
      {mobile ? (
        <Button type="text" aria-label="打开导航" icon={<MenuUnfoldOutlined />} onClick={onOpenNav} />
      ) : (
        <Button
          type="text"
          aria-label={siderHidden ? '展开侧边栏' : '隐藏侧边栏'}
          icon={siderHidden ? <MenuUnfoldOutlined /> : <MenuFoldOutlined />}
          onClick={onToggleSider}
        />
      )}

      {/* 面包屑在顶栏左侧；中间留一个弹性占位，把待办与头像推到右边。 */}
      <AppBreadcrumb />
      <div className="app-header-grow" />

      <Space size={4} align="center">
        <Tooltip title={pending > 0 ? `${pending} 件到期的事` : '没有到期的事'}>
          <Badge count={pending} size="small" offset={[2, -2]}>
            <Button
              type="text"
              aria-label="待办"
              icon={<BellOutlined />}
              onClick={() => navigate('/todos')}
            />
          </Badge>
        </Tooltip>

        <Dropdown trigger={['click']} menu={userMenu}>
          <Button type="text" className="app-user">
            <Avatar size={26} icon={<UserOutlined />} />
            <span>{identity?.nickname || identity?.username || '未登录'}</span>
          </Button>
        </Dropdown>
      </Space>
    </Layout.Header>
  )
}

/**
 * 所有受保护页面的外壳：固定顶栏 + 标签条 + 独立滚动的侧栏 + 内容区。
 *
 * 布局不交给 refine 的 ThemedLayout：它是"整页滚动"的形状（外层 minHeight 100vh、
 * 内容区不给高度），要做出"顶栏不动、侧栏与内容各滚各的"只能层层覆写它的内联样式。
 * 这里直接给出结构，四个滚动边界的责任就一目了然：
 *
 *   .app-shell      正好一屏高，overflow: hidden —— 窗口永远不滚
 *   .app-sider      左侧一列，菜单区自己滚（见 AppSider）
 *   .app-header     在 .app-main 顶部，不在任何滚动容器里：面包屑 + 待办 + 用户
 *   .app-tabbar-row 顶栏下面那一条，也不滚；标签多了自己在条内横向滚
 *   .app-tab-pane   每个标签页一份，各自滚 —— 见 TabPanes
 *
 * 侧栏外面套了第二个 ConfigProvider（buildSiderThemeConfig）：侧栏按自己的
 * 偏好取配色，与内容区的主题互不影响——内容区换明暗不会重置侧栏的选择。
 */
export function AppLayout() {
  const { pathname } = useLocation()

  /*
   * 手输的错路径先送回首页。
   *
   * 不这么做的话，它会先变成一个标签页，再由路由表末尾的 `*` 兜底重定向 ——
   * 标签条上于是留下一个点一下就跳走的死标签。
   * 路由表与 resources 本来就要一起登记（见 config/resources.tsx）。
   */
  if (!isKnownPath(pathname)) return <Navigate to="/" replace />

  // 标签页状态只在这里有用：菜单、标签条、内容区都在它下面。
  return (
    <TabsProvider>
      <AppShell />
    </TabsProvider>
  )
}

function AppShell() {
  const { mode, compact, siderMode } = useUiPrefs()
  // 与 refine 同一条判定（lg 之下侧栏是抽屉）；首帧拿不到断点时按宽屏处理。
  const mobile = Grid.useBreakpoint().lg === false

  const [collapsed, setCollapsed] = useState(false)
  const [mobileOpen, setMobileOpen] = useState(false)
  // 侧栏整体隐藏：顶栏左侧按钮控制，与底部"收成 60px"的 collapsed 互不干扰。
  const [siderHidden, setSiderHidden] = useState(false)
  // 全屏模式：标签栏右侧按钮控制，隐藏侧栏与顶栏、只留标签条与内容（见 index.css 的 .is-fullscreen）。
  const [fullscreen, setFullscreen] = useState(false)

  const siderColorMode = resolveSiderMode(siderMode, mode)
  const siderTheme = useMemo(
    () => buildSiderThemeConfig({ mode: siderColorMode, compact }),
    [siderColorMode, compact],
  )
  const siderVars = useMemo(() => siderCssVars(siderColorMode), [siderColorMode])

  // 抽屉只在窄屏有意义：拉宽窗口后不能让它继续盖着页面。
  useEffect(() => {
    if (!mobile) setMobileOpen(false)
  }, [mobile])

  /*
   * 外壳几何以 CSS 变量下发：index.css 的结构性样式引用它们，不抄数字。
   * 顶栏高度、内容内距、品牌块与标签胶囊的度量都在这一批里（见 theme.ts 的 shell）。
   */
  return (
    <div className={`app-shell${fullscreen ? ' is-fullscreen' : ''}`} style={shellCssVars}>
      {/* 未保存提醒要知道已打开的标签页，所以挂在外壳里而不是 App 根上。 */}
      <UnsavedTabGuard />

      <ConfigProvider theme={siderTheme}>
        <AppSider
          collapsed={collapsed}
          hidden={siderHidden}
          mobile={mobile}
          mobileOpen={mobileOpen}
          onToggleCollapse={() => setCollapsed((prev) => !prev)}
          onMobileClose={() => setMobileOpen(false)}
          vars={siderVars}
        />
      </ConfigProvider>

      <div className="app-main">
        <AppHeader
          mobile={mobile}
          onOpenNav={() => setMobileOpen(true)}
          siderHidden={siderHidden}
          onToggleSider={() => setSiderHidden((prev) => !prev)}
        />
        {/* 标签条独立一条：顶栏放面包屑与全局操作，它放"开着哪些页"。 */}
        <AppTabs
          fullscreen={fullscreen}
          onToggleFullscreen={() => setFullscreen((prev) => !prev)}
        />
        <main className="app-content">
          <TabPanes />
        </main>
      </div>
    </div>
  )
}
