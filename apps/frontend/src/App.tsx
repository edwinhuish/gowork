import {
  AppstoreOutlined,
  CommentOutlined,
  ContactsOutlined,
  DashboardOutlined,
  DollarOutlined,
  ShopOutlined,
  SwapOutlined,
  TeamOutlined,
} from '@ant-design/icons'
import { Authenticated, Refine } from '@refinedev/core'
import routerProvider, {
  CatchAllNavigate,
  DocumentTitleHandler,
  NavigateToResource,
  UnsavedChangesNotifier,
} from '@refinedev/react-router'
import { App as AntApp, ConfigProvider } from 'antd'
import zhCN from 'antd/locale/zh_CN'
import { BrowserRouter, Navigate, Outlet, Route, Routes } from 'react-router'

import { AppLayout } from '@/components/layout/AppLayout'
import { ActivityCreate, ActivityEdit, ActivityList } from '@/pages/activities'
import { ContactCreate, ContactEdit, ContactList } from '@/pages/contacts'
import { CurrencyCreate, CurrencyEdit, CurrencyList } from '@/pages/currencies'
import { CustomerCreate, CustomerEdit, CustomerList, CustomerShow } from '@/pages/customers'
import { DashboardPage } from '@/pages/dashboard'
import { ExchangeRateCreate, ExchangeRateList } from '@/pages/exchange-rates'
import { LoginPage } from '@/pages/login'
import { ProductCreate, ProductEdit, ProductList, ProductShow } from '@/pages/products'
import { SupplierCreate, SupplierEdit, SupplierList, SupplierShow } from '@/pages/suppliers'
import { authProvider } from '@/providers/authProvider'
import { dataProvider } from '@/providers/dataProvider'
import { themeConfig } from '@/theme'

/**
 * 应用根。
 *
 * 三段式路由，是 refine 的标准鉴权布局：
 *   1 受保护段：Authenticated 未登录时 fallback 到 /login，登录后渲染 AppLayout
 *   2 登录段：已登录访问 /login 时 NavigateToResource 把人送回首页
 *   3 兜底：未知路径回首页
 *
 * resources 是导航与面包屑的唯一来源 —— 加一个模块只需要在这里加一条，
 * 侧边栏菜单、页面标题、返回链接都会跟着变。
 */
export default function App() {
  return (
    <BrowserRouter>
      <ConfigProvider locale={zhCN} theme={themeConfig}>
        <AntApp>
          <Refine
            dataProvider={dataProvider}
            authProvider={authProvider}
            routerProvider={routerProvider}
            resources={[
              {
                name: 'dashboard',
                list: '/',
                meta: { label: '概览', icon: <DashboardOutlined /> },
              },
              {
                name: 'products',
                list: '/products',
                create: '/products/create',
                edit: '/products/edit/:id',
                show: '/products/show/:id',
                meta: { label: '产品', icon: <AppstoreOutlined /> },
              },
              {
                name: 'suppliers',
                list: '/suppliers',
                create: '/suppliers/create',
                edit: '/suppliers/edit/:id',
                show: '/suppliers/show/:id',
                meta: { label: '供应商', icon: <ShopOutlined /> },
              },
              {
                name: 'customers',
                list: '/customers',
                create: '/customers/create',
                edit: '/customers/edit/:id',
                show: '/customers/show/:id',
                meta: { label: '客户', icon: <TeamOutlined /> },
              },
              {
                name: 'contacts',
                list: '/contacts',
                create: '/contacts/create',
                edit: '/contacts/edit/:id',
                meta: { label: '联系人', icon: <ContactsOutlined /> },
              },
              {
                name: 'activities',
                list: '/activities',
                create: '/activities/create',
                edit: '/activities/edit/:id',
                meta: { label: '跟进记录', icon: <CommentOutlined /> },
              },
              {
                name: 'currencies',
                list: '/currencies',
                create: '/currencies/create',
                edit: '/currencies/edit/:id',
                meta: { label: '币种', icon: <DollarOutlined /> },
              },
              {
                // 汇率没有 edit：历史汇率不可改，所以 resources 里也不声明编辑路径。
                name: 'exchange-rates',
                list: '/exchange-rates',
                create: '/exchange-rates/create',
                meta: { label: '汇率', icon: <SwapOutlined /> },
              },
            ]}
            options={{
              syncWithLocation: true,
              warnWhenUnsavedChanges: true,
              title: { text: 'GoWork ERP · CRM' },
            }}
          >
            <Routes>
              <Route
                element={
                  <Authenticated key="protected-routes" fallback={<CatchAllNavigate to="/login" />}>
                    <Outlet />
                  </Authenticated>
                }
              >
                <Route element={<AppLayout />}>
                  <Route index element={<DashboardPage />} />

                  <Route path="products">
                    <Route index element={<ProductList />} />
                    <Route path="create" element={<ProductCreate />} />
                    <Route path="edit/:id" element={<ProductEdit />} />
                    <Route path="show/:id" element={<ProductShow />} />
                  </Route>

                  <Route path="suppliers">
                    <Route index element={<SupplierList />} />
                    <Route path="create" element={<SupplierCreate />} />
                    <Route path="edit/:id" element={<SupplierEdit />} />
                    <Route path="show/:id" element={<SupplierShow />} />
                  </Route>

                  <Route path="customers">
                    <Route index element={<CustomerList />} />
                    <Route path="create" element={<CustomerCreate />} />
                    <Route path="edit/:id" element={<CustomerEdit />} />
                    <Route path="show/:id" element={<CustomerShow />} />
                  </Route>

                  <Route path="contacts">
                    <Route index element={<ContactList />} />
                    <Route path="create" element={<ContactCreate />} />
                    <Route path="edit/:id" element={<ContactEdit />} />
                  </Route>

                  <Route path="activities">
                    <Route index element={<ActivityList />} />
                    <Route path="create" element={<ActivityCreate />} />
                    <Route path="edit/:id" element={<ActivityEdit />} />
                  </Route>

                  <Route path="currencies">
                    <Route index element={<CurrencyList />} />
                    <Route path="create" element={<CurrencyCreate />} />
                    <Route path="edit/:id" element={<CurrencyEdit />} />
                  </Route>

                  <Route path="exchange-rates">
                    <Route index element={<ExchangeRateList />} />
                    <Route path="create" element={<ExchangeRateCreate />} />
                  </Route>
                </Route>
              </Route>

              <Route
                element={
                  <Authenticated key="auth-routes" fallback={<Outlet />}>
                    <NavigateToResource resource="dashboard" />
                  </Authenticated>
                }
              >
                <Route path="login" element={<LoginPage />} />
              </Route>

              <Route path="*" element={<Navigate to="/" replace />} />
            </Routes>

            <UnsavedChangesNotifier />
            <DocumentTitleHandler />
          </Refine>
        </AntApp>
      </ConfigProvider>
    </BrowserRouter>
  )
}
