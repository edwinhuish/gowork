import { http } from './client'
import type { Contract } from '@/types'

/**
 * 合同的动作入口。
 *
 * 创建与编辑走 refine 的 dataProvider（POST /contracts、PUT /contracts/:id），
 * 这里只放非 CRUD 的状态变更：它和"改条款"是两件事，
 * 混进 PUT 里就会出现"保存一下顺手把合同确认了"。
 */
export const contractApi = {
  /** 只接受 20（已确认）/ 90（已取消）。 */
  updateStatus: (id: number, status: number) =>
    http.put<Contract>(`/contracts/${id}/status`, { status }),
}
