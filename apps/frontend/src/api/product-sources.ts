import { http } from './client'
import type { ProductSource } from '@/types'

/**
 * 货源写入口。
 *
 * 列表走 dataProvider（useTable 直接翻译查询参数），写入走这里：
 * 表单要的是"提交哪些字段"这个明确契约，而不是任何一个 refine 钩子在
 * 当前版本里返回什么形状的 mutation 对象。
 */
export interface ProductSourceInput {
  product_id: number
  partner_id: number
  supplier_sku: string
  /** 后端 money.Decimal：字符串收发，避免过一遍 JS number 丢精度。 */
  purchase_price: string
  /** 留空即本币，由后端折算并回填本币价与汇率。 */
  currency_code?: string
  moq: number
  delivery_days: number
  status: number
  remark: string
}

export const productSourceApi = {
  create: (body: ProductSourceInput) => http.post<ProductSource>('/product-sources', body),

  // 产品与供应商不可改（后端会拒），所以入参里的这两项只做校验用。
  update: (id: number, body: ProductSourceInput) =>
    http.put<ProductSource>(`/product-sources/${id}`, body),

  remove: (id: number) => http.delete<{ deleted: boolean }>(`/product-sources/${id}`),
}
