import { http, toQuery } from './client'
import type {
  AgingReport,
  ExchangeGainReport,
  PageResult,
  SalesDashboard,
  ShipmentProfit,
} from '@/types'

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

export interface ExchangeGainParams {
  /** 按到账日筛，闭区间；留空不限。 */
  from?: string
  to?: string
  partner_id?: number
  currency_code?: string
}

export interface SalesDashboardParams {
  /** customer / product / country / month，留空按客户。 */
  dimension?: string
  /**
   * 闭区间 YYYY-MM-DD。两个口径各筛各的：
   * 出货毛利按**出货日**，订单额按**下单日**。
   */
  from?: string
  to?: string
  /** 行数上限，留空 20；传 0 取后端上限 500。 */
  limit?: number
}

export const reportApi = {
  aging: (params: AgingParams = {}) =>
    http.get<AgingReport>(`/reports/aging${toQuery({ ...params })}`),

  /** 单票毛利，带 shipment_id 时是 0 或 1 条。金额全是本币。 */
  shipmentProfits: (params: ShipmentProfitParams = {}) =>
    http.get<PageResult<ShipmentProfit>>(`/reports/shipment-profits${toQuery({ ...params })}`),

  /** 汇兑损益：逐笔核销，正收益负损失，单位本币。 */
  exchangeGains: (params: ExchangeGainParams = {}) =>
    http.get<ExchangeGainReport>(`/reports/exchange-gains${toQuery({ ...params })}`),

  /** 销售看板：按维度汇总订单额与毛利，口径与单票毛利同源。 */
  salesDashboard: (params: SalesDashboardParams = {}) =>
    http.get<SalesDashboard>(`/reports/sales-dashboard${toQuery({ ...params })}`),
}
