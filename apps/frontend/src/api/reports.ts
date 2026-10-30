import { http, toQuery } from './client'
import type {
  AgingReport,
  CollectionReport,
  ExchangeGainReport,
  PageResult,
  Partner360,
  ProductProfitReport,
  SalesDashboard,
  ShipmentProfit,
  TodoReport,
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

export interface CollectionParams {
  /** 10 应收回款 / 20 应付付款，留空按应收。 */
  direction?: number
  /** 回款率的统计期间，闭区间，按**到期日**筛。 */
  from?: string
  to?: string
  /** 超期明细的截止日，留空取今天。 */
  as_of?: string
  /** 超期明细行数上限，留空 50；0 取后端上限 500。 */
  limit?: number
}

export interface ProductProfitParams {
  product_id: number
  /** 按出货日筛，闭区间；留空不限。 */
  from?: string
  to?: string
}

export interface TodoParams {
  /** 每类清单的行数上限，留空 20；0 取后端上限 100。 */
  limit?: number
  /** 超期判断的截止日，留空取服务器当天。 */
  as_of?: string
}

export interface Partner360Params {
  partner_id: number
  /** 每块的行数上限，留空 10；0 取后端上限 50。 */
  limit?: number
  /** 超期判断的截止日，留空取今天。 */
  as_of?: string
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

export interface StatementParams {
  /** 必填：对账单是「跟某一家对」，没有全量对账单。 */
  partner_id: number
  /** 留空取该客户出现过的第一个币种；跨币种的客户要按币种分别对。 */
  currency_code?: string
  /** 账期区间，YYYY-MM-DD，左闭右闭；留空表示不限。 */
  from?: string
  to?: string
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

  /** 回款率与超期明细：前者是流量、后者是截至 as_of 的存量。 */
  collection: (params: CollectionParams = {}) =>
    http.get<CollectionReport>(`/reports/collection${toQuery({ ...params })}`),

  /**
   * 客户 360：一个主体的订单 / 出货 / 应收 / 跟进 / 商机。
   * 每块截断到 limit（默认 10），抬头合计永远是全量。
   */
  partner360: (params: Partner360Params) =>
    http.get<Partner360>(`/reports/partner-360${toQuery({ ...params })}`),

  /** 产品毛利下钻：某个产品逐票的收入 / 成本 / 运杂费 / 毛利（按出货日倒序）。 */
  productProfit: (params: ProductProfitParams) =>
    http.get<ProductProfitReport>(`/reports/product-profit${toQuery({ ...params })}`),

  /**
   * 客户对账单（T-53）：按主体 + 币种 + 账期出一张能直接发给客户的 PDF。
   *
   * 走 PDF 而不是页面表格：对账单是对方签回来存档的东西，屏幕上看的最终也要
   * 导出成文件；数字与 JSON 版同源，不会出现两套口径。
   */
  statementPdf: (params: StatementParams) =>
    http.download(`/reports/receivable-statement/pdf${toQuery({ ...params })}`, 'STATEMENT.pdf'),

  /** 待办清单：待跟进、应收待催、缺当日汇率（T-34）。 */
  todos: (params: TodoParams = {}) =>
    http.get<TodoReport>(`/reports/todos${toQuery({ ...params })}`),

  /** 销售看板：按维度汇总订单额与毛利，口径与单票毛利同源。 */
  salesDashboard: (params: SalesDashboardParams = {}) =>
    http.get<SalesDashboard>(`/reports/sales-dashboard${toQuery({ ...params })}`),
}
