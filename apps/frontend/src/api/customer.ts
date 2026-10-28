import { http, toQuery } from './client'
import type { Customer, PageResult } from '../types'

export interface CustomerQuery {
  page?: number
  page_size?: number
  keyword?: string
  country?: string
  status?: number
}

export interface CustomerInput {
  code: string
  name: string
  country?: string
  contact?: string
  phone?: string
  email?: string
  address?: string
  status?: number
  remark?: string
}

export const customerApi = {
  list: (q: CustomerQuery) =>
    http.get<PageResult<Customer>>(
      `/customers${toQuery(q as unknown as Record<string, unknown>)}`,
    ),
  create: (input: CustomerInput) => http.post<Customer>('/customers', input),
  update: (id: number, input: CustomerInput) => http.put<Customer>(`/customers/${id}`, input),
  remove: (id: number) => http.delete<{ deleted: boolean }>(`/customers/${id}`),
}
