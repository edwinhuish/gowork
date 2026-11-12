import { Authenticated, Refine } from '@refinedev/core'
import routerProvider, {
  CatchAllNavigate,
  DocumentTitleHandler,
  NavigateToResource,
} from '@refinedev/react-router'
import { App as AntApp, ConfigProvider } from 'antd'
import zhCN from 'antd/locale/zh_CN'
import { useMemo } from 'react'
import { BrowserRouter, Outlet, Route, Routes } from 'react-router'

import { ApiError } from '@/api/client'
import { AppLayout } from '@/components/layout/AppLayout'
import { TopProgress } from '@/components/layout/top-progress'
import { resources } from '@/config/resources'
import { LoginPage } from '@/pages/login'
import { authProvider } from '@/providers/authProvider'
import { dataProvider } from '@/providers/dataProvider'
import { i18nProvider } from '@/providers/i18nProvider'
import { useUiPrefs } from '@/providers/uiPrefs'
import { buildThemeConfig } from '@/theme'

/**
 * 应用根。
 *
 * 三段式路由，是 refine 的标准鉴权布局：
 *   1 受保护段：Authenticated 未登录时 fallback 到 /login，登录后渲染 AppLayout
 *   2 登录段：已登录访问 /login 时 NavigateToResource 把人送回首页
 *   3 兜底：受保护段里的 `*` 全交给 AppLayout，由它内部的标签页路由表决定渲染哪一页
 *
 * 为什么第 3 段不再把每条页面路由平铺在这里：
 * 多标签页要求同一个页面能**同时存在多份实例**（各带自己的 :id 与 query），
 * 而 `<Routes>` 一个地址只匹配一棵树。页面路由因此下沉到 `router/routes.tsx`
 * 这份数据里，由每个标签页各自 `useRoutes(..., tab.location)` 匹配一次 ——
 * 见 components/layout/tab-panes.tsx。
 *
 * resources 同样抽到了 `config/resources.tsx`：它现在是导航与标签标题的共同来源。
 */
export default function App() {
  // 亮暗与密度只影响观感，改一次就整棵组件树重算一次主题，代价可以忽略。
  const { mode, compact } = useUiPrefs()
  const themeConfig = useMemo(() => buildThemeConfig({ mode, compact }), [mode, compact])

  return (
    <BrowserRouter>
      <ConfigProvider locale={zhCN} theme={themeConfig}>
        <AntApp>
          {/*
           * 顶部加载进度条挂在这里而不是外壳里：登录页、鉴权校验、各页面请求
           * 都从 api/client.ts 出去，只有挂在最外层才全都能反映出来。
           */}
          <TopProgress />
          <Refine
            dataProvider={dataProvider}
            authProvider={authProvider}
            i18nProvider={i18nProvider}
            routerProvider={routerProvider}
            resources={resources}
            options={{
              syncWithLocation: true,
              warnWhenUnsavedChanges: true,
              title: { text: 'GoWork ERP · CRM' },
              reactQuery: {
                clientConfig: {
                  defaultOptions: {
                    queries: {
                      /*
                       * 4xx 不重试。
                       *
                       * react-query 默认对所有失败重试 3 次，可请求本身就是错的
                       * （日期格式不对、越权、单据已作废），再试一万次还是同一个
                       * 结果。更要紧的是重试退避的那十来秒里界面**什么都不显示**：
                       * refine 的 QueryClient 默认 placeholderData 是
                       * keepPreviousData，旧数据还摆在原处，而 isFetching 在退避
                       * 间隙是 false，连 loading 都没有 —— 看着就像成功了。
                       * 实测账龄页输个 "abc"，要等 9 秒才浮出「日期格式应为
                       * YYYY-MM-DD」。5xx 与网络错误才值得重试。
                       *
                       * 只覆盖 retry：refine 把传进去的 queries 合并在自己的默认
                       * （refetchOnWindowFocus / keepPreviousData）之后，不会顶掉。
                       */
                      retry: (count, error) => {
                        if (error instanceof ApiError) {
                          // 业务码是「HTTP 状态 × 100 + 序号」（40001 → 400），
                          // 取不到报文时记的是裸状态码，网络层失败记 -1。
                          const status = error.code > 1000 ? Math.floor(error.code / 100) : error.code
                          if (status >= 400 && status < 500) return false
                        }
                        return count < 3
                      },
                    },
                  },
                },
              },
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
                {/* 受保护段不分辨具体路径：进到外壳之后由标签页路由表匹配。 */}
                <Route path="*" element={<AppLayout />} />
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
            </Routes>

            {/*
             * 未保存提醒在外壳里（UnsavedTabGuard）：它要知道"这次跳转的目标
             * 是不是已打开的标签页"，才能只对真正的离开弹确认。
             */}
            <DocumentTitleHandler />
          </Refine>
        </AntApp>
      </ConfigProvider>
    </BrowserRouter>
  )
}
