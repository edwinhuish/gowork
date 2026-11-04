import { useLayoutEffect, useRef } from 'react'
import type { ComponentProps } from 'react'
import { UNSAFE_LocationContext, useRoutes } from 'react-router'
import type { Location } from 'react-router'

import { useTabs, type TabItem } from '@/providers/tabs'
import { appRoutes } from '@/router/routes'

/*
 * react-router 把 navigationType 定成一个**没有导出**的内部枚举 Action
 * （成员值就是 'POP' / 'PUSH' / 'REPLACE'）。标签页里的这份 location 不是由
 * history 推送来的，语义上就是 'POP'；类型断言只集中在下面这一行，
 * 升级 react-router 时只需要核对它。
 */
type LocationContextValue = ComponentProps<typeof UNSAFE_LocationContext.Provider>['value']
const TAB_NAVIGATION_TYPE = 'POP' as unknown as LocationContextValue['navigationType']

/**
 * 内容区：每个已打开的标签页各占一个 pane，全部保持挂载，只切换显隐。
 *
 * 为什么不是"切走就卸载、回来重建"：那样筛选条件、表单里打了一半的字、
 * 展开的分组、无限滚动已加载的几页都会回到初始态。用户开标签页的本意
 * 恰恰是"先放着，一会儿回来接着弄"，卸载就把这件事的意义抹掉了。
 *
 * 因此每个 pane 都要能**独立匹配路由**：
 *   `useRoutes(appRoutes, tab.location)` 让 :id 从那个标签页自己的地址里取，
 *   否则两个详情页都会读到当前地址栏的 id；
 *   外面再包一层 `UNSAFE_LocationContext`，让页面里的 useLocation /
 *   useSearchParams 也读自己那份 —— refine 的 syncWithLocation 正是靠
 *   useLocation 解析筛选条件的，不隔离的话，切走再切回就等于把筛选清空。
 *
 * 这也是全站唯一一处用 react-router 的 UNSAFE_ 接口：它没有官方等价的
 * "给子树指定 location" 的 API，而这是"多标签页 + 保留状态"绕不开的一步。
 * 升级 react-router 时优先检查这里。
 */
export function TabPanes() {
  const { tabs, activeKey } = useTabs()

  return (
    <>
      {tabs.map((tab) => (
        <TabPane key={tab.key} tab={tab} active={tab.key === activeKey} />
      ))}
    </>
  )
}

interface TabPaneProps {
  tab: TabItem
  active: boolean
}

function TabPane({ tab, active }: TabPaneProps) {
  const { rememberScroll, restoreScroll } = useTabs()
  const ref = useRef<HTMLDivElement>(null)

  /*
   * 切回来时把滚动位置放回去。
   *
   * 必须是 layout effect：等浏览器把 display: none 撤掉、量完高度之后立刻设置，
   * 否则会先闪一帧顶部再跳下去。放在 effect 里则晚一帧，肉眼可见。
   */
  useLayoutEffect(() => {
    if (!active) return
    const element = ref.current
    if (element) element.scrollTop = restoreScroll(tab.key)
  }, [active, restoreScroll, tab.key])

  return (
    <div
      ref={ref}
      className="app-tab-pane"
      // 每个 pane 都是 data-app-content：utils/scroll.ts 取"可见的那一个"，
      // 页面里"回到顶部""记住滚动位置"因此始终作用在用户正看的这一页上。
      data-app-content
      hidden={!active}
      onScroll={(event) => rememberScroll(tab.key, event.currentTarget.scrollTop)}
    >
      <TabRoutes location={tab.location} />
    </div>
  )
}

function TabRoutes({ location }: { location: Location }) {
  // 在 Provider 外匹配：匹配依据是传进去的 location，而不是当前地址栏。
  const element = useRoutes(appRoutes, location)

  return (
    <UNSAFE_LocationContext.Provider
      value={{ location, navigationType: TAB_NAVIGATION_TYPE }}
    >
      {element}
    </UNSAFE_LocationContext.Provider>
  )
}
