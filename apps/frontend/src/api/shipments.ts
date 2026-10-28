import { http, toQuery } from './client'
import type { Shipment, ShipmentCandidate } from '@/types'

/**
 * 出货单写入口。
 *
 * 入参里没有金额：出货本身不收钱也不付钱，费用归集在出货杂费（T-18）。
 * 数量上限由后端按「订单行数量 − 已出数量」校验，前端填的 max 只是提示。
 */
export interface ShipmentAllocationInput {
  sales_order_item_id: number
  qty: number
  remark?: string
}

export interface ShipmentInput {
  /** YYYY-MM-DD，留空取今天。 */
  ship_date?: string
  remark: string
  allocations: ShipmentAllocationInput[]
}

export const shipmentApi = {
  create: (body: ShipmentInput) => http.post<Shipment>('/shipments', body),

  update: (id: number, body: ShipmentInput) => http.put<Shipment>(`/shipments/${id}`, body),

  remove: (id: number) => http.delete<{ deleted: boolean }>(`/shipments/${id}`),

  /** 状态流转走服务端状态机，跳步会被拒（409）。 */
  updateStatus: (id: number, status: number) =>
    http.put<Shipment>(`/shipments/${id}/status`, { status }),

  /**
   * 还能出货的销售订单行。独立前缀而不是 /shipments/candidates：
   * 后者会被 /shipments/:id 当成 id=candidates 解析。
   *
   * excludeShipmentId 只在编辑页传：把这一票自己占的数量还给自己，
   * 否则"已出完的行"在编辑页的下拉里会消失。
   */
  candidates: (keyword?: string, excludeShipmentId?: number) =>
    http.get<ShipmentCandidate[]>(
      `/shipment-candidates${toQuery({
        keyword: keyword?.trim() || undefined,
        exclude_shipment_id: excludeShipmentId,
      })}`,
    ),
}
