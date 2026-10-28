// 统一的 HTTP 客户端：注入令牌、拆包 {code,message,data}、把业务错误变成异常。

const TOKEN_KEY = 'gowork_token'
const BASE_URL = '/api/v1'

// 服务端滑动续期时下发的响应头，与后端 middleware.RenewedTokenHeader 对应。
const RENEWED_TOKEN_HEADER = 'X-Renewed-Token'

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
  // multipart 的 Content-Type 必须由浏览器生成（它要带 boundary），
  // 手写 application/json 会让服务端解析不出任何表单字段。
  if (!(init?.body instanceof FormData)) {
    headers.set('Content-Type', 'application/json')
  }

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

  // 令牌剩余有效期不足一半时服务端会自动续期，新令牌放在响应头里。
  // 这一步放在解析响应体之前：body 解析失败也不该丢掉刚拿到的新令牌。
  const renewed = res.headers.get(RENEWED_TOKEN_HEADER)
  if (renewed) {
    setToken(renewed)
  }

  const body = (await res.json().catch(() => null)) as ApiBody<T> | null

  if (!res.ok || !body || body.code !== 0) {
    const message = body?.message || `请求失败 (${res.status})`
    const code = body?.code ?? res.status
    // 令牌失效：清掉本地令牌。跳转由 authProvider.onError 统一处理 ——
    // 它在 dataProvider 的失败链路上由 refine 调用，这里不需要再广播事件。
    if (code === 40101 || code === 40102) {
      setToken(null)
    }
    throw new ApiError(message, code)
  }

  return body.data
}

/**
 * 下载二进制附件（单据 PDF）。
 *
 * 不走 request()：那条链路假定响应体是 {code,message,data} 的 JSON，
 * 而 PDF 是二进制流，用 res.json() 解析只会得到 null。
 * 失败时后端仍然返回统一响应体，所以错误照样能按 code 判断。
 */
export async function download(path: string, fallbackName: string): Promise<void> {
  const headers = new Headers()
  const token = getToken()
  if (token) {
    headers.set('Authorization', `Bearer ${token}`)
  }

  let res: Response
  try {
    res = await fetch(`${BASE_URL}${path}`, { headers })
  } catch {
    throw new ApiError('网络异常，请检查后端服务是否已启动', -1)
  }

  const renewed = res.headers.get(RENEWED_TOKEN_HEADER)
  if (renewed) {
    setToken(renewed)
  }

  if (!res.ok) {
    const body = (await res.json().catch(() => null)) as ApiBody<unknown> | null
    const code = body?.code ?? res.status
    if (code === 40101 || code === 40102) {
      setToken(null)
    }
    throw new ApiError(body?.message || `下载失败 (${res.status})`, code)
  }

  // 文件名优先用服务端给的：单号在服务端才算得出来，前端只能猜。
  // 优先取 RFC 5987 的 filename*：中文名只有它带得回来，
  // filename="..." 那份是给老客户端的 ASCII 兜底（中文全被换成 _）。
  const disposition = res.headers.get('Content-Disposition') ?? ''
  const extended = /filename\*=UTF-8''([^;]+)/i.exec(disposition)
  const plain = /filename="?([^";]+)"?/i.exec(disposition)
  let filename = fallbackName
  if (extended) {
    try {
      filename = decodeURIComponent(extended[1])
    } catch {
      filename = plain?.[1] || fallbackName
    }
  } else if (plain) {
    filename = plain[1]
  }

  const blob = await res.blob()
  const url = URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  link.download = filename
  document.body.appendChild(link)
  link.click()
  link.remove()
  URL.revokeObjectURL(url)
}

export const http = {
  get: <T>(path: string) => request<T>(path),
  post: <T>(path: string, body?: unknown) =>
    request<T>(path, { method: 'POST', body: JSON.stringify(body ?? {}) }),
  put: <T>(path: string, body?: unknown) =>
    request<T>(path, { method: 'PUT', body: JSON.stringify(body ?? {}) }),
  delete: <T>(path: string) => request<T>(path, { method: 'DELETE' }),
  /**
   * 上传文件（multipart/form-data）。
   *
   * 单独一个方法而不是复用 post：FormData 的 Content-Type 里带 boundary，
   * 只能由浏览器生成；行为上它与 post 一致（同一套令牌注入与错误拆包）。
   */
  postForm: <T>(path: string, form: FormData) =>
    request<T>(path, { method: 'POST', body: form }),
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
