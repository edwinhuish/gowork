import { http, toQuery } from './client'
import type { Opportunity, OpportunityStageLog, OpportunitySummary } from '@/types'

/**
 * 商机汇总。
 *
 * 走独立路径 `/opportunities-summary` 而不是 `/opportunities/summary`：
 * 后者会与 `/opportunities/:id` 抢同一段路径（见后端 handler 注释）。
 */
export const opportunityApi = {
  summary: (params: { partner_id?: number; open_only?: number } = {}) =>
    http.get<OpportunitySummary>(`/opportunities-summary${toQuery(params)}`),
  /**
   * 按今日汇率重估本币金额（T-61）。
   *
   * 保存商机不重算本币金额——汇率一漂，上一季的加权预测就跟着漂。
   * 要按今天的汇率重新看一遍，点这个显式动作。终态商机返回 409。
   */
  revalue: (id: number) => http.post<Opportunity>(`/opportunities/${id}/revalue`),
  /**
   * 阶段变更历史（T-61）：什么时间从哪一步走到哪一步、谁改的。
   * 只读——历史改不了，要更正只能再写一条。
   */
  stageLogs: (id: number) => http.get<OpportunityStageLog[]>(`/opportunities/${id}/stage-logs`),
}
