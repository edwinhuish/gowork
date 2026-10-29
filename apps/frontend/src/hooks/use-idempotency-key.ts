import { useCallback, useState } from 'react'

/**
 * 给一次「表单提交会话」分配一个幂等键，配合后端的 Idempotency-Key 中间件。
 *
 * 用法：挂到 useForm 的 meta 上，并在提交成功后 rotate。
 *
 *   const { key, rotate } = useIdempotencyKey()
 *   const { formProps } = useForm({ resource, action: 'create',
 *     meta: { idempotencyKey: key }, onFinish: () => rotate() })
 *
 * - 提交失败：键不变。用户再点一次是**重试**，服务端若上次其实已经成功，
 *   会直接把那次的结果回放回来，不会建出第二条。
 * - 提交成功：换新键。下一次新建是另一次操作，不能被上一单的结果顶掉。
 *
 * 键必须在「用户意图」这一层生成。放在 http 层每次自动生成，就等于每次都是新操作，
 * 幂等无从谈起；反过来跨页面复用同一个键，则会把两笔不同的业务并成一笔。
 */
export function useIdempotencyKey() {
  const [key, setKey] = useState(newIdempotencyKey)
  const rotate = useCallback(() => setKey(newIdempotencyKey()), [])
  return { key, rotate }
}

function newIdempotencyKey(): string {
  // crypto.randomUUID 只在安全上下文可用：内网用 http:// 打开时它是 undefined，
  // 那种环境下降级成时间戳 + 随机串，够用且不抛错。
  if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
    return crypto.randomUUID()
  }
  return `${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 12)}`
}
