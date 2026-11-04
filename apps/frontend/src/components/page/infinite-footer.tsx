import { useCallback, useEffect, useRef, useState } from 'react'
import { Button, Space, Spin } from 'antd'

import type { InfiniteFeed } from '@/hooks/use-infinite-feed'
import { getContentScroller } from '@/utils/scroll'

/**
 * 列表底部的无限滚动状态条。
 *
 * 分页条退场之后，「还有没有下一批」这件事必须在界面上有明确答案，
 * 否则用户不知道是"到底了"还是"卡住了"。这里只有四种状态，各一句话：
 *
 *   还有下一批 → 一根 1px 的哨兵（滚到附近自动触发）+ 一个「加载更多」
 *   正在加载   → 转圈 + 正在加载…
 *   加载失败   → 失败文案 + 重试（重试的是失败的那一批，不是从第一页重来）
 *   已经到底   → 已加载全部 N 条
 *
 * 哨兵为什么用 IntersectionObserver 而不是 scroll 事件：滚的是内容区
 * （`[data-app-content]`，见 utils/scroll.ts），scroll 事件每帧都来，
 * 在长列表里自己算"距底部还有多少"是白烧 CPU；观察器只回调一次越界。
 * rootMargin 提前 480px 触发，等真的贴到底再请求，用户会先看到一段空白。
 */
export interface InfiniteFooterProps {
  feed: InfiniteFeed<unknown>
  /** 结束提示里的量词：客户 / 条 / 张。 */
  noun?: string
}

export function InfiniteFooter({ feed, noun = '条' }: InfiniteFooterProps) {
  const active = feed.hasMore && !feed.error && !feed.loadingMore
  const sentinelRef = useSentinel(active, feed.loadMore)

  if (feed.error) {
    return (
      <div className="infinite-footer" role="status">
        <Space size={8} wrap>
          <span className="infinite-footer-text is-error">
            加载失败：{feed.error.message || '请稍后重试'}
          </span>
          <Button size="small" onClick={feed.retry}>
            重试
          </Button>
        </Space>
      </div>
    )
  }

  if (feed.loadingMore) {
    return (
      <div className="infinite-footer" role="status" aria-live="polite">
        <Space size={8}>
          <Spin size="small" />
          <span className="infinite-footer-text">正在加载…</span>
        </Space>
      </div>
    )
  }

  if (feed.hasMore) {
    return (
      <div className="infinite-footer" role="status">
        <div ref={sentinelRef} className="infinite-sentinel" aria-hidden />
        {/* 自动加载之外留一个可点的入口：键盘用户与观察器失效时的退路。 */}
        <Button type="link" size="small" onClick={feed.loadMore}>
          加载更多
        </Button>
      </div>
    )
  }

  // 一条都没有时不写「已加载全部 0 条」：空态提示已经说明过了。
  if (feed.total === 0) return null

  return (
    <div className="infinite-footer" role="status">
      <span className="infinite-footer-text">
        已加载全部 {feed.total} {noun}
      </span>
    </div>
  )
}

/** 观察哨兵：一旦它进入内容区可视范围（含提前量）就取下一批。 */
function useSentinel(enabled: boolean, onHit: () => void) {
  const [node, setNode] = useState<HTMLDivElement | null>(null)
  // 回调每次渲染都是新的，放进 effect 依赖会让观察器反复重建，用 ref 转一手。
  const hitRef = useRef(onHit)
  hitRef.current = onHit

  useEffect(() => {
    if (!node || !enabled) return
    if (typeof IntersectionObserver === 'undefined') return

    // root 取哨兵自己所在的那个滚动容器，而不是 document 里"当前可见"的那个：
    // 多标签页下每个标签页各有一个滚动容器，观察器一旦绑在别人的 pane 上，
    // 切回来之后这个列表就再也不会自动加载了。
    const observer = new IntersectionObserver(
      (entries) => {
        if (entries.some((entry) => entry.isIntersecting)) hitRef.current()
      },
      {
        root: node.closest<HTMLElement>('[data-app-content]') ?? getContentScroller(),
        rootMargin: '0px 0px 480px 0px',
      },
    )
    observer.observe(node)
    return () => observer.disconnect()
  }, [node, enabled])

  return useCallback((element: HTMLDivElement | null) => setNode(element), [])
}
