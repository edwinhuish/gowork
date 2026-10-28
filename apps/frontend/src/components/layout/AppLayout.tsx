import { LogoutOutlined, MenuFoldOutlined, MenuUnfoldOutlined, UserOutlined } from '@ant-design/icons'
import { ThemedLayout, useThemedLayoutContext } from '@refinedev/antd'
import { useGetIdentity, useLogout } from '@refinedev/core'
import { Avatar, Button, Dropdown, Layout } from 'antd'
import { Outlet } from 'react-router'

import type { UserInfo } from '@/types'

/**
 * 侧边栏顶部的品牌块。
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
 * 顶栏。
 *
 * 折叠按钮用 useThemedLayoutContext 控制侧栏 —— 窄屏下 ThemedLayout 会自动把
 * 侧栏变成抽屉，这个按钮就是抽屉的开关，所以必须自己渲染而不是依赖默认头部。
 */
function AppHeader() {
  const { siderCollapsed, setSiderCollapsed } = useThemedLayoutContext()
  const { data: identity } = useGetIdentity<UserInfo>()
  const { mutate: logout } = useLogout()

  return (
    <Layout.Header className="app-header">
      <Button
        type="text"
        aria-label={siderCollapsed ? '展开导航' : '收起导航'}
        icon={siderCollapsed ? <MenuUnfoldOutlined /> : <MenuFoldOutlined />}
        onClick={() => setSiderCollapsed(!siderCollapsed)}
      />

      <div className="app-header-spacer" />

      <Dropdown
        trigger={['click']}
        menu={{
          items: [
            {
              key: 'logout',
              icon: <LogoutOutlined />,
              label: '退出登录',
              onClick: () => logout(),
            },
          ],
        }}
      >
        <button type="button" className="app-user">
          <Avatar size={26} icon={<UserOutlined />} />
          <span>{identity?.nickname || identity?.username || '未登录'}</span>
        </button>
      </Dropdown>
    </Layout.Header>
  )
}

/** 所有受保护页面的外壳：侧栏 + 顶栏 + 内容区，由 refine 的 ThemedLayout 提供。 */
export function AppLayout() {
  return (
    <ThemedLayout Title={BrandTitle} Header={AppHeader}>
      <Outlet />
    </ThemedLayout>
  )
}
