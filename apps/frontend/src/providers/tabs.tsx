import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState } from 'react'
import type { ReactNode } from 'react'
import { useLocation, useNavigate } from 'react-router'
import type { Location } from 'react-router'

import { describeRoute } from '@/config/resources'

/**
 * 多标签页：已打开页面的清单 + 当前激活的那个。
 *
 * 一条状态，两个驱动源：
 *   - 地址栏（点侧栏、点链接、前进后退、手改 URL）变化 → 这里打开 / 激活对应标签页；
 *   - 点标签页 → 把地址栏导航过去，再由上一条把它标成激活。
 * 只让地址栏单向驱动，就不会出现「标签页说 A、地址栏是 B」的两套真相。
 *
 * 标签页的 key 用 **pathname**：
 *   /products?page=2 与 /products?page=3 是同一页翻页，算一个标签页；
 *   /products/show/1 与 /products/show/2 是两张单据，必须各占一个。
 * 这正是"query 属于页面内部状态、pathname 属于页面身份"的划分。
 *
 * 状态保留靠的是"不卸载"：每个标签页的内容由 AppLayout 各渲染一份、
 * 用 hidden 切换显隐。所以筛选、表单、展开折叠、已加载数据都还在原来的组件里，
 * 这里只需要额外记住滚动位置（滚动容器会随 display: none 一起丢掉 scrollTop）。
 */

export interface TabItem {
  /** pathname，见上面的划分。 */
  key: string
  /** 完整地址（含 query 与 hash），切回这个标签页时导航到这里。 */
  url: string
  label: string
  /** 这个标签页自己的 location：内容区按它匹配路由、读 query。 */
  location: Location
}

interface TabsValue {
  tabs: TabItem[]
  activeKey: string
  activate: (key: string) => void
  close: (key: string) => void
  rememberScroll: (key: string, top: number) => void
  restoreScroll: (key: string) => number
}

/** 首页固定：标签页全关掉时还要有个落点。 */
export const HOME_TAB_KEY = '/'

const TabsContext = createContext<TabsValue | null>(null)

function toTab(location: Pick<Location, 'pathname' | 'search' | 'hash'>): TabItem {
  return {
    key: location.pathname,
    url: `${location.pathname}${location.search}${location.hash}`,
    label: describeRoute(location.pathname),
    location: {
      pathname: location.pathname,
      search: location.search,
      hash: location.hash,
      state: null,
      // key 用固定值而不是路由给的随机 key：它只是给下游判断"是不是同一份地址"，
      // 每次导航都换会让依赖 location.key 的 effect 白白重跑。
      key: `tab:${location.pathname}`,
    },
  }
}

export function TabsProvider({ children }: { children: ReactNode }) {
  const location = useLocation()
  const navigate = useNavigate()

  // 首帧就要有当前页：否则内容区会空白一帧，等到 effect 跑完才出现。
  const [tabs, setTabs] = useState<TabItem[]>(() => [toTab(location)])
  const [activeKey, setActiveKey] = useState<string>(location.pathname)

  /**
   * 滚动位置按标签页记。
   *
   * 用 ref 而不是 state：滚动每帧都在发生，放进 state 会让整棵外壳跟着重渲染，
   * 而这只是一个"下次切回来时用一下"的数字。
   */
  const scrollTops = useRef(new Map<string, number>())

  const rememberScroll = useCallback((key: string, top: number) => {
    scrollTops.current.set(key, top)
  }, [])

  const restoreScroll = useCallback((key: string) => scrollTops.current.get(key) ?? 0, [])

  // 地址栏 → 标签页。query 变了只更新已有标签页的地址，不新开一个。
  useEffect(() => {
    const { pathname, search, hash } = location
    const url = `${pathname}${search}${hash}`

    setTabs((prev) => {
      const index = prev.findIndex((tab) => tab.key === pathname)
      if (index === -1) return [...prev, toTab(location)]
      if (prev[index].url === url) return prev
      const next = prev.slice()
      next[index] = { ...prev[index], url, location: { ...prev[index].location, pathname, search, hash } }
      return next
    })

    setActiveKey(pathname)
  }, [location])

  const activate = useCallback(
    (key: string) => {
      const target = tabs.find((tab) => tab.key === key)
      if (target) navigate(target.url)
    },
    [navigate, tabs],
  )

  const close = useCallback(
    (key: string) => {
      if (key === HOME_TAB_KEY) return
      const index = tabs.findIndex((tab) => tab.key === key)
      if (index === -1) return

      const next = tabs.filter((tab) => tab.key !== key)
      setTabs(next)

      // 关的不是当前页时，只把它从条上摘掉，视图不动。
      if (key !== activeKey) return
      // 关掉当前页后落到右邻，没有右邻就落到左邻；都没有就回首页。
      const neighbour = next[Math.min(index, next.length - 1)]
      navigate(neighbour?.url ?? HOME_TAB_KEY)
    },
    [activeKey, navigate, tabs],
  )

  const value = useMemo<TabsValue>(
    () => ({ tabs, activeKey, activate, close, rememberScroll, restoreScroll }),
    [tabs, activeKey, activate, close, rememberScroll, restoreScroll],
  )

  return <TabsContext.Provider value={value}>{children}</TabsContext.Provider>
}

/** 读取标签页状态。未包 Provider 时抛错，避免静默失效。 */
export function useTabs(): TabsValue {
  const ctx = useContext(TabsContext)
  if (!ctx) throw new Error('useTabs 必须在 TabsProvider 内使用')
  return ctx
}
