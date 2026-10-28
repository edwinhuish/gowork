import { Tag } from 'antd'

import { categoryLabel } from '@/constants/catalog'

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
