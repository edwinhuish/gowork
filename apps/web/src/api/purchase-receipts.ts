import { http } from './client'

/**
 * 收货单（T-51a）。类型就近定义而不放 types/index.ts：
 * 它只服务采购详情里的收货面板，没有第二处消费方；等它被第二个页面用到时再上移。
 */
export interface PurchaseReceiptItem {
  id: number
  purchase_order_item_id: number
  product_id: number
  sku_code: string
  name_cn: string
  unit: string
  qty: string
  remark: string
}

export interface PurchaseReceipt {
  id: number
  doc_no: string
  purchase_order_id: number
  status: number
  received_date: string
  remark: string
  items?: PurchaseReceiptItem[]
}

/** 采购详情的收货视图：列表 + 每行已收量（键是采购订单行 id）。 */
export interface PurchaseReceiptSummary {
  receipts: PurchaseReceipt[]
  received_by_item: Record<string, string>
}

export interface PurchaseReceiptInput {
  received_date?: string
  remark?: string
  items: { purchase_order_item_id: number; qty: string; remark?: string }[]
}

/**
 * 收货单接口。建单与列表挂在采购单下（收货永远属于某张采购单），
 * 确认与删除挂在收货单自己的前缀下。
 */
export const purchaseReceiptApi = {
  summary: (orderId: number) =>
    http.get<PurchaseReceiptSummary>(`/purchase-orders/${orderId}/receipts`),
  create: (orderId: number, input: PurchaseReceiptInput) =>
    http.post<PurchaseReceipt>(`/purchase-orders/${orderId}/receipts`, input),
  /** 确认会生成并确认一张采购入库单；重复确认返回 409。 */
  confirm: (id: number) => http.put<PurchaseReceipt>(`/purchase-receipts/${id}/confirm`),
  remove: (id: number) => http.delete<void>(`/purchase-receipts/${id}`),
  /**
   * 从收货单发起退货（T-52b3）：生成一张采购退货出库单，库存减少。
   * 累计退货不得超过收货量，超出返回 409。
   */
  returnItems: (id: number, input: PurchaseReturnInput) =>
    http.post<{ doc_no: string }>(`/purchase-receipts/${id}/return`, input),
}

/** 退货入参：按收货行填数量。 */
export interface PurchaseReturnInput {
  return_date?: string
  reason?: string
  items: { purchase_receipt_item_id: number; qty: string; remark?: string }[]
}
