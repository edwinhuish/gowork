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
 *   sort_field + sort_order 目前只有产品列表消费（白名单在 repository/product.go）
 *   category+supplier_id 仅产品
 *   country+role+entity_type 仅往来单位（/partners 及其 customers/suppliers 别名）
 *   currency_code 仅汇率
 *   status 是两处残留下来的：币种的归档标记与账号的启用 / 禁用。
 *     其余主数据的启用状态已经拿掉，那些资源的后端不再认识这个参数。
 *   partner_id 仅联系人 / 跟进记录 / 产品货源（按主体看是 CRM 的主维度，货源侧是反向查）
 *   product_id 仅产品货源（产品详情页的货源面板按产品取数）
 *   shipment_id 仅运费杂费（出货详情页的费用面板按票取数）
 *   source_type / source_id 仅应收应付（查"这张订单开了几张票"）
 *   from / to 仅库存流水（业务发生日 YYYY-MM-DD，含两端；对账要按时间段看流水）
 *   due 仅跟进记录（=1 时只取已到期未跟进，判定在后端按整天算）
 *   type 库存流水（model.StockMovementType）与运费杂费（model.ShipmentChargeType）共用
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
  // 批次追溯（T-58）：给定批号列出它全部的出入流水。
  'batch_no',
  'shipment_id',
  // 采购跟单节点逾期筛选（T-38）：筛"这个节点已逾期"的采购单。
  'overdue_node',
  // 出货列表按订单筛（「这张订单出了几次货」）。后端 dto.ShipmentQuery 早就支持，
  // 白名单漏了它，界面传过去会被静默丢弃（T-43）。
  'sales_order_id',
  'source_type',
  'source_id',
  'from',
  'to',
  'due',
  // 产品列表的「含已删除」开关：只有这一步能让软删的行出现在列表里。
  'include_deleted',
  // 下面四个是 T-63 补上的：后端 dto 里早就有，前端漏了，界面传过去被静默丢弃，
  // 表现是「勾了/选了但列表没变」——比报错更难查。
  //   owner_id  ：主体列表按负责人筛（主管要把某人的客户拉出来看）
  //   direction ：收付款与汇兑损益报表按收/付方向筛
  //   overdue   ：应收应付只看到期未结的
  //   base_code ：汇率列表换一个基准币种看
  'owner_id',
  'direction',
  'overdue',
  'base_code',
  'stage',
  'open_only',
  'source',
  // 库存流水的类型（期初 / 盘盈亏 / 其它…）。名字很泛，但目前只有这一个资源用它。
  'type',
])

function buildListQuery({ pagination, filters, sorters }: GetListParams): Record<string, unknown> {
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

  // 排序只取第一个：后端各资源的排序接口都只有一个 ORDER BY 字段，
  // 多字段排序传过去会被后者覆盖，结果与界面上显示的两个箭头不一致。
  // 目前只有产品列表实现了 sort_field / sort_order，其余资源不认识这两个参数
  // （Go 的 form binding 会直接忽略），不会报错，也不会改变它们原本的顺序。
  const first = sorters?.[0]
  if (first?.field) {
    query.sort_field = String(first.field)
    query.sort_order = first.order ?? 'asc'
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
  getList: async ({ resource, pagination, filters, sorters }: GetListParams) => {
    const res = await http.get<PageResult<BaseRecord>>(
      `/${resource}${toQuery(
        buildListQuery({ resource, pagination, filters, sorters } as GetListParams),
      )}`,
    )
    // 列表接口统一返回 {list,total}；个别接口只给数组（/warehouses 曾经就是这样）。
    // 直接取 .list 会拿到 undefined —— refine 的 useSelect 内部 data.data.map
    // 抛错，React 19 把整棵树卸载，表现是整页白屏（出货单 / 出入库单新建页实测）。
    // 两种形状都认：最坏是空列表，不是白屏。
    const list = Array.isArray(res) ? res : (res?.list ?? [])
    const total = Array.isArray(res) ? list.length : (res?.total ?? list.length)
    return { data: list, total }
  },

  getOne: async ({ resource, id }: GetOneParams) => {
    const data = await http.get<BaseRecord>(`/${resource}/${id}`)
    return { data }
  },

  create: async ({ resource, variables, meta }: CreateParams) => {
    // 幂等键由页面提供（见 hooks/use-idempotency-key）：同一次提交的重试必须复用它，
    // 否则"超时后又点一次保存"会真的建出两条单。没传就不带，退化成普通请求。
    const key = (meta as { idempotencyKey?: string } | undefined)?.idempotencyKey
    const data = await http.post<BaseRecord>(
      `/${resource}`,
      variables,
      key ? { 'Idempotency-Key': key } : undefined,
    )
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
