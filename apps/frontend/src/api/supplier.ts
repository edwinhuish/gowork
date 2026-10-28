import { http, toQuery } from './client'
import type { PageResult, Supplier } from '../types'

export interface SupplierQuery {
  page?: number
  page_size?: number
  keyword?: string
  status?: number
}

export interface SupplierInput {
  code: string
  name: string
  contact?: string
  phone?: string
  email?: string
  address?: string
  status?: number
  remark?: string
}

export const supplierApi = {
  list: (q: SupplierQuery) =>
    http.get<PageResult<Supplier>>(
      `/suppliers${toQuery(q as unknown as Record<string, unknown>)}`,
    ),
  create: (input: SupplierInput) => http.post<Supplier>('/suppliers', input),
  update: (id: number, input: SupplierInput) => http.put<Supplier>(`/suppliers/${id}`, input),
  remove: (id: number) => http.delete<{ deleted: boolean }>(`/suppliers/${id}`),
}
