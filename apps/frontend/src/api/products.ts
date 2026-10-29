import { http, toQuery } from './client'
import type { PageResult, Product } from '@/types'

/**
 * 产品远程检索。
 *
 * 只读两个用途：下拉按输入实时搜产品、编辑单据时按 id 补一条回显。
 * 列表与增删改仍然走 refine 的 dataProvider —— 那套已经处理了分页与筛选的翻译。
 *
 * 为什么不给下拉用 useSelect / useList：它们只取一次全量（page_size=200），
 * 产品过两百条之后后半段就选不到，而且输入关键词只能在前端已取到的 200 条里过滤。
 * 客户报一个编号来问价，恰恰是需要后端按编号查的时候。
 */
export const productApi = {
  search: (keyword: string, pageSize = 20) =>
    http.get<PageResult<Product>>(
      `/products${toQuery({ keyword, page_size: pageSize, status: 1 })}`,
    ),

  get: (id: number) => http.get<Product>(`/products/${id}`),

  /**
   * 恢复一条已软删的产品。
   *
   * 只把删除标记去掉，不会把删除时清掉的编号带回来——产品下线那段时间里
   * 客户可能已经把那个号挪给了别的货，静默恢复等于替用户断言归属。
   * SKU 已被别的产品占用时后端返回 409，提示里会说明是哪个号。
   */
  restore: (id: number) => http.post<Product>(`/products/${id}/restore`),
}
