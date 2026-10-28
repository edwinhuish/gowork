import { http, toQuery } from './client'
import type { OpportunitySummary } from '@/types'

/**
 * 商机汇总。
 *
 * 走独立路径 `/opportunities-summary` 而不是 `/opportunities/summary`：
 * 后者会与 `/opportunities/:id` 抢同一段路径（见后端 handler 注释）。
 */
export const opportunityApi = {
  summary: (params: { partner_id?: number; open_only?: number } = {}) =>
    http.get<OpportunitySummary>(`/opportunities-summary${toQuery(params)}`),
}
