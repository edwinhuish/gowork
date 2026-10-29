import { http } from './client'
import type { UserInfo } from '@/types'

/**
 * 账号目录（T-32）。
 *
 * 只有"列表"，而且只返回**启用中**的账号：它唯一的用途是"把客户转给谁"的下拉。
 * 建号、改角色、重置密码属于用户维护模块，等真有第二个使用者再做。
 */
export const userApi = {
  listActive: () => http.get<UserInfo[]>('/users'),
}
