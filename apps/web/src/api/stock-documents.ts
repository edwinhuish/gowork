import { http } from './client'
import type { StockDocument } from '@/types'

/**
 * 出入库单写入口。
 *
 * 新建 / 编辑走 dataProvider；这里补"确认"这个动宾接口——
 * 它是唯一会动库存的动作，重复确认返回 409（不是幂等的状态设置）。
 */
export const stockDocApi = {
  /**
   * 红冲（T-52c）：生成一张方向相反的单据并立即确认，库存回到原单确认前。
   * 原单保留，红冲单的 reversal_of_id 指向它。
   */
  reverse: (id: number) => http.post<{ doc_no: string }>(`/stock-documents/${id}/reverse`),
  confirm: (id: number) => http.post<StockDocument>(`/stock-documents/${id}/confirm`),
}
