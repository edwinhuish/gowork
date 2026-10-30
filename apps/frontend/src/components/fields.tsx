import { useList } from '@refinedev/core'
import { Space, Tag, theme } from 'antd'
import type { CSSProperties, ReactNode } from 'react'

import {
  categoryLabel,
  ENABLE_STATUS_OPTIONS,
  PARTNER_ROLE,
} from '@/constants/catalog'
import type { StatusTone } from '@/constants/catalog'
import type { Currency } from '@/types'
import { mixHex } from '@/utils/color'

/**
 * 业务字段的统一展示件。
 *
 * 三个主数据模块里「状态」「金额」「类别」出现十几次，改造前每个页面各写一遍
 * 渲染函数，停用色和金额小数位都对不齐。收敛到这里，列定义里只写
 * `render: (v) => <StatusTag value={v} />`。
 */

/**
 * 启用 / 停用。
 *
 * 只对账号与币种这类"还留着这个状态"的对象用：主数据的启用状态已经拿掉，
 * 列表里剩下的筛选维度是软删除。用柔和的 success/default 而不是红绿，
 * 停用不等于出错。币种那边同一个字段叫归档，说辞在 CURRENCY_STATUS_OPTIONS。
 */
export function StatusTag({ value }: { value?: number | null }) {
  return <OptionTag value={value} options={ENABLE_STATUS_OPTIONS} />
}

/**
 * 字典状态标签。
 *
 * 色调写在 `constants/catalog.ts` 的选项表里，页面只传「值 + 用哪本字典」——
 * 改造前每个单据页各写一份 statusColor()，同一个值在不同页上颜色不同。
 * `bordered={false}` 让标签融进一行，不额外加一圈边框抢注意力。
 */
export function OptionTag({
  value,
  options,
}: {
  value?: string | number | null
  options: readonly { value: string | number; label: string; tone?: StatusTone }[]
}) {
  if (value === null || value === undefined || value === '') return <Empty />
  const item = options.find((option) => option.value === value)
  if (!item) return <Tag bordered={false}>{String(value)}</Tag>
  return (
    <Tag bordered={false} color={item.tone ?? 'default'}>
      {item.label}
    </Tag>
  )
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
 *
 * 三个角色都是中性标签，不再各给一种蓝：角色是**身份**而不是状态，
 * 三个蓝（blue / geekblue / cyan）并排还几乎分不出来，读的人只会以为
 * "客户 / 供应商 / 货代"是三种需要不同处理的状态。灰底 + 文字已经够区分了。
 */
export function RoleTag({ value }: { value?: number | null }) {
  const role = value ?? 0
  const customer = (role & PARTNER_ROLE.customer) !== 0
  const supplier = (role & PARTNER_ROLE.supplier) !== 0
  const forwarder = (role & PARTNER_ROLE.forwarder) !== 0

  if (!customer && !supplier && !forwarder) return <Empty />
  return (
    <Space size={4}>
      {customer && <Tag>客户</Tag>}
      {supplier && <Tag>供应商</Tag>}
      {forwarder && <Tag>货代</Tag>}
    </Space>
  )
}

/**
 * 主体类型。
 *
 * 公司是常态，用中性标签；个人用强调色 ——
 * 它意味着报关资料、发票抬头都跟公司主体不同，不是纯粹的标签差异。
 */
export function EntityTypeTag({ value }: { value?: number | null }) {
  if (value === 1) return <AccentTag>个人</AccentTag>
  return <Tag>公司</Tag>
}

/**
 * 强调标记：「默认地址 / 默认仓库 / 本币 / 主联系人 / 以销定采 / 个人主体」这一类。
 *
 * 它们不是状态——没有"进行中 / 已完成"可言，只是"这一条是特殊的那一个"。
 * 塞进五档语义色会被读成状态（橙 = 要处理、红 = 出错），所以另开一档：
 * 主色淡底 + 主色字，与 Menu 选中项、Select 选中项同源，
 * 一眼是"被挑出来的那个"。亮色 6.08、暗色 5.48，都过正文标准。
 */
export function AccentTag({ children, style }: { children: ReactNode; style?: CSSProperties }) {
  const { token } = theme.useToken()
  return (
    <Tag
      bordered={false}
      style={{ color: token.colorPrimaryHover, background: token.colorPrimaryBg, ...style }}
    >
      {children}
    </Tag>
  )
}

/**
 * 有序量的梯度标签：`level` 0 最轻（橙），3 最重（红），中间两档在二者之间插值。
 *
 * 逾期天数分桶这类数字是**同一个量在变大**，四个色相（gold / orange /
 * volcano / red）会读成四种状态。中间档不新增色号：在 warning 与 error 之间
 * 插值，底色是字色压进容器底 14%，四档深浅齐平（亮色 5.08~5.88、暗色 4.77~4.87）。
 */
export function HeatTag({ level, children }: { level: 0 | 1 | 2 | 3; children: ReactNode }) {
  const { token } = theme.useToken()
  const fg = mixHex(token.colorError, token.colorWarning, level / 3)
  return (
    <Tag
      bordered={false}
      style={{ color: fg, background: mixHex(fg, token.colorBgContainer, 0.14) }}
    >
      {children}
    </Tag>
  )
}
