import { http } from './client'
import type { PurchaseOrder } from '@/types'

/**
 * 采购订单写入口。
 *
 * 与销售订单同理：入参里没有合计字段，合计由后端从明细汇总。
 * sales_order_item_id 用来挂「以销定采」——这行采购是为哪张销售单的哪一行备的。
 */
export interface PurchaseOrderItemInput {
  product_id: number
  qty: number
  unit_price: number
  sales_order_item_id?: number
  source_id?: number
  remark?: string
}

export interface PurchaseOrderInput {
  supplier_id: number
  trade_term: string
  currency_code?: string
  /** YYYY-MM-DD，留空取今天。 */
  order_date?: string
  /** YYYY-MM-DD，可留空；不能早于下单日。 */
  delivery_date?: string
  remark: string
  items: PurchaseOrderItemInput[]
}

export const purchaseOrderApi = {
  create: (body: PurchaseOrderInput) => http.post<PurchaseOrder>('/purchase-orders', body),

  update: (id: number, body: PurchaseOrderInput) =>
    http.put<PurchaseOrder>(`/purchase-orders/${id}`, body),

  remove: (id: number) => http.delete<{ deleted: boolean }>(`/purchase-orders/${id}`),

  /** 状态流转走服务端状态机，跳步会被拒（409）。 */
  updateStatus: (id: number, status: number) =>
    http.put<PurchaseOrder>(`/purchase-orders/${id}/status`, { status }),
}
