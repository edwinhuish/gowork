import { http } from './client'
import type { Payable } from '@/types'

/**
 * 应付单写入口，规则与应收镜像。
 *
 * 应付的币种通常与销售不同（收美元、付人民币），所以采购单上的汇率
 * 必须原样锁进应付单——拿销售的汇率去折，等于把两笔独立的汇兑风险混成一笔。
 */
export interface PayableInput {
  /** 只接受 30（采购订单）。 */
  source_type: number
  source_id: number
  /** 留空（或 0）表示把来源未开票余额全开出来。 */
  amount?: number
  due_date?: string
  /** 只在编辑时用：90 表示作废。 */
  status?: number
  remark: string
}

export const payableApi = {
  create: (body: PayableInput) => http.post<Payable>('/payables', body),
  update: (id: number, body: PayableInput) => http.put<Payable>(`/payables/${id}`, body),
  remove: (id: number) => http.delete<{ deleted: boolean }>(`/payables/${id}`),
}
