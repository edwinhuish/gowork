import { http } from './client'
import type { SalesOrder } from '@/types'

/**
 * 销售订单写入口。
 *
 * 入参里**没有** amount_total / amount_base_total：合计由后端从明细汇总，
 * 前端即使把总额塞进请求体也会被丢弃。这样"改了明细没改总额"在结构上就不可能发生。
 */
export interface SalesOrderItemInput {
  product_id: number
  qty: number
  unit_price: number
  remark?: string
}

export interface SalesOrderInput {
  partner_id: number
  trade_term: string
  currency_code?: string
  /** YYYY-MM-DD，留空取今天。 */
  order_date?: string
  /** YYYY-MM-DD，可留空；不能早于下单日。 */
  delivery_date?: string
  remark: string
  items: SalesOrderItemInput[]
}

export const salesOrderApi = {
  create: (body: SalesOrderInput) => http.post<SalesOrder>('/sales-orders', body),

  update: (id: number, body: SalesOrderInput) =>
    http.put<SalesOrder>(`/sales-orders/${id}`, body),

  remove: (id: number) => http.delete<{ deleted: boolean }>(`/sales-orders/${id}`),

  /** 状态流转走服务端状态机，跳步会被拒（409）。 */
  updateStatus: (id: number, status: number) =>
    http.put<SalesOrder>(`/sales-orders/${id}/status`, { status }),
}
