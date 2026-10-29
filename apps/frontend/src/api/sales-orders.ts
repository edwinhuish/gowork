import { http } from './client'
import type { SalesOrder, SalesOrderChange } from '@/types'

/**
 * 销售订单写入口。
 *
 * 入参里**没有** amount_total / amount_base_total：合计由后端从明细汇总，
 * 前端即使把总额塞进请求体也会被丢弃。这样"改了明细没改总额"在结构上就不可能发生。
 */
export interface SalesOrderItemInput {
  product_id: number
  qty: string
  unit_price: number
  remark?: string
}

export interface SalesOrderInput {
  partner_id: number
  trade_term: string
  currency_code?: string
  /** YYYY-MM-DD，留空取今天。 */
  order_date?: string
  /** YYYY-MM-DD，可留空；不能早于下单日。 */
  delivery_date?: string
  remark: string
  items: SalesOrderItemInput[]
}

/**
 * 变更单里一行订单明细的新值。
 *
 * 是**目标值**而不是增量：客户说"改成 800"就填 800，事后核对不用拿计算器加一遍。
 */
export interface SalesOrderChangeLineInput {
  sales_order_item_id: number
  qty: string
  unit_price: string
}

/**
 * 一次订单变更。
 *
 * 只有已确认 / 备货中的订单走这里；草稿直接编辑，已出货之后的改动属于另一笔生意。
 * 不含增删行：出货分配与以销定采按明细行 id 引用，增删会让引用指向不存在的行。
 */
export interface SalesOrderChangeInput {
  /** 必填：变更记录的价值全在"为什么改"。 */
  reason: string
  /**
   * YYYY-MM-DD。**省略**表示这次不动交期；带空串表示把交期清掉。
   * 两种意思必须分开，所以这个字段只在交期真的变了时才出现。
   */
  delivery_date?: string
  /** 只提交变了的行；空数组表示这次只改交期。 */
  lines: SalesOrderChangeLineInput[]
  /** 打开页面时读到的订单版本，用于条件更新；别人先改过会拿到 409。 */
  updated_at?: string
}

export const salesOrderApi = {
  create: (body: SalesOrderInput) => http.post<SalesOrder>('/sales-orders', body),

  update: (id: number, body: SalesOrderInput) =>
    http.put<SalesOrder>(`/sales-orders/${id}`, body),

  remove: (id: number) => http.delete<{ deleted: boolean }>(`/sales-orders/${id}`),

  /**
   * 状态流转走服务端状态机，跳步会被拒（409）。
   *
   * forceCredit 只在确认（10→20）时有意义：越过客户信用额度。
   * 前端勾选只是表达意图，后端还会看当前登录身份是不是 admin。
   */
  updateStatus: (id: number, status: number, forceCredit = false) =>
    http.put<SalesOrder>(`/sales-orders/${id}/status`, {
      status,
      force_credit: forceCredit,
    }),

  /** 变更历史，按序号倒序：最近一次改了什么，是翻旧账时的第一个问题。 */
  listChanges: (id: number) => http.get<SalesOrderChange[]>(`/sales-orders/${id}/changes`),

  /**
   * 提交一次变更：订单更新到新值，同时留下改前改后与原因。
   *
   * key 是幂等键（T-46）：这个接口每次调用都会**新建一条变更记录**，
   * 网络抖动后用户再点一次就会多出一条永远解释不清的变更单。
   * 提交成功后由调用方换新键；失败时保持不变，重试才会命中去重。
   */
  change: (id: number, body: SalesOrderChangeInput, key?: string) =>
    http.post<SalesOrderChange>(
      `/sales-orders/${id}/changes`,
      body,
      key ? { 'Idempotency-Key': key } : undefined,
    ),
}
