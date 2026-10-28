import type {
  BaseRecord,
  CreateParams,
  CrudFilters,
  DataProvider,
  DeleteOneParams,
  GetListParams,
  GetOneParams,
  UpdateParams,
} from '@refinedev/core'

import { http, toQuery } from '@/api/client'
import type { PageResult } from '@/types'

/**
 * 后端分页与筛选契约（见 apps/backend/internal/dto/common.go 与各资源 Query）：
 *   page / page_size / keyword 全资源通用
 *   category+supplier_id+status 仅产品
 *   country+status+role+entity_type 仅往来单位（/partners 及其 customers/suppliers 别名）
 *   currency_code 仅汇率；币种本身只用 status + keyword
 *   partner_id 仅联系人 / 跟进记录 / 产品货源（按主体看是 CRM 的主维度，货源侧是反向查）
 *   product_id 仅产品货源（产品详情页的货源面板按产品取数）
 *   due 仅跟进记录（=1 时只取已到期未跟进，判定在后端按整天算）
 *
 * refine 用 CrudFilters 表达筛选条件，这里翻译成后端认可的同名查询参数。
 * 白名单是有意的：不认识的条件直接丢弃，而不是拼进 URL 让后端静默忽略——
 * 后者会在筛选没生效时表现成「点了没反应」，很难查。
 */
const PASSTHROUGH_FILTERS = new Set([
  'category',
  'supplier_id',
  'status',
  'country',
  'role',
  'entity_type',
  'currency_code',
  'partner_id',
  'product_id',
  'due',
  'stage',
  'open_only',
  'source',
])

function buildListQuery({ pagination, filters }: GetListParams): Record<string, unknown> {
  const query: Record<string, unknown> = { page: 1, page_size: 15 }

  if (pagination?.mode === 'off') {
    // 下拉选项场景（useSelect 配 { mode: 'off' }）：需要一次取满。
    // 否则只拿到第一页，供应商超过 15 个时下拉就开始丢项，而且不报错。
    // 200 是后端 dto.MaxPageSize 的上限。
    query.page_size = 200
  } else if (pagination) {
    query.page = pagination.currentPage ?? 1
    query.page_size = pagination.pageSize ?? 15
  }

  for (const filter of (filters ?? []) as CrudFilters) {
    // 逻辑组合子（and/or）不往后端传，当前接口没有对应的表达能力。
    if (!('field' in filter) || !('value' in filter)) continue

    const { field, value } = filter
    if (field === 'keyword') {
      query.keyword = value as string
      continue
    }
    if (PASSTHROUGH_FILTERS.has(field)) {
      query[field] = value
    }
  }

  return query
}

/**
 * REST dataProvider。
 *
 * 之所以不用 @refinedev/simple-rest：本项目的响应体是 {code,message,data} 统一包裹，
 * 分页参数是 page/page_size 而非 _start/_end，且 401 需要触发刷新令牌与登出。
 * 复用现有的 http 封装（api/client.ts）就同时拿到了拆包、令牌注入、续期和 401 处理。
 *
 * 末尾的 as unknown as DataProvider 是必要的：DataProvider 的每个方法都是泛型
 * （返回值随调用方的 TData 变），而这里的实现返回运行时确定的结构，无法在类型上
 * 对每个 TData 都成立。逐个方法断言反而更难读。
 */
export const dataProvider = {
  getList: async ({ resource, pagination, filters }: GetListParams) => {
    const res = await http.get<PageResult<BaseRecord>>(
      `/${resource}${toQuery(buildListQuery({ resource, pagination, filters } as GetListParams))}`,
    )
    return { data: res.list, total: res.total }
  },

  getOne: async ({ resource, id }: GetOneParams) => {
    const data = await http.get<BaseRecord>(`/${resource}/${id}`)
    return { data }
  },

  create: async ({ resource, variables }: CreateParams) => {
    const data = await http.post<BaseRecord>(`/${resource}`, variables)
    return { data }
  },

  update: async ({ resource, id, variables }: UpdateParams) => {
    const data = await http.put<BaseRecord>(`/${resource}/${id}`, variables)
    return { data }
  },

  deleteOne: async ({ resource, id }: DeleteOneParams) => {
    await http.delete<{ deleted: boolean }>(`/${resource}/${id}`)
    // 后端只回 {deleted:true}，不回被删记录。refine 需要一条 data 用来做失效匹配，
    // 用 id 兜底即可（列表随后会整体重查）。
    return { data: { id } as unknown as BaseRecord }
  },

  getApiUrl: () => '/api/v1',
} as unknown as DataProvider
