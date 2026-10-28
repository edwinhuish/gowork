import { http } from './client'
import type { StockDocument } from '@/types'

/**
 * 出入库单写入口。
 *
 * 新建 / 编辑走 dataProvider；这里补"确认"这个动宾接口——
 * 它是唯一会动库存的动作，重复确认返回 409（不是幂等的状态设置）。
 */
export const stockDocApi = {
  confirm: (id: number) => http.post<StockDocument>(`/stock-documents/${id}/confirm`),
}
