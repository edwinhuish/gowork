import { http, toQuery } from './client'
import type { ProductCode } from '@/types'

/** 新增编号的入参。没有 code_norm：归一化形态由服务端算。 */
export interface ProductCodeInput {
  product_id: number
  code: string
  /** sku / customer / supplier / factory / other，留空按 other。 */
  source?: string
  remark?: string
}

/**
 * 产品编号（SKU 之外的别名）读写。
 *
 * 没有 update：编号改了就是另一个号，改原文会让人以为「客户换了叫法」，
 * 而实际是录入时打错了。要改就删掉重录。
 */
export const productCodeApi = {
  list: (productId: number) =>
    http.get<ProductCode[]>(`/product-codes${toQuery({ product_id: productId })}`),

  create: (body: ProductCodeInput) => http.post<ProductCode>('/product-codes', body),

  remove: (id: number) => http.delete<{ deleted: boolean }>(`/product-codes/${id}`),
}
