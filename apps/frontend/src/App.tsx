import type { ReactElement } from 'react'
import { BrowserRouter, Navigate, Route, Routes, useLocation } from 'react-router-dom'

import { Layout } from '@/components/Layout'
import { Customers } from '@/pages/Customers'
import { Dashboard } from '@/pages/Dashboard'
import { Login } from '@/pages/Login'
import { Products } from '@/pages/Products'
import { Suppliers } from '@/pages/Suppliers'
import { AuthProvider, useAuth } from '@/store/auth'

function RequireAuth({ children }: { children: ReactElement }) {
  const { user, loading } = useAuth()
  const location = useLocation()

  if (loading) {
    return <div className="page-loading">正在校验登录状态…</div>
  }
  if (!user) {
    // 带上来源地址，登录后能回到被打断的那一页，配合表单草稿接上之前的输入。
    return <Navigate to="/login" state={{ from: location }} replace />
  }
  return children
}

export default function App() {
  return (
    <AuthProvider>
      <BrowserRouter>
        <Routes>
          <Route path="/login" element={<Login />} />

          <Route
            element={
              <RequireAuth>
                <Layout />
              </RequireAuth>
            }
          >
            <Route path="/" element={<Dashboard />} />
            <Route path="/products" element={<Products />} />
            <Route path="/suppliers" element={<Suppliers />} />
            <Route path="/customers" element={<Customers />} />
          </Route>

          <Route path="*" element={<Navigate to="/" replace />} />
        </Routes>
      </BrowserRouter>
    </AuthProvider>
  )
}
