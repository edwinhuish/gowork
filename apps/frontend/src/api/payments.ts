import { http } from './client'
import type { Payment } from '@/types'

/** 一行核销：还的是哪张单、还多少（原币）。 */
export interface PaymentAllocationInput {
  /** 10 应收单 / 20 应付单，必须与方向匹配；收款只能核应收。 */
  target_type: number
  target_id: number
  /** 定点字符串（后端 money.Decimal）：算术一律在服务端。 */
  amount: string
}

/**
 * 收付款写入口。
 *
 * 没有 exchange_rate：汇率取**到账日**的，由后端查汇率表。允许前端传，就等于
 * 允许把汇兑损益提前抹平——那笔差额一旦记成 0，报表里再也找不回来。
 *
 * partner_id / currency_code 可留空（从核销目标带出），amount 留空表示取核销合计；
 * 只有不核销的预收 / 预付款才必须显式给这三样。
 */
export interface PaymentInput {
  /** 10 收款 / 20 付款。 */
  direction: number
  partner_id?: number
  /** 留空取今天。 */
  pay_date?: string
  /** 留空随核销目标的币种，没有核销目标时用本币。 */
  currency_code?: string
  /** 留空（或 "0"）= 取核销合计。 */
  amount?: string
  method?: string
  reference?: string
  remark: string
  allocations?: PaymentAllocationInput[]
}

/**
 * 编辑只改不涉及金额的字段。
 *
 * 金额与核销明细不给改：改一行核销要先回滚它再重算目标单据的状态，
 * 而「录错一笔」的真实操作是删掉重录——与汇率录错删掉重录同一条约定。
 */
export interface PaymentUpdateInput {
  pay_date?: string
  method?: string
  reference?: string
  remark: string
}

export const paymentApi = {
  /**
   * 撤销单条核销（T-52）。
   *
   * 只回滚这一行：目标单据的已核销额减回去，收付款单本身与其它核销行都不动。
   * 在此之前撤销核销只能删整张单——一笔录错，同单上其它正确的核销也一起消失。
   */
  removeAllocation: (paymentId: number, allocationId: number) =>
    http.delete<Payment>(`/payments/${paymentId}/allocations/${allocationId}`),

  create: (body: PaymentInput) => http.post<Payment>('/payments', body),
  update: (id: number, body: PaymentUpdateInput) => http.put<Payment>(`/payments/${id}`, body),
  remove: (id: number) => http.delete<{ deleted: boolean }>(`/payments/${id}`),
}
