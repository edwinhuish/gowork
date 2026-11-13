import { App } from 'antd'
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
 *
 * 不卸载不等于无限保留：开着的标签超过 TAB_LIMIT 时按"最久未激活"淘汰一个，
 * 被淘汰的那一页连同它的组件树一起卸载 —— 这是缓存边界，见 docs/ui-shell-refactor.md 4.3。
 */

export interface TabItem {
  /** pathname，见上面的划分。 */
  key: string
  /** 完整地址（含 query 与 hash），切回这个标签页时导航到这里。 */
  url: string
  label: string
  /** 这个标签页自己的 location：内容区按它匹配路由、读 query。 */
  location: Location
  /**
   * 刷新计数。加一就让这一页的页面组件重挂载（见 tab-panes 的 TabRoutes key）：
   * 重挂载 ≈ 重新请求，而"刷新"在标签页语义里就该是这个意思 ——
   * 不重挂载的话，refine 的 query 还带着旧数据，看起来什么也没发生。
   */
  version: number
}

interface TabsValue {
  tabs: TabItem[]
  activeKey: string
  activate: (key: string) => void
  close: (key: string) => void
  /** 刷新某一页：只重挂载它自己的内容树，标签条与其它页不动。 */
  refresh: (key: string) => void
  /** 右键菜单里的批量关闭。固定标签（首页）在任何一种里都保留。 */
  closeOthers: (key: string) => void
  closeLeft: (key: string) => void
  closeRight: (key: string) => void
  closeAll: () => void
  /** 拖拽排序：把 key 挪到 targetKey 之前或之后（after 为真表示之后）。 */
  move: (key: string, targetKey: string, after: boolean) => void
  rememberScroll: (key: string, top: number) => void
  restoreScroll: (key: string) => number
}

/** 首页固定：标签页全关掉时还要有个落点。 */
export const HOME_TAB_KEY = '/'

/**
 * 标签上限（固定标签不计入）。
 *
 * 每个标签页都是一份活着的组件树 + 一份 query 缓存，不封顶等于让"开着不关"
 * 慢慢吃掉内存。10 是能覆盖"查主数据 → 开单据 → 回来看一眼"这条实际动线的数量，
 * 再多就该用搜索或列表筛选，而不是靠标签页并排。
 */
export const TAB_LIMIT = 10

/** 固定标签：不可关闭，右键菜单里的四种批量关闭都要绕开它。 */
export function isFixedTab(key: string): boolean {
  return key === HOME_TAB_KEY
}

/**
 * 首页始终排第一。
 *
 * 只有它一个固定标签，落到中间会让"固定"看起来像"碰巧还在"；
 * 顺序也只是内存里的，不落盘。
 */
function homeFirst(list: TabItem[]): TabItem[] {
  const index = list.findIndex((tab) => tab.key === HOME_TAB_KEY)
  if (index <= 0) return list
  const next = list.slice()
  const [home] = next.splice(index, 1)
  next.unshift(home)
  return next
}

const TabsContext = createContext<TabsValue | null>(null)

function toTab(location: Pick<Location, 'pathname' | 'search' | 'hash'>): TabItem {
  return {
    key: location.pathname,
    url: `${location.pathname}${location.search}${location.hash}`,
    label: describeRoute(location.pathname),
    version: 0,
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
  const { message } = App.useApp()

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

  /**
   * 每个标签最后一次被激活的时刻，淘汰时挑最小的那个。
   *
   * 同样用 ref：它只影响"关哪个"，不参与渲染。时间戳用 `Date.now()` 而不是计数器，
   * 是因为这里不需要严格的全序 —— 只在一堆互不相等的值里挑最小，撞毫秒也不影响结果。
   */
  const lastActive = useRef(new Map<string, number>())

  // 地址栏 → 标签页。query 变了只更新已有标签页的地址，不新开一个。
  useEffect(() => {
    const { pathname, search, hash } = location
    const url = `${pathname}${search}${hash}`

    setTabs((prev) => {
      const index = prev.findIndex((tab) => tab.key === pathname)
      if (index === -1) return homeFirst([...prev, toTab(location)])
      if (prev[index].url === url) return prev
      const next = prev.slice()
      next[index] = { ...prev[index], url, location: { ...prev[index].location, pathname, search, hash } }
      return next
    })

    setActiveKey(pathname)
    lastActive.current.set(pathname, Date.now())
  }, [location])

  /**
   * 超限淘汰：关掉"最久没被激活"的那一个，并说清关的是谁。
   *
   * 几处刻意的选择：
   *   - **不淘汰当前页**：把用户正在看的页关掉，等于抽掉脚下的地板；
   *   - 固定标签永远不在候选里；
   *   - 顺手清掉已关闭标签的时间戳，否则这张表会跟着会话一直长；
   *   - 提示走顶部 message 而不是弹窗：这是外壳自己做的决定，不该打断手上的活。
   */
  useEffect(() => {
    const live = new Set(tabs.map((tab) => tab.key))
    for (const key of [...lastActive.current.keys()]) {
      if (!live.has(key)) lastActive.current.delete(key)
    }

    const closable = tabs.filter((tab) => !isFixedTab(tab.key))
    if (closable.length <= TAB_LIMIT) return

    const candidates = closable.filter((tab) => tab.key !== activeKey)
    if (candidates.length === 0) return

    const victim = candidates.reduce((oldest, tab) =>
      (lastActive.current.get(tab.key) ?? 0) < (lastActive.current.get(oldest.key) ?? 0)
        ? tab
        : oldest,
    )

    setTabs((prev) => prev.filter((tab) => tab.key !== victim.key))
    message.warning(`标签页最多 ${TAB_LIMIT} 个，已关闭最久未使用的「${victim.label}」`)
  }, [activeKey, message, tabs])

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

  /**
   * 批量关闭的公共出口：先摘标签，再决定要不要导航。
   *
   * 落点与单关一致（右邻 → 左邻 → 首页），否则"关掉当前页"在两种入口下会落到
   * 两个不同的地方。固定标签由调用方过滤，这里不再判一次 ——
   * 两处都判，改规则时必然只改一处。
   */
  const applyClose = useCallback(
    (removedKeys: string[]) => {
      if (removedKeys.length === 0) return
      const removed = new Set(removedKeys)
      const index = tabs.findIndex((tab) => tab.key === activeKey)
      const next = homeFirst(tabs.filter((tab) => !removed.has(tab.key)))
      setTabs(next)

      if (!removed.has(activeKey)) return
      const neighbour = next[Math.min(Math.max(index, 0), next.length - 1)]
      navigate(neighbour?.url ?? HOME_TAB_KEY)
    },
    [activeKey, navigate, tabs],
  )

  const refresh = useCallback((key: string) => {
    setTabs((prev) =>
      prev.map((tab) => (tab.key === key ? { ...tab, version: tab.version + 1 } : tab)),
    )
  }, [])

  const closeOthers = useCallback(
    (key: string) => {
      applyClose(tabs.filter((tab) => tab.key !== key && !isFixedTab(tab.key)).map((tab) => tab.key))
    },
    [applyClose, tabs],
  )

  const closeLeft = useCallback(
    (key: string) => {
      const index = tabs.findIndex((tab) => tab.key === key)
      if (index <= 0) return
      applyClose(tabs.slice(0, index).filter((tab) => !isFixedTab(tab.key)).map((tab) => tab.key))
    },
    [applyClose, tabs],
  )

  const closeRight = useCallback(
    (key: string) => {
      const index = tabs.findIndex((tab) => tab.key === key)
      if (index === -1) return
      applyClose(tabs.slice(index + 1).filter((tab) => !isFixedTab(tab.key)).map((tab) => tab.key))
    },
    [applyClose, tabs],
  )

  const closeAll = useCallback(() => {
    applyClose(tabs.filter((tab) => !isFixedTab(tab.key)).map((tab) => tab.key))
  }, [applyClose, tabs])

  /**
   * 拖拽排序：**只改标签条上的顺序，不导航**。
   *
   * 激活态由地址栏决定，所以"排在前面的标签页"和"当前正在看的页"本来就是两件事，
   * 拖动不该顺手把某一页切过来。顺序也不落盘：它是这一次会话里的摆放习惯，
   * 刷新后按打开顺序重来（标签页本身同样不持久化）。
   */
  const move = useCallback((key: string, targetKey: string, after: boolean) => {
    setTabs((prev) => {
      const from = prev.findIndex((tab) => tab.key === key)
      const target = prev.findIndex((tab) => tab.key === targetKey)
      if (from === -1 || target === -1 || from === target) return prev

      const next = prev.slice()
      const [moved] = next.splice(from, 1)
      // 目标被抽走之后它的下标会前移一格，插到它后面就是"新下标 + 1"。
      const at = (from < target ? target - 1 : target) + (after ? 1 : 0)
      next.splice(at, 0, moved)
      return next
    })
  }, [])

  const value = useMemo<TabsValue>(
    () => ({
      tabs,
      activeKey,
      activate,
      close,
      refresh,
      closeOthers,
      closeLeft,
      closeRight,
      closeAll,
      move,
      rememberScroll,
      restoreScroll,
    }),
    [
      tabs,
      activeKey,
      activate,
      close,
      refresh,
      closeOthers,
      closeLeft,
      closeRight,
      closeAll,
      move,
      rememberScroll,
      restoreScroll,
    ],
  )

  return <TabsContext.Provider value={value}>{children}</TabsContext.Provider>
}

/** 读取标签页状态。未包 Provider 时抛错，避免静默失效。 */
export function useTabs(): TabsValue {
  const ctx = useContext(TabsContext)
  if (!ctx) throw new Error('useTabs 必须在 TabsProvider 内使用')
  return ctx
}
