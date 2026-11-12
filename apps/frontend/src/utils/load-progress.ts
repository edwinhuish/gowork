/**
 * 全局「有多少请求正在飞」的计数，供顶部加载进度条使用。
 *
 * 不做成 Provider：它跟 UI 树没有关系，而请求可以从任何地方发出
 * （refine 的 react-query、页面里直接 http.post、附件与导出下载）。
 * 计数的入口只有一处 —— api/client.ts，所有请求都从那里出去。
 *
 * 为什么是计数而不是布尔：并发请求很常见（列表 + 详情 + 下拉选项），
 * 用布尔的话第一个请求结束就把进度条收了，其余还在飞。
 */

type Listener = (active: boolean) => void

const listeners = new Set<Listener>()
let pending = 0

function emit(active: boolean) {
  listeners.forEach((listener) => listener(active))
}

/**
 * 记一次请求开始，返回「这次请求结束」的回调 —— 调用方在 finally 里执行，**只生效一次**。
 *
 * 幂等是容错：出错与超时路径里可能重复调用，重复执行会让计数掉成负数，
 * 那样下一次请求的 0 → 1 就浮不上来，进度条从此不再出现。
 */
export function trackRequest(): () => void {
  pending += 1
  if (pending === 1) emit(true)

  let settled = false
  return () => {
    if (settled) return
    settled = true
    pending -= 1
    if (pending === 0) emit(false)
  }
}

/** 订阅「有没有请求在飞」。返回取消订阅的函数。 */
export function subscribeProgress(listener: Listener): () => void {
  listeners.add(listener)
  return () => {
    listeners.delete(listener)
  }
}
