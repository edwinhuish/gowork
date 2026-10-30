import { http } from './client'
import type { ProductImage } from '@/types'

/**
 * 产品图片读写。
 *
 * 排序提交的是**完整排列**，不是「上移一格」：后者要求客户端与库里的位置
 * 严格对齐，两个人同时拖动就会错位；全量提交时集合对不上，后端直接 409。
 *
 * 取图路径（url / thumb_url）由后端回填，前端不拼：路由或版本号规则变了
 * 只需改服务端一处。
 */
export const productImageApi = {
  list: (productId: number) => http.get<ProductImage[]>(`/products/${productId}/images`),

  /**
   * 一次传多张（后端单批上限 4 张，前端负责分批）。
   *
   * 字段名是 files（复数），与 handler 的 form.File["files"] 对应；
   * 写成 file 会得到一个「请选择要上传的图片」的 400。
   */
  upload: (productId: number, files: File[]) => {
    const form = new FormData()
    files.forEach((file) => form.append('files', file))
    return http.postForm<ProductImage[]>(`/products/${productId}/images`, form)
  },

  /** 替换内容，保留位置：换的是外观，不是位置。 */
  replace: (productId: number, imageId: number, file: File) => {
    const form = new FormData()
    form.append('file', file)
    return http.putForm<ProductImage>(`/products/${productId}/images/${imageId}`, form)
  },

  /** ids 的先后即最终顺序。 */
  reorder: (productId: number, ids: number[]) =>
    http.put<ProductImage[]>(`/products/${productId}/images/order`, { ids }),

  remove: (productId: number, imageId: number) =>
    http.delete<{ deleted: boolean }>(`/products/${productId}/images/${imageId}`),
}
