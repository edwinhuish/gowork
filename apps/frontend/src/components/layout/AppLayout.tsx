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
import { ThemedLayout, ThemedSider, useThemedLayoutContext } from '@refinedev/antd'
import { useGetIdentity, useLogout } from '@refinedev/core'
import { useQuery } from '@tanstack/react-query'
import { Avatar, Badge, Button, Dropdown, Grid, Layout, Space, Tooltip } from 'antd'
import type { MenuProps } from 'antd'
import type { FC } from 'react'
import { useEffect } from 'react'
import { Outlet, useLocation, useNavigate } from 'react-router'

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
 * refine 的 ThemedSider 默认把「Logout」当成最后一个菜单项塞进导航里，
 * 和业务菜单混在一起；退出登录已经收在顶栏头像下拉里，导航栏不该再放一遍。
 * render 回调拿到的就是 `{ items, logout, collapsed }`，只取 items 即可。
 */
function AppSider({ Title }: { Title?: SiderTitle }) {
  return <ThemedSider Title={Title} render={({ items }) => items} />
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
