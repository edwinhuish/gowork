import { useEffect, useRef } from 'react'

import { subscribeProgress } from '@/utils/load-progress'

/**
 * 顶部加载进度条。
 *
 * 它只回答「还有没有请求在飞」（计数在 api/client.ts），不猜真实完成度：
 * 浏览器不给已读字节数，装成百分比只会出现「卡在 80% 不动」的假精确。
 * 所以走常见的两段式 —— 开始时快速爬到 90% 附近（越接近越慢），
 * 最后一个请求回来再补齐 100%，然后淡出。
 *
 * 全程 ref + 直接改样式，不进 state：这条线每 120ms 动一次，
 * 为它重渲染整棵树不值得；而它只写自己的 transform / opacity，
 * 又固定在视口顶部，所以既不参与布局也不影响下面的交互。
 */
export function TopProgress() {
  const barRef = useRef<HTMLDivElement>(null)
  const progress = useRef(0)
  const trickle = useRef<number | null>(null)
  const settle = useRef<number | null>(null)

  useEffect(() => {
    const bar = () => barRef.current

    const paint = (value: number) => {
      progress.current = value
      const el = bar()
      if (el) el.style.transform = `scaleX(${value})`
    }

    const clearTimers = () => {
      if (trickle.current !== null) window.clearInterval(trickle.current)
      if (settle.current !== null) window.clearTimeout(settle.current)
      trickle.current = null
      settle.current = null
    }

    /** 把过渡关掉做一次瞬时赋值：不这么做，下面的起步与归零会被合并成一次动画。 */
    const jump = (value: number) => {
      const el = bar()
      if (!el) return
      el.style.transition = 'none'
      paint(value)
      // 读一次布局属性，强制浏览器结算上面这次赋值
      void el.offsetWidth
      el.style.transition = ''
    }

    const tick = () => {
      // 越接近 90% 涨得越少：这条尾巴表示"还在等"，不能提前到终点。
      // 已经在 90% 以上就不再动 —— 只涨不跌，回退会看成"进度倒流"。
      if (progress.current >= 0.9) return
      paint(progress.current + (0.9 - progress.current) * 0.12)
    }

    const start = () => {
      const el = bar()
      if (!el) return
      // 条还亮着说明上一段加载没真正结束（登录完紧接着拉首页就是这种），
      // 接着从当前位置往上走；归零再重来会看到"进度条退回去"。
      const resuming = el.style.opacity === '1' && progress.current > 0
      clearTimers()
      el.style.opacity = '1'
      if (!resuming) {
        jump(0)
        // 起步给一点长度：0 → 12% 的那一下是"我知道了"，比从 0 慢慢长更快被看见。
        paint(0.12)
      }
      trickle.current = window.setInterval(tick, 120)
    }

    const finish = () => {
      clearTimers()
      paint(1)
      const el = bar()
      if (!el) return
      // 走满再淡出：立刻淡出就看不出"这一次结束了"。
      settle.current = window.setTimeout(() => {
        el.style.opacity = '0'
        settle.current = window.setTimeout(() => jump(0), 240)
      }, 200)
    }

    /**
     * 请求清零时不立刻收条，先等 200ms。
     *
     * 一次页面加载常常是几个请求排队（列表 + 下拉 + 详情），中间隔着几十毫秒的空档。
     * 一清零就收条，看到的是"满格 → 消失 → 又从 12% 冒出来"，
     * 明明是一次加载却像加载了两次。等到安静下来再收，才是用户感知的那一段。
     */
    const finishSoon = () => {
      clearTimers()
      settle.current = window.setTimeout(finish, 200)
    }

    const unsubscribe = subscribeProgress((active) => (active ? start() : finishSoon()))
    return () => {
      unsubscribe()
      clearTimers()
    }
  }, [])

  // aria-hidden：它不携带可操作的信息，读屏软件没必要为每次请求念一句"加载中"。
  return <div ref={barRef} className="app-progress" aria-hidden="true" />
}
