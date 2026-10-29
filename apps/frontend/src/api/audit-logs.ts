import { http, toQuery } from './client'
import type { AuditLog, PageResult } from '@/types'

export interface AuditLogParams {
  page?: number
  page_size?: number
  /** 资源名，如 sales-orders。 */
  entity?: string
  /** 路径上的 :id。 */
  entity_id?: number
  /** 业务动作：create / update / status / merge … */
  action?: string
  actor_id?: number
  /** 按发生时间，闭区间且 to 含当天。 */
  from?: string
  to?: string
  /** 在路径、请求体与操作人里搜：手里通常只有一个单号。 */
  keyword?: string
}

/**
 * 操作日志接口（T-33）。
 *
 * 只有查：日志不可改，所以这里也没有 create / update / delete。
 * 不走 dataProvider 是因为筛选条件里有「关键字 + 时间区间」这类
 * 非资源字段，硬套 useTable 的 filters 反而更绕。
 */
export const auditLogApi = {
  list: (params: AuditLogParams = {}) =>
    http.get<PageResult<AuditLog>>(`/audit-logs${toQuery({ ...params })}`),
}
