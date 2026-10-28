import { http } from './client'
import type { CreditSummary } from '@/types'

/**
 * 客户信用额度。
 *
 * 只有读：额度本身是主体上的商务条件，改它走 PUT /partners/:id。
 * 拦截发生在订单确认那一步（后端 409），这里返回的数字是用来"提前看见"的。
 */
export const creditApi = {
  summary: (partnerId: number) => http.get<CreditSummary>(`/partners/${partnerId}/credit`),
}
