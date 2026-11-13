/**
 * 内容区的滚动位置。
 *
 * 外壳把窗口锁成一屏高（顶栏固定、侧栏与内容区各自滚），整页不再有纵向滚动。
 * 也就是说 `window.scrollY` 恒为 0、`window.scrollTo` 是空操作 ——
 * "回到顶部""记住滚动位置"这类需求必须打在内容区那个元素上。
 *
 * 多标签页之后，滚动容器不是一个而是**每个标签页一个**：已打开的标签页都还在
 * DOM 里（隐藏而非卸载，见 components/layout/tab-panes.tsx），所以 `[data-app-content]`
 * 会同时匹配到好几个。下面一律取"可见的那一个"——用户能看见的才该跟着动。
 *
 * 用 DOM 查询而不是 Context：调用点只有视图切换与无限滚动两处，
 * 为了它加一层 Provider，要求所有页面都挂在下面，代价比收益大。
 * 选择器与 AppLayout 里的 `data-app-content` 是同一份约定。
 */
const CONTENT_SELECTOR = '[data-app-content]'

/**
 * 当前可见的内容区滚动容器；登录页等外壳之外的地方返回 null。
 *
 * 全隐藏时（标签页切换的中间帧）退回最后一个：返回一个真实元素总比返回 null
 * 让调用方去判空省事，而那种帧里本来也没人读它。
 */
export function getContentScroller(): HTMLElement | null {
  const nodes = document.querySelectorAll<HTMLElement>(CONTENT_SELECTOR)
  let last: HTMLElement | null = null
  for (const node of nodes) {
    last = node
    // display: none 的元素量不到任何矩形，用它判断可见比读 offsetParent 稳
    // （后者对 position: fixed 也返回 null）。
    if (node.getClientRects().length > 0) return node
  }
  return last
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
