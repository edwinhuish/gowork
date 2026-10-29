import { http, toQuery } from './client'

/**
 * 列表导出（T-35）。
 *
 * 导出走 `GET /exports/...`，参数与各自列表一致；后端按 200 一页把结果翻完
 * （上限 10000 行），所以它比列表慢一点，但拿到的是同一批数据、同一套数据范围。
 */
export const exportApi = {
  partners: (role: number) =>
    http.download(`/exports/partners${toQuery({ role })}`, 'partners.csv'),
  products: () => http.download('/exports/products', 'products.csv'),
}
