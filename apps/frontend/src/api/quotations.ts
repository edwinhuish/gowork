import { http } from './client'
import type { QuotationDetail, QuotationPreview } from '@/types'

/**
 * 报价单写入口。
 *
 * 明细里没有金额字段：金额、成本、毛利一律由后端算。
 * 前端能提交的只有「数量 × 单价 × 指哪个货源」，避免了
 * "改了明细没改总额"这种在任何系统里都真实发生过的事。
 */
export interface QuotationItemInput {
  product_id: number
  qty: number
  unit_price: number
  /** 留空取该产品本币价最低的启用货源。 */
  source_id?: number
  remark?: string
}

export interface QuotationInput {
  partner_id: number
  trade_term: string
  /** 留空按本币。 */
  currency_code?: string
  /** YYYY-MM-DD，含当天。 */
  valid_until: string
  remark: string
  items: QuotationItemInput[]
}

export const quotationApi = {
  create: (body: QuotationInput) => http.post<QuotationDetail>('/quotations', body),

  update: (id: number, body: QuotationInput) =>
    http.put<QuotationDetail>(`/quotations/${id}`, body),

  remove: (id: number) => http.delete<{ deleted: boolean }>(`/quotations/${id}`),

  /** 只接受 20（已确认）/ 90（已取消）。 */
  updateStatus: (id: number, status: number) =>
    http.put<QuotationDetail>(`/quotations/${id}/status`, { status }),

  /**
   * 另存新版本：单号不变、版本 +1，回到草稿。
   * 已过期的报价单必须给新的有效期，否则新版存下来就确认不了。
   */
  newVersion: (id: number, validUntil?: string) =>
    http.post<QuotationDetail>(`/quotations/${id}/versions`, {
      valid_until: validUntil ?? '',
    }),

  /** 试算：不落库，只算金额、成本与毛利。 */
  preview: (body: { currency_code?: string; items: QuotationItemInput[] }) =>
    http.post<QuotationPreview>('/quotations-preview', body),
}
