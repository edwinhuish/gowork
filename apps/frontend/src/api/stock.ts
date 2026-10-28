import { http } from './client'
import type { StockRecalcResult } from '@/types'

/**
 * 库存写入的唯一入口是"重算"。
 *
 * 列表与新建走 dataProvider（`stock-snapshots` / `stock-movements`），
 * 这里只补一个 dataProvider 表达不了的动宾接口。
 *
 * 没有"设置库存"：库存只能由流水推出来，能改它的只有流水。
 */
export const stockApi = {
  /** productId 留空表示重算全部有流水的产品。 */
  recalc: (productId?: number) =>
    http.post<StockRecalcResult>('/stock-snapshots/recalc', { product_id: productId }),
}
