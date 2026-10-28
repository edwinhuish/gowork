import { http } from './client'
import type { Lead, Partner } from '@/types'

/**
 * 线索转化：建主体 + 把线索置为已转化，两件事在服务端一次完成。
 *
 * 不能拆成"先建客户、再改线索状态"两次调用：中间失败会留下一条
 * 「已转化的线索没有主体」或「主体建了但线索还在跟」的脏数据。
 */
export const leadApi = {
  convert: (id: number, body: { code: string; role?: number }) =>
    http.post<{ lead: Lead; partner: Partner; warning?: string }>(`/leads/${id}/convert`, body),
}
