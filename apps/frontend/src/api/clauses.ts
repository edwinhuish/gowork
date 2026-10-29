import { http } from './client'
import type { Clause } from '@/types'

export interface ClauseInput {
  code: string
  title: string
  body: string
  category?: string
  /** 留空按启用处理。 */
  status?: number
  remark?: string
}

/**
 * 条款模板库（T-27）。
 *
 * 列表走 refine 的 `useTable`（resource = `clauses`），这里只放写操作：
 * 模板的增改删是主数据维护，与合同的状态流转不是一回事。
 */
export const clauseApi = {
  create: (input: ClauseInput) => http.post<Clause>('/clauses', input),
  update: (id: number, input: ClauseInput) => http.put<Clause>(`/clauses/${id}`, input),
  remove: (id: number) => http.delete<{ id: number }>(`/clauses/${id}`),
  restore: (id: number) => http.post<Clause>(`/clauses/${id}/restore`),
}
