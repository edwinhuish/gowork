import { useCallback, useLayoutEffect, useRef, useState } from 'react'
import { useSearchParams } from 'react-router'
import { Grid } from 'antd'

export type ListViewMode = 'list' | 'card'

/**
 * 列表页的「表格 / 卡片」双视图状态。
 *
 * 三个来源的优先级是 **URL > localStorage > 默认**：
 *   - URL 最优先，是因为把链接发给同事时要看到同一屏（`?view=card`）；
 *   - localStorage 次之，是因为一个人习惯卡片视图，不该每次进来都退回表格；
 *   - 都没有才用默认。
 *
 * **窄屏（< 768px）强制卡片**，压过上面三个来源：此时表格只能靠横向滚动看全，
 * 而这个宽度上横滑五六屏比竖着读几张卡难得多。强制只在渲染层生效，
 * 不改 URL 也不改 localStorage —— 把窗口拉宽，用户原来选的视图还在。
 * 页面拿到 `narrow` 后应当把切换控件隐藏掉：一个点了没反应的开关
 * 比没有开关更让人困惑。
 *
 * 切换时记住滚动位置，布局换完再按新的文档高度夹一次：卡片比表格高，
 * 但结果只有三五条时反而更矮，直接还原原位置会停在空白处。
 *
 * 视图切换只改渲染方式，不重建列表状态 —— 筛选、排序、页码都由 useTable
 * 持有，组件没有卸载，所以天然保留。滚动位置是唯一需要手动接的东西。
 */
export function useListView(storageKey: string, fallback: ListViewMode = 'list') {
  const [searchParams, setSearchParams] = useSearchParams()
  const [view, setViewState] = useState<ListViewMode>(() => readInitial(searchParams.get('view'), storageKey, fallback))
  const pendingScroll = useRef<number | null>(null)
  // antd 的 md 断点是 768px：md 为 false 即「比平板窄」。
  const narrow = Grid.useBreakpoint().md === false

  const setView = useCallback(
    (next: ListViewMode) => {
      setViewState((current) => {
        if (current === next) return current
        pendingScroll.current = window.scrollY
        try {
          window.localStorage.setItem(storageKey, next)
        } catch {
          // 隐私模式下 localStorage 会抛；记不住偏好不影响当下这次切换。
        }
        return next
      })
      setSearchParams(
        (prev: URLSearchParams) => {
          const params = new URLSearchParams(prev)
          params.set('view', next)
          return params
        },
        { replace: true },
      )
    },
    [setSearchParams, storageKey],
  )

  useLayoutEffect(() => {
    const target = pendingScroll.current
    if (target === null) return
    pendingScroll.current = null
    // 等一帧：此时新布局已经量完，scrollHeight 才是切换后的真实高度。
    const raf = requestAnimationFrame(() => {
      const max = Math.max(document.documentElement.scrollHeight - window.innerHeight, 0)
      window.scrollTo({ top: Math.min(target, max) })
    })
    return () => cancelAnimationFrame(raf)
  }, [view])

  return { view: narrow ? 'card' : view, setView, narrow }
}

function readInitial(fromUrl: string | null, storageKey: string, fallback: ListViewMode): ListViewMode {
  if (fromUrl === 'list' || fromUrl === 'card') return fromUrl
  try {
    const saved = window.localStorage.getItem(storageKey)
    if (saved === 'list' || saved === 'card') return saved
  } catch {
    // 读不到就退回默认值。
  }
  return fallback
}
