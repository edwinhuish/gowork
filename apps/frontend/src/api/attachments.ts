import { http, toQuery } from './client'
import type { Attachment } from '@/types'

/** 附件能挂在哪：取值与后端 model.Attach* 常量一一对应。 */
export type AttachmentEntity =
  | 'partner'
  | 'product'
  | 'quotation'
  | 'contract'
  | 'sales_order'
  | 'purchase_order'
  | 'shipment'
  | 'customs_declaration'
  | 'receivable'
  | 'payable'
  | 'payment'
  | 'stock_document'

/**
 * 附件读写。
 *
 * 没有 update：换文件等于重新传一份再删旧的——就地覆盖会让"这版是客户签的那版吗"
 * 变得无法回答，而附件的价值恰恰在于它当时就是那个样子。
 */
export const attachmentApi = {
  list: (entityType: AttachmentEntity, entityId: number) =>
    http.get<Attachment[]>(
      `/attachments${toQuery({ entity_type: entityType, entity_id: entityId })}`,
    ),

  /**
   * 上传一个文件。
   *
   * 文件与"挂在哪"一次提交：先建记录再传内容的话，中间断掉会留下一条
   * 看上去完全正常、点开却打不开的附件。
   */
  upload: (entityType: AttachmentEntity, entityId: number, file: File, remark?: string) => {
    const form = new FormData()
    form.append('entity_type', entityType)
    form.append('entity_id', String(entityId))
    form.append('remark', remark ?? '')
    form.append('file', file)
    return http.postForm<Attachment>('/attachments', form)
  },

  /** 下载路径交给 client 的 download()：它负责带令牌与按响应头命名。 */
  downloadPath: (id: number) => `/attachments/${id}/download`,

  remove: (id: number) => http.delete<{ deleted: boolean }>(`/attachments/${id}`),
}
