import { http, toQuery } from './client'
import type { PartnerImportReport } from '@/types'

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

/**
 * 主体导入（T-35）。
 *
 * dryRun=true 只出报告、不落库：先让人看清"哪几行会被跳过"，改完再真导。
 * 文件就是 `/exports/partners` 导出的那份，改完另存 CSV 即可。
 */
export const importApi = {
  partners: (file: File, dryRun: boolean) => {
    const form = new FormData()
    form.append('file', file)
    return http.postForm<PartnerImportReport>(
      `/imports/partners${toQuery({ dry_run: dryRun })}`,
      form,
    )
  },
}
