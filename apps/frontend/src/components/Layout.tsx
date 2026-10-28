import { NavLink, Outlet, useLocation, useNavigate } from 'react-router-dom'

import { useAuth } from '../store/auth'

const navItems = [
  { to: '/', label: '概览', end: true },
  { to: '/products', label: '产品' },
  { to: '/suppliers', label: '供应商' },
  { to: '/customers', label: '客户' },
]

const titleMap: Record<string, string> = {
  '/': '概览',
  '/products': '产品管理',
  '/suppliers': '供应商管理',
  '/customers': '客户管理',
}

export function Layout() {
  const { user, logout } = useAuth()
  const navigate = useNavigate()
  const location = useLocation()

  const handleLogout = async () => {
    await logout()
    navigate('/login', { replace: true })
  }

  return (
    <div className="app-shell">
      <aside className="sidebar">
        <div className="brand">
          <span className="brand-mark">G</span>
          <span>GoWork ERP</span>
        </div>

        <div className="nav-group-label">业务</div>
        {navItems.map((item) => (
          <NavLink
            key={item.to}
            to={item.to}
            end={item.end}
            className={({ isActive }) => (isActive ? 'nav-link active' : 'nav-link')}
          >
            {item.label}
          </NavLink>
        ))}
      </aside>

      <div className="main">
        <header className="topbar">
          <span className="topbar-title">{titleMap[location.pathname] ?? 'GoWork ERP'}</span>
          <div className="topbar-user">
            <span>{user?.nickname || user?.username || '未登录'}</span>
            <button type="button" className="btn btn-sm" onClick={handleLogout}>
              退出
            </button>
          </div>
        </header>

        <main className="content">
          <Outlet />
        </main>
      </div>
    </div>
  )
}
