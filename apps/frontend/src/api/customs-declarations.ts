import { http } from './client'
import type { CustomsDeclaration } from '@/types'

/**
 * 报关单写入口。
 *
 * 与运费杂费同一套做法：列表走 dataProvider（面板按 shipment_id 永久筛选），
 * 写入走这里 —— 表单要的是"提交哪些字段"的明确契约。
 */
export interface CustomsDeclarationInput {
  shipment_id: number
  /** 10 待申报 / 20 已申报 / 30 已放行 / 90 已退单 */
  status: number
  /** 海关给的号，待申报时可以为空。 */
  decl_no?: string
  /** YYYY-MM-DD，留空表示还没申报。 */
  decl_date?: string
  supervision_mode?: string
  remark: string
}

export const customsDeclarationApi = {
  create: (body: CustomsDeclarationInput) =>
    http.post<CustomsDeclaration>('/customs-declarations', body),

  // 不能改挂到另一票货（后端会拒），shipment_id 只做校验用。
  update: (id: number, body: CustomsDeclarationInput) =>
    http.put<CustomsDeclaration>(`/customs-declarations/${id}`, body),

  remove: (id: number) => http.delete<{ deleted: boolean }>(`/customs-declarations/${id}`),
}
