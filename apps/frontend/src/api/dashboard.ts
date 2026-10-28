import { http } from './client'
import type { DashboardSummary } from '../types'

export const dashboardApi = {
  summary: () => http.get<DashboardSummary>('/dashboard/summary'),
}
