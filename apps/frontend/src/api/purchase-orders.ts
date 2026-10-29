import { http } from './client'
import type { PurchaseMilestone, PurchaseOrder } from '@/types'

/**
 * 采购订单写入口。
 *
 * 与销售订单同理：入参里没有合计字段，合计由后端从明细汇总。
 * sales_order_item_id 用来挂「以销定采」——这行采购是为哪张销售单的哪一行备的。
 */
export interface PurchaseOrderItemInput {
  product_id: number
  qty: string
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

  /**
   * 跟单节点（T-38）：一行一个节点，计划日与完成日分开。
   *
   * 补录是**部分**的——只提交这一次动过的节点。整体替换会把别人刚填的覆盖掉，
   * 而跟单恰恰是两个人各填各的。
   */
  milestones: (id: number) =>
    http.get<PurchaseMilestone[]>(`/purchase-orders/${id}/milestones`),

  saveMilestones: (id: number, milestones: PurchaseMilestoneInput[]) =>
    http.put<PurchaseMilestone[]>(`/purchase-orders/${id}/milestones`, { milestones }),
}

/** 一条节点的补录入参：空串表示清空该日期（不是"不修改"）。 */
export interface PurchaseMilestoneInput {
  purchase_order_item_id: number
  node: number
  plan_date: string
  done_date: string
  remark?: string
}
