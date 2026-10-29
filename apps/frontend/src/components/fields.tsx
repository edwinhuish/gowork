import { useList } from '@refinedev/core'
import { Space, Tag } from 'antd'

import { categoryLabel, PARTNER_ROLE } from '@/constants/catalog'
import type { Currency } from '@/types'

/**
 * 业务字段的统一展示件。
 *
 * 三个主数据模块里「状态」「金额」「类别」出现十几次，改造前每个页面各写一遍
 * 渲染函数，停用色和金额小数位都对不齐。收敛到这里，列定义里只写
 * `render: (v) => <StatusTag value={v} />`。
 */

/** 启用/停用。用柔和的 success/default 而不是红绿，停用不等于出错。 */
export function StatusTag({ value }: { value?: number | null }) {
  const enabled = value === 1
  return <Tag color={enabled ? 'success' : 'default'}>{enabled ? '启用' : '停用'}</Tag>
}

/**
 * 金额：千分位 + 按币种小数位，等宽数字避免列内错位。
 *
 * 不传 currency 时按 2 位（改造前的行为）：绝大多数列没有币种上下文，
 * 而给每个调用点都补一个 currency 是一次全站改动，分批做。
 *
 * **小数位必须读币种，不能固定 2 位**：JPY 是 0 位，显示成 `1,000.00`
 * 会让人以为有小数；KWD 是 3 位，固定 2 位会把末位截掉、金额凭空变小。
 */
export function Money({
  value,
  currency,
}: {
  value?: number | string | null
  currency?: string | null
}) {
  // 只在传了币种时才拉币种表：react-query 按 key 去重，
  // 一页几十个 Money 只会发一次请求。
  const { result } = useList<Currency>({
    resource: 'currencies',
    pagination: { mode: 'off' },
    queryOptions: { enabled: !!currency },
  })
  const dp = currency
    ? (result?.data.find((c) => c.code === currency)?.decimal_places ?? 2)
    : 2

  // 后端金额已经是定点小数的字符串（money.Decimal）：这里只做千分位与舍入显示，
  // 不参与任何算术——要算就在服务端算，前端算出来的数没法与票据对上。
  const num = typeof value === 'string' ? Number(value) : value
  if (num === null || num === undefined || Number.isNaN(num)) {
    return <span className="cell-empty">—</span>
  }
  return (
    <span style={{ fontVariantNumeric: 'tabular-nums' }}>
      {num.toLocaleString('zh-CN', {
        minimumFractionDigits: dp,
        maximumFractionDigits: dp,
      })}
      {currency ? <span className="cell-sub"> {currency}</span> : null}
    </span>
  )
}

/** 类别：显示中文名，未知码原样透出（不吞数据，便于发现脏值）。 */
export function CategoryTag({ value }: { value?: string | null }) {
  const label = categoryLabel(value)
  if (!label) return <span className="cell-empty">未分类</span>
  return <Tag>{label}</Tag>
}

/** 空值占位。表格里大量空字段，统一成一个短横，避免整列看起来像渲染失败。 */
export function Empty() {
  return <span className="cell-empty">—</span>
}

/** 编码类字段用等宽，SKU/供应商编码对齐后更易扫读。 */
export function Mono({ children }: { children?: string | null }) {
  if (!children) return <Empty />
  return <span className="cell-mono">{children}</span>
}

/**
 * 主体角色。
 *
 * 必须按位判断，不能写 role === 1：一个主体可以同时是客户和供应商（值为 3），
 * 用等号判断会让它在两个列表里都显示成"未知角色"。
 */
export function RoleTag({ value }: { value?: number | null }) {
  const role = value ?? 0
  const customer = (role & PARTNER_ROLE.customer) !== 0
  const supplier = (role & PARTNER_ROLE.supplier) !== 0
  const forwarder = (role & PARTNER_ROLE.forwarder) !== 0

  if (!customer && !supplier && !forwarder) return <Empty />
  return (
    <Space size={4}>
      {customer && <Tag color="blue">客户</Tag>}
      {supplier && <Tag color="geekblue">供应商</Tag>}
      {forwarder && <Tag color="cyan">货代</Tag>}
    </Space>
  )
}

/**
 * 主体类型。
 *
 * 公司是常态，用中性样式不抢注意力；个人要显眼一些 ——
 * 它意味着报关资料、发票抬头都跟公司主体不同，不是纯粹的标签差异。
 */
export function EntityTypeTag({ value }: { value?: number | null }) {
  if (value === 1) return <Tag color="gold">个人</Tag>
  return <Tag>公司</Tag>
}
