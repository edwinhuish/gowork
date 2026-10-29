import { http, toQuery } from './client'
import type { Partner, PartnerDuplicateCandidate } from '@/types'

export interface PartnerDuplicateQuery {
  name?: string
  country?: string
  email?: string
  phone?: string
  /** 编辑时排除自身。不传的话每条记录都会命中自己。 */
  exclude_id?: number
}

/**
 * 往来单位（客户 / 供应商 / 货代是同一张表）里只做 refine 的 dataProvider 表达不了的动件。
 *
 * 列表与增删改走 dataProvider；恢复与查重在这里。
 */
export const partnerApi = {
  /**
   * 恢复一条已软删的主体。
   *
   * 编码已被别的记录占用时后端返回 409，提示里会说明是哪个编码；
   * 恢复不会把删除期间被摘掉的角色或联系方式补回来。
   */
  restore: (id: number) => http.post<Partner>(`/partners/${id}/restore`),

  /**
   * 录入时查重：只提示不阻断，保存与否由人决定。
   *
   * 挂在 /partner-duplicates 而不是 /partners/duplicates：后端 gin 的路由树
   * 不允许同一层既有 :id 又有静态段（会 panic），与 /shipment-candidates 同一处理。
   * 编辑时传 exclude_id，否则每条记录都会命中自己。
   */
  duplicateCandidates: (q: PartnerDuplicateQuery) =>
    http.get<PartnerDuplicateCandidate[]>(`/partner-duplicates/candidates${toQuery({ ...q })}`),
}
