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
  qty: string
  /** 装箱数据：印装箱单用，可留空。重量是这一行的合计，不是单箱。 */
  cartons?: number
  gross_weight?: string
  net_weight?: string
  measurement?: string
  remark?: string
}

export interface ShipmentInput {
  /** YYYY-MM-DD，留空取今天。 */
  ship_date?: string
  remark: string
  allocations: ShipmentAllocationInput[]
}

/**
 * 物流节点。单独一个入口：提单号与船期订舱后才有，
 * 那时出货单早已确认、走不了 edit。
 */
export interface ShipmentLogisticsInput {
  /** 承运人给的提单号，外部号，只记录。 */
  bl_no: string
  vessel: string
  /** YYYY-MM-DD，留空表示未定（会把已填的值清掉）。 */
  etd?: string
  eta?: string
}

export const shipmentApi = {
  create: (body: ShipmentInput) => http.post<Shipment>('/shipments', body),

  update: (id: number, body: ShipmentInput) => http.put<Shipment>(`/shipments/${id}`, body),

  remove: (id: number) => http.delete<{ deleted: boolean }>(`/shipments/${id}`),

  /** 状态流转走服务端状态机，跳步会被拒（409）。 */
  updateStatus: (id: number, status: number) =>
    http.put<Shipment>(`/shipments/${id}/status`, { status }),

  /** 只改物流节点，任何状态都能改。ETA 早于 ETD 会被拒（400）。 */
  updateLogistics: (id: number, body: ShipmentLogisticsInput) =>
    http.put<Shipment>(`/shipments/${id}/logistics`, body),

  /**
   * 改证重开：号码不变、商票版本加一（文件名与打印件带 -R{n}）。
   *
   * 没有入参：版本号由服务端自增，前端传"新版本"就等于允许跳版或倒退。
   * 草稿与已取消的一票会被拒（409）。
   */
  reissue: (id: number) => http.post<Shipment>(`/shipments/${id}/reissue`),

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
