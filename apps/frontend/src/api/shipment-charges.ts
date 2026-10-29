import { http, toQuery } from './client'
import type { ShipmentCharge } from '@/types'

/**
 * 运费杂费写入口。
 *
 * 与货源同一套做法：列表走 dataProvider，写入走这里 ——
 * 表单要的是"提交哪些字段"这个明确契约，而不是某个 refine 钩子返回什么形状的 mutation。
 */
export interface ShipmentChargeInput {
  shipment_id: number
  /** 10 海运费 / 20 报关费 / 30 拖车费 / 40 保险费 / 90 其它 */
  type: number
  /** 收款方（货代或供应商），可空：账单只来一个总数时先记账、后补对象。 */
  partner_id?: number | null
  /** 定点字符串（后端 money.Decimal）：折算由后端按录入当日汇率做。 */
  amount: string
  /** 留空即本币，由后端按录入当日汇率折算并回填本币金额与汇率。 */
  currency_code?: string
  remark: string
}

export interface ShipmentChargeSummary {
  shipment_id: number
  total_base: string
}

export const shipmentChargeApi = {
  create: (body: ShipmentChargeInput) => http.post<ShipmentCharge>('/shipment-charges', body),

  // 不能改挂到另一票货（后端会拒），shipment_id 只做校验用。
  update: (id: number, body: ShipmentChargeInput) =>
    http.put<ShipmentCharge>(`/shipment-charges/${id}`, body),

  remove: (id: number) => http.delete<{ deleted: boolean }>(`/shipment-charges/${id}`),

  // 合计由后端算：费用分页展示，前端手里那一页不是全部。
  summary: (shipmentId: number) =>
    http.get<ShipmentChargeSummary>(`/shipment-charges-summary${toQuery({ shipment_id: shipmentId })}`),
}
