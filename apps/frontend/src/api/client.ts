// 统一的 HTTP 客户端：注入令牌、拆包 {code,message,data}、把业务错误变成异常。

const TOKEN_KEY = 'gowork_token'
const BASE_URL = '/api/v1'

/**
 * 登录态失效的广播事件名。
 *
 * client 不能直接 import store/auth：依赖链会是 client -> store -> api/auth -> client，
 * 形成循环。改成广播事件，由 AuthProvider 订阅后清空 user，路由守卫随即生效。
 */
export const AUTH_EXPIRED_EVENT = 'gowork:auth-expired'

export class ApiError extends Error {
  code: number

  constructor(message: string, code: number) {
    super(message)
    this.name = 'ApiError'
    this.code = code
  }
}

export function getToken(): string | null {
  return localStorage.getItem(TOKEN_KEY)
}

export function setToken(token: string | null) {
  if (token) {
    localStorage.setItem(TOKEN_KEY, token)
  } else {
    localStorage.removeItem(TOKEN_KEY)
  }
}

interface ApiBody<T> {
  code: number
  message: string
  data: T
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const headers = new Headers(init?.headers)
  headers.set('Content-Type', 'application/json')

  const token = getToken()
  if (token) {
    headers.set('Authorization', `Bearer ${token}`)
  }

  let res: Response
  try {
    res = await fetch(`${BASE_URL}${path}`, { ...init, headers })
  } catch {
    throw new ApiError('网络异常，请检查后端服务是否已启动', -1)
  }

  const body = (await res.json().catch(() => null)) as ApiBody<T> | null

  if (!res.ok || !body || body.code !== 0) {
    const message = body?.message || `请求失败 (${res.status})`
    const code = body?.code ?? res.status
    // 令牌失效：清本地令牌并广播，由 AuthProvider 退出登录态、路由守卫送回登录页。
    if (code === 40101 || code === 40102) {
      setToken(null)
      window.dispatchEvent(new Event(AUTH_EXPIRED_EVENT))
    }
    throw new ApiError(message, code)
  }

  return body.data
}

export const http = {
  get: <T>(path: string) => request<T>(path),
  post: <T>(path: string, body?: unknown) =>
    request<T>(path, { method: 'POST', body: JSON.stringify(body ?? {}) }),
  put: <T>(path: string, body?: unknown) =>
    request<T>(path, { method: 'PUT', body: JSON.stringify(body ?? {}) }),
  delete: <T>(path: string) => request<T>(path, { method: 'DELETE' }),
}

// toQuery 把对象拼成查询串，自动跳过空值。
export function toQuery(params: Record<string, unknown>): string {
  const search = new URLSearchParams()
  Object.entries(params).forEach(([key, value]) => {
    if (value === undefined || value === null || value === '') return
    search.set(key, String(value))
  })
  const qs = search.toString()
  return qs ? `?${qs}` : ''
}
