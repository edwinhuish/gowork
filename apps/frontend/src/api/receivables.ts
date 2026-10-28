import { http } from './client'
import type { Receivable } from '@/types'

/**
 * 应收单写入口。
 *
 * 只有三个可填字段：来源订单、金额、到期日。币种、汇率、对方主体一律由后端
 * 从来源订单带出并锁定 —— 允许前端传，就允许它和订单不一致，对账时就是两张单
 * 各说一个数（蓝图 F3 的硬规则）。
 */
export interface ReceivableInput {
  /** 只接受 10（销售订单）。 */
  source_type: number
  source_id: number
  /** 留空（或 0）表示把来源未开票余额全开出来。 */
  amount?: number
  /** 留空表示按客户账期推算：订单日 + credit_days。 */
  due_date?: string
  /** 只在编辑时用：90 表示作废。 */
  status?: number
  remark: string
}

export const receivableApi = {
  create: (body: ReceivableInput) => http.post<Receivable>('/receivables', body),
  update: (id: number, body: ReceivableInput) => http.put<Receivable>(`/receivables/${id}`, body),
  remove: (id: number) => http.delete<{ deleted: boolean }>(`/receivables/${id}`),
}
