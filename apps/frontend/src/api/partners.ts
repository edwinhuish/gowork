import { http, toQuery } from './client'
import type {
  Partner,
  PartnerAddress,
  PartnerAddressType,
  PartnerDuplicateCandidate,
  PartnerMergeResult,
} from '@/types'

/** 地址新增与编辑共用的入参。编辑是全量覆盖（PUT 语义）。 */
export interface PartnerAddressInput {
  type: PartnerAddressType
  is_default: boolean
  contact?: string
  phone?: string
  country?: string
  province?: string
  city?: string
  postal_code?: string
  address: string
  remark?: string
}

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

  /**
   * 把 sourceId 并进 survivorId：引用整体迁过去，被合并方软删。
   *
   * 方向写死在参数名里——"哪条留下"是最容易点反的一步，而它不可逆。
   * updatedAt 是主记录打开时的版本，走乐观锁；不传则按普通更新处理。
   */
  merge: (survivorId: number, sourceId: number, updatedAt?: string) =>
    http.post<PartnerMergeResult>(`/partners/${survivorId}/merge`, {
      source_id: sourceId,
      updated_at: updatedAt,
    }),

  /**
   * 转移负责人（T-32）。ownerId 传 0 表示退回**未分配**（公共池）。
   *
   * 只有管理员与主管有权调用；业务员拿到 403。updatedAt 走乐观锁，
   * 别把刚看过的旧版本覆盖掉别人的改动。
   */
  transferOwner: (id: number, ownerId: number, updatedAt?: string) =>
    http.put<Partner>(`/partners/${id}/owner`, {
      owner_id: ownerId,
      updated_at: updatedAt,
    }),

  /**
   * 多地址（T-49）。走 /partners/:id/addresses，不给客户 / 供应商各复制一份：
   * 地址没有角色语义，能打开这个主体就能看它的地址。
   *
   * 默认地址的唯一性由后端保证：新增时若该用途还没有地址会自动置默认，
   * 把另一条设为默认会把原默认降级，取消唯一一条默认会被拒绝（400）。
   */
  addresses: {
    list: (partnerId: number, type?: PartnerAddressType) =>
      http.get<PartnerAddress[]>(`/partners/${partnerId}/addresses${toQuery({ type })}`),
    create: (partnerId: number, input: PartnerAddressInput) =>
      http.post<PartnerAddress>(`/partners/${partnerId}/addresses`, input),
    update: (partnerId: number, addressId: number, input: PartnerAddressInput) =>
      http.put<PartnerAddress>(`/partners/${partnerId}/addresses/${addressId}`, input),
    remove: (partnerId: number, addressId: number) =>
      http.delete<void>(`/partners/${partnerId}/addresses/${addressId}`),
  },
}
