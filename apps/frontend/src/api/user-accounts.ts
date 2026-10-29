import { http } from './client'
import type { SystemUser } from '@/types'

/** 建号入参。用户名与密码只在建号时提交。 */
export interface UserCreateInput {
  username: string
  password: string
  nickname?: string
  role: string
}

/**
 * 账号维护入参：三组字段都可选，nil 表示这次不改。
 *
 * 前端用 undefined 表达"不改"，与后端的指针语义一致——
 * 用空字符串表达"不改"会把昵称清空，用 0 表达会把账号停用。
 */
export interface UserUpdateInput {
  nickname?: string
  role?: string
  status?: number
  password?: string
}

/**
 * 账号维护。走 /user-accounts 而不是 /users：
 * 后者是"能派给谁"的选择器（只有启用账号），这里要连停用的一起看。
 *
 * 三个接口都要求管理员，业务员拿到 403。
 */
export const userAccountApi = {
  list: () => http.get<SystemUser[]>('/user-accounts'),
  create: (input: UserCreateInput) => http.post<SystemUser>('/user-accounts', input),
  /**
   * 改角色 / 停用 / 重置密码之后，该账号已签发的令牌会立即失效，
   * 他下一次请求会拿到 401 并被要求重新登录。
   */
  update: (id: number, input: UserUpdateInput) =>
    http.put<SystemUser>(`/user-accounts/${id}`, input),
}
