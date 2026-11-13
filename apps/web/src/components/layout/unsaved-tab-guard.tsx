import { useWarnAboutChange } from '@refinedev/core'
import { useContext, useEffect, useRef } from 'react'
import { UNSAFE_NavigationContext, useLocation } from 'react-router'

import { useTabs } from '@/providers/tabs'

/**
 * 未保存提醒 —— 多标签页版。
 *
 * refine 自带的 `UnsavedChangesNotifier` 拦的是**所有**路由跳转，而多标签页之后
 * "点另一个标签页"也是一次跳转：那一页并没有卸载，表单还在原地，
 * 弹出「确定要离开吗」既不准确又烦人（每次切换都要点一下）。
 *
 * 所以这里只改一处判断：目标路径**已经是打开的标签页**时直接放行，
 * 其余情况（点侧栏去新页面、跳转登录等）仍然是原来的确认框。
 * 刷新 / 关窗口的 beforeunload 也照旧。
 *
 * 拦截方式与 refine 原版一致：包一层 `navigator.push` / `navigator.go`。
 * 不用 react-router 的 `useBlocker` —— 它要求 data router，而本项目是
 * `<BrowserRouter>`，调它会直接抛 "useBlocker must be used within a data router"。
 *
 * 两件事不能省：
 *   - 地址真的换过之后把警告状态清掉，否则下一个页面会继承上一页的 dirty 标记；
 *   - 取消时 reset（什么都不做），让用户留在原页面。
 */
const MESSAGE = '当前页面有未保存的修改，确定要离开吗？'

/** 从 navigate 的参数里取出 pathname；数字（前进/后退）与状态对象取不到。 */
function resolvePathname(to: unknown): string | null {
  if (typeof to === 'string') return to.split(/[?#]/)[0] || '/'
  if (to && typeof to === 'object' && 'pathname' in to) {
    const { pathname } = to as { pathname?: unknown }
    if (typeof pathname === 'string') return pathname
  }
  return null
}

export function UnsavedTabGuard() {
  const { warnWhen, setWarnWhen } = useWarnAboutChange()
  const { tabs } = useTabs()
  const { pathname } = useLocation()
  const { navigator } = useContext(UNSAFE_NavigationContext)

  // patch 只在依赖变化时重装，但判断要用最新的标签页清单：走 ref 读。
  const tabsRef = useRef(tabs)
  tabsRef.current = tabs

  useEffect(() => {
    return () => setWarnWhen(false)
  }, [pathname, setWarnWhen])

  useEffect(() => {
    if (!warnWhen) return
    const onBeforeUnload = (event: BeforeUnloadEvent) => {
      event.preventDefault()
      event.returnValue = MESSAGE
      return MESSAGE
    }
    window.addEventListener('beforeunload', onBeforeUnload)
    return () => window.removeEventListener('beforeunload', onBeforeUnload)
  }, [warnWhen])

  useEffect(() => {
    if (!warnWhen) return

    const go = navigator.go
    const push = navigator.push

    const allow = (to: unknown): boolean => {
      const target = resolvePathname(to)
      // 已打开的标签页：页面不卸载，表单与滚动都在，不算离开。
      if (target && tabsRef.current.some((tab) => tab.key === target)) return true
      return window.confirm(MESSAGE)
    }

    navigator.push = (...args: Parameters<typeof push>) => {
      if (allow(args[0])) push(...args)
    }
    navigator.go = (...args: Parameters<typeof go>) => {
      if (allow(args[0])) go(...args)
    }

    return () => {
      navigator.push = push
      navigator.go = go
    }
  }, [navigator, warnWhen])

  return null
}
