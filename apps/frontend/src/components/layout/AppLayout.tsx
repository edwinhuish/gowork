import {
  BellOutlined,
  CompressOutlined,
  ExpandOutlined,
  LogoutOutlined,
  MenuFoldOutlined,
  MenuUnfoldOutlined,
  MoonOutlined,
  SunOutlined,
  UserOutlined,
} from '@ant-design/icons'
import { ThemedLayout, useThemedLayoutContext } from '@refinedev/antd'
import { useGetIdentity, useLogout, useMenu, type TreeMenuItem } from '@refinedev/core'
import { useQuery } from '@tanstack/react-query'
import {
  Avatar,
  Badge,
  Button,
  Drawer,
  Dropdown,
  Grid,
  Layout,
  Menu,
  Space,
  Tooltip,
  theme,
} from 'antd'
import type { MenuProps } from 'antd'
import type { FC } from 'react'
import { useEffect, useMemo } from 'react'
import { Link, Outlet, useLocation, useNavigate } from 'react-router'

import { reportApi } from '@/api/reports'
import { useUiPrefs } from '@/providers/uiPrefs'
import type { UserInfo } from '@/types'
import { ModuleSearch } from './module-search'

type SiderTitle = FC<{ collapsed: boolean }>

/**
 * 侧栏品牌块。
 *
 * ThemedLayout 会在侧栏收起时把 collapsed 传进来，收起态只留标志图形，
 * 避免 228px 收成 80px 后文字被裁成半截。
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

/**
 * 侧栏。
 *
 * 不用 refine 的 ThemedSider：它用的是 antd 已废弃的 Menu children 写法
 * （<Menu.Item> / <Menu.SubMenu>），每次加载刷两条告警 —— "children is
 * deprecated" 与 React 19 的 "Accessing element.ref"，30 个页面各一次，
 * 把真问题埋在噪音里。菜单数据仍来自 refine 的 useMenu（同一份 resources，
 * 分组 / 图标 / 选中态都在里面），只是换成 items 写法自己渲染。
 *
 * 与 ThemedSider 有意保持一致的两处：退出登录不进导航（顶栏头像下拉里已有）；
 * 窄屏侧栏是抽屉，由顶栏左侧那个按钮打开（见 AppHeader），不再渲染 refine
 * 那个会压住页面标题的浮动开关。
 *
 * 分成两层是因为 refine 判 hasSider 的方式是**把 Sider 当普通函数调用一次**
 * （`!!SiderToRender({ Title })`）——那一刻还没有 context provider，
 * 带 hook 的组件直接被调用会拿不到 context。外层不带 hook，只返回一个元素。
 */
function AppSider({ Title }: { Title?: SiderTitle }) {
  return <AppSiderBody Title={Title} />
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

function AppSiderBody({ Title }: { Title?: SiderTitle }) {
  const { token } = theme.useToken()
  const { siderCollapsed, setSiderCollapsed, mobileSiderOpen, setMobileSiderOpen } =
    useThemedLayoutContext()
  const { menuItems, selectedKey, defaultOpenKeys } = useMenu()
  // 与 refine 同一条判定（lg 之下是抽屉）；首帧拿不到断点时当宽屏处理。
  const isMobile = Grid.useBreakpoint().lg === false
  const RenderToTitle = Title ?? (() => null)
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
        if (isMobile) setMobileSiderOpen(false)
      }}
    />
  )

  // 品牌块：宽 200 / 收起 80，与侧栏同宽，高与顶栏对齐。
  const brand = (collapsed: boolean) => (
    <div
      style={{
        width: collapsed ? '80px' : '200px',
        padding: collapsed ? '0' : '0 16px',
        display: 'flex',
        justifyContent: collapsed ? 'center' : 'flex-start',
        alignItems: 'center',
        height: '64px',
        backgroundColor: token.colorBgElevated,
        fontSize: '14px',
      }}
    >
      <RenderToTitle collapsed={collapsed} />
    </div>
  )

  if (isMobile) {
    return (
      <Drawer
        placement="left"
        width={200}
        open={mobileSiderOpen}
        onClose={() => setMobileSiderOpen(false)}
        closable={false}
        styles={{ body: { padding: 0 } }}
      >
        {brand(false)}
        {menu}
      </Drawer>
    )
  }

  return (
    <Layout.Sider
      collapsible
      collapsed={siderCollapsed}
      // 只认手动点开关：breakpoint 触发的（type === 'responsive'）由侧栏自己管。
      onCollapse={(collapsed, type) => {
        if (type === 'clickTrigger') setSiderCollapsed(collapsed)
      }}
      collapsedWidth={80}
      breakpoint="lg"
      width={200}
      trigger={
        <Button
          type="text"
          aria-label={siderCollapsed ? '展开导航' : '收起导航'}
          icon={siderCollapsed ? <MenuUnfoldOutlined /> : <MenuFoldOutlined />}
          style={{
            borderRadius: 0,
            height: '100%',
            width: '100%',
            backgroundColor: token.colorBgElevated,
          }}
        />
      }
    >
      {brand(siderCollapsed)}
      {menu}
    </Layout.Sider>
  )
}

/**
 * 顶栏。
 *
 * 折叠按钮用 useThemedLayoutContext 控制侧栏：宽屏是收起 / 展开侧栏，
 * 窄屏下侧栏是抽屉（refine 按 lg 断点切换），这里改成打开抽屉 ——
 * setSiderCollapsed 管不到抽屉，原先注释里"这个按钮就是抽屉的开关"是错的，
 * 实机上点它没有任何反应。refine 自带的那个 fixed 抽屉开关与这里是重复入口、
 * 还会压在页面标题上，已在样式里藏掉（见 index.css 的窄屏段）。
 *
 * 四类信息放在这一条上：先找（模块搜索），再看（待办条目数），
 * 然后是观感（明暗）与自己（密度、退出）。全部用 antd 的 text 按钮，
 * 不再手写 `<button>` + 自定义 hover 样式。
 */
function AppHeader() {
  const { siderCollapsed, setSiderCollapsed, setMobileSiderOpen } = useThemedLayoutContext()
  // 与 refine 的 ThemedSider 同一条判定（lg 之下是抽屉）。首帧拿不到断点时
  // 当宽屏处理，和 refine 的 isMobile 一致。
  const narrow = Grid.useBreakpoint().lg === false
  const { data: identity } = useGetIdentity<UserInfo>()
  const { mutate: logout } = useLogout()
  const { mode, toggleMode, compact, toggleCompact } = useUiPrefs()
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
      <Button
        type="text"
        aria-label={narrow ? '打开导航' : siderCollapsed ? '展开导航' : '收起导航'}
        icon={!narrow && !siderCollapsed ? <MenuFoldOutlined /> : <MenuUnfoldOutlined />}
        onClick={() => (narrow ? setMobileSiderOpen(true) : setSiderCollapsed(!siderCollapsed))}
      />

      <ModuleSearch />

      <div className="app-header-spacer" />

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

        <Tooltip title={mode === 'dark' ? '切换到浅色' : '切换到深色'}>
          <Button
            type="text"
            aria-label="切换主题"
            icon={mode === 'dark' ? <SunOutlined /> : <MoonOutlined />}
            onClick={toggleMode}
          />
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
 * 所有受保护页面的外壳：侧栏 + 顶栏 + 内容区，由 refine 的 ThemedLayout 提供。
 *
 * 路由切换时把滚动位置拉回顶部：ThemedLayout 的内容区不是滚动容器，
 * 滚的是 window，不手动复位的话从长列表点进详情会停在页面中间。
 */
export function AppLayout() {
  const { pathname } = useLocation()

  useEffect(() => {
    window.scrollTo({ top: 0 })
  }, [pathname])

  return (
    <ThemedLayout Title={BrandTitle} Header={AppHeader} Sider={AppSider}>
      <Outlet />
    </ThemedLayout>
  )
}
