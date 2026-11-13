import type { AuthProvider } from '@refinedev/core'

import { authApi } from '@/api/auth'
import { getToken, setToken } from '@/api/client'
import type { UserInfo } from '@/types'

const LOGIN_PATH = '/login'

/**
 * 身份缓存。
 *
 * refine 的 check() 在每次路由切换时都会跑，直接调 /auth/profile 会让每个页面跳转
 * 都多一次请求。这里缓存到模块作用域：刷新页面缓存为空、走一次接口，
 * 之后同一次会话内直接命中。登出和 401 时清空。
 */
let identity: UserInfo | null = null

export function clearIdentity() {
  identity = null
}

export const authProvider: AuthProvider = {
  login: async ({ username, password }) => {
    try {
      const res = await authApi.login(username, password)
      setToken(res.token)
      identity = res.user
      return { success: true, redirectTo: '/' }
    } catch (err) {
      return { success: false, error: err as Error }
    }
  },

  logout: async () => {
    try {
      await authApi.logout()
    } catch {
      // 服务端会话清理失败不阻塞本地退出：令牌本来就可能已经失效了。
    }
    setToken(null)
    clearIdentity()
    return { success: true, redirectTo: LOGIN_PATH }
  },

  check: async () => {
    if (!getToken()) {
      return { authenticated: false, redirectTo: LOGIN_PATH }
    }
    if (identity) {
      return { authenticated: true }
    }
    try {
      identity = await authApi.profile()
      return { authenticated: true }
    } catch {
      setToken(null)
      return { authenticated: false, redirectTo: LOGIN_PATH }
    }
  },

  /**
   * 任意请求失败都会走到这里。40101/40102 表示令牌无效或过期，
   * 此时必须 logout（清令牌）并跳登录页，否则用户会卡在页面上反复报错。
   */
  onError: async (error) => {
    const code = (error as { code?: number } | null)?.code
    if (code === 40101 || code === 40102) {
      setToken(null)
      clearIdentity()
      return { logout: true, redirectTo: LOGIN_PATH }
    }
    // 其他错误交给页面自己展示，不作全局跳转。
    return { error: error as Error }
  },

  getIdentity: async () => {
    if (identity) return identity
    if (!getToken()) return null
    identity = await authApi.profile()
    return identity
  },

  getPermissions: async () => {
    if (!identity && getToken()) {
      identity = await authApi.profile()
    }
    return identity?.role ?? null
  },
}
