import { Space, Tag } from 'antd'

import { categoryLabel, PARTNER_ROLE } from '@/constants/catalog'

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

/** 金额：两位小数 + 千分位，等宽数字避免列内错位。 */
export function Money({ value }: { value?: number | null }) {
  if (value === null || value === undefined || Number.isNaN(value)) {
    return <span className="cell-empty">—</span>
  }
  return (
    <span style={{ fontVariantNumeric: 'tabular-nums' }}>
      {value.toLocaleString('zh-CN', { minimumFractionDigits: 2, maximumFractionDigits: 2 })}
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

  if (!customer && !supplier) return <Empty />
  return (
    <Space size={4}>
      {customer && <Tag color="blue">客户</Tag>}
      {supplier && <Tag color="geekblue">供应商</Tag>}
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
