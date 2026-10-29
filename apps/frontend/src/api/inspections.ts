import { http } from './client'

/** 验货结论（T-51b）。与后端 model.InspectionResult 一致。 */
export type InspectionResult = 'passed' | 'failed' | 'partial'

export const INSPECTION_RESULT_OPTIONS = [
  { value: 'passed', label: '合格' },
  { value: 'failed', label: '不合格（要求返工）' },
  { value: 'partial', label: '有条件接受（让价 / 挑拣）' },
]

/** 一次验货记录。类型就近定义：只服务采购详情的验货面板。 */
export interface Inspection {
  id: number
  purchase_order_id: number
  purchase_order_item_id: number | null
  result: InspectionResult
  sample_rate: number
  inspected_at: string
  inspector: string
  defects: string
  remark: string
  /** 采购行的 SKU 投影，整单验货时为 undefined。 */
  sku_code?: string
}

export interface InspectionInput {
  purchase_order_item_id?: number | null
  result: InspectionResult
  sample_rate: number
  inspected_at?: string
  inspector?: string
  defects?: string
  remark?: string
}

/**
 * 验货记录接口。不做编辑：验货是「某天某人验了这批货」的事实，
 * 改它等于改历史；录错删掉重录。
 */
export const inspectionApi = {
  list: (orderId: number) =>
    http.get<Inspection[]>(`/purchase-orders/${orderId}/inspections`),
  create: (orderId: number, input: InspectionInput) =>
    http.post<Inspection>(`/purchase-orders/${orderId}/inspections`, input),
  remove: (id: number) => http.delete<void>(`/inspections/${id}`),
}
