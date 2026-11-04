/**
 * 内容区的滚动位置。
 *
 * 外壳把窗口锁成一屏高（顶栏固定、侧栏与内容区各自滚），整页不再有纵向滚动。
 * 也就是说 `window.scrollY` 恒为 0、`window.scrollTo` 是空操作 ——
 * "回到顶部""记住滚动位置"这类需求必须打在内容区那个元素上。
 *
 * 用 DOM 查询而不是 Context：调用点只有路由切换与视图切换两处，
 * 为了它加一层 Provider，要求所有页面都挂在下面，代价比收益大。
 * 选择器与 AppLayout 里的 `data-app-content` 是同一份约定。
 */
const CONTENT_SELECTOR = '[data-app-content]'

/** 内容区的滚动容器；登录页等外壳之外的地方返回 null。 */
export function getContentScroller(): HTMLElement | null {
  return document.querySelector<HTMLElement>(CONTENT_SELECTOR)
}

/** 内容区当前滚了多少；不在外壳里时按 0 处理，调用方不必判空。 */
export function getScrollTop(): number {
  return getContentScroller()?.scrollTop ?? 0
}

/** 把内容区滚到指定位置（浏览器会自己夹到可滚范围内）。 */
export function scrollContentTo(top: number): void {
  getContentScroller()?.scrollTo({ top })
}

/** 内容区还能往下滚多少：用于"记住的位置超过了新内容的长度"时收一下。 */
export function getMaxScrollTop(): number {
  const el = getContentScroller()
  if (!el) return 0
  return Math.max(el.scrollHeight - el.clientHeight, 0)
}
