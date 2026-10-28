import { http, toQuery } from './client'
import type { PageResult, Product } from '../types'

export interface ProductQuery {
  page?: number
  page_size?: number
  keyword?: string
  category?: string
  status?: number
}

export interface ProductInput {
  sku_code: string
  name_cn: string
  name_en?: string
  category?: string
  spec?: string
  unit?: string
  supplier_id?: number | null
  cost_price: number
  sale_price: number
  status?: number
  remark?: string
}

export const productApi = {
  list: (q: ProductQuery) =>
    http.get<PageResult<Product>>(
      `/products${toQuery(q as unknown as Record<string, unknown>)}`,
    ),
  get: (id: number) => http.get<Product>(`/products/${id}`),
  create: (input: ProductInput) => http.post<Product>('/products', input),
  update: (id: number, input: ProductInput) => http.put<Product>(`/products/${id}`, input),
  remove: (id: number) => http.delete<{ deleted: boolean }>(`/products/${id}`),
}
