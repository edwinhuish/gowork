import { http, toQuery } from './client'
import type { AgingReport, PageResult, ShipmentProfit } from '@/types'

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
export interface ShipmentProfitParams {
  /** 只看一票：出货详情页的毛利面板传当前票。 */
  shipment_id?: number
  page?: number
  page_size?: number
}

export const reportApi = {
  aging: (params: AgingParams = {}) =>
    http.get<AgingReport>(`/reports/aging${toQuery({ ...params })}`),

  /** 单票毛利，带 shipment_id 时是 0 或 1 条。金额全是本币。 */
  shipmentProfits: (params: ShipmentProfitParams = {}) =>
    http.get<PageResult<ShipmentProfit>>(`/reports/shipment-profits${toQuery({ ...params })}`),
}
