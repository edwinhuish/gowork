import { http, toQuery } from './client'
import type { AgingReport } from '@/types'

export interface AgingParams {
  /** 10 应收 / 20 应付，留空按应收。 */
  direction?: number
  /** 只看一个主体。 */
  partner_id?: number
  /** 截止日 YYYY-MM-DD，留空取今天。所有分桶相对它算。 */
  as_of?: string
}

/**
 * 报表接口。
 *
 * 不走 dataProvider：报表返回聚合结果而不是分页资源，硬套 list 只会拿到
 * 一层 {list,total} 包装，把分桶字段挤到另一层去。
 */
export const reportApi = {
  aging: (params: AgingParams = {}) =>
    http.get<AgingReport>(`/reports/aging${toQuery({ ...params })}`),
}
