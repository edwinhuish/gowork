import { http } from './client'
import type { Partner } from '@/types'

/**
 * 往来单位（客户 / 供应商 / 货代是同一张表）里只做恢复。
 *
 * 列表与增删改走 refine 的 dataProvider，这里只补它表达不了的那一个动作。
 * 恢复只认 id，所以三个角色入口都调同一个路径，不按角色再分一套。
 */
export const partnerApi = {
  /**
   * 恢复一条已软删的主体。
   *
   * 编码已被别的记录占用时后端返回 409，提示里会说明是哪个编码；
   * 恢复不会把删除期间被摘掉的角色或联系方式补回来。
   */
  restore: (id: number) => http.post<Partner>(`/partners/${id}/restore`),
}
