import { Pagination, Skeleton } from 'antd'
import type { PaginationProps } from 'antd'
import { Children, Fragment, isValidElement } from 'react'
import type { ReactNode } from 'react'

/**
 * 窄屏（< 768px）下把宽表降级成的卡片列表。
 *
 * 列表页常见 8–12 列（单号 / 客户 / 状态 / 金额 / 交期…），平板上只有两条路：
 * 横向滚动，或者把一行拆成一张卡。前者是"把桌面版塞进手机"——
 * 二十列的表在 375px 上要横滑五六屏，扫一列金额时前面的列早滑走了；
 * 卡片是同一个 useTable 的另一种渲染方式，数据、筛选、排序、页码都不变，
 * 只是把"一行横着读"改成"一张竖着读"。
 *
 * 卡片内容**从表格的列定义推导**，不是每个列表页再写一份 card：
 * 24 个列表页各写一套卡片视图，改一个字段就要改两处，必然有人漏。
 * 推导规则只有三条：
 *   1. 第一列（非操作列）当卡片的标题 —— 各页都是"这条是谁"（单号 / 名称 / 编码）；
 *   2. 操作列（无 dataIndex、只有 render 的那一列）落到卡片底部；
 *   3. 其余列按顺序排成「标签：值」。
 *
 * 产品与三张往来单位表不走这里：它们本来就有双视图（useListView），
 * 卡片是设计过的（产品有图、主体有 Logo），窄屏只是把默认视图改成卡片。
 */

/** 列定义只需要这几个字段，够渲染一张卡即可。 */
export interface CardColumn<T> {
  key: string
  title?: ReactNode
  dataIndex?: string | readonly (string | number)[]
  render?: (value: unknown, record: T, index: number) => ReactNode
  /** 操作列：落到卡片底部，不进「标签：值」列表。 */
  actions?: boolean
}

interface ColumnLikeProps {
  title?: ReactNode
  dataIndex?: string | readonly (string | number)[]
  render?: (value: never, record: never, index: number) => ReactNode
  key?: string | number
}

/**
 * 从 `<Table.Column>` 的 children 里读出列定义。
 *
 * 读的是 React 元素的 props，不是渲染结果：antd 的 Column 是个占位组件，
 * 真正建列的是 Table 内部，页面侧拿不到 columns 数组。
 * 这些页面写死的都是 `<Table.Column>`，所以读 props 是稳定的；
 * 万一哪天读不到（条件渲染、包了一层不认识的节点），
 * 结果是空数组，PageList 会退回表格 —— 最坏情况是没有降级，不会白屏。
 */
export function collectCardColumns(children: ReactNode): CardColumn<never>[] {
  const columns: CardColumn<never>[] = []

  Children.forEach(children, (child, index) => {
    if (!isValidElement(child)) return

    // 片段（页面里用 <> 包一组列）继续往下找，不要把片段当成一列。
    if (child.type === Fragment) {
      columns.push(...collectCardColumns((child.props as { children?: ReactNode }).children))
      return
    }

    const props = child.props as ColumnLikeProps
    if (props.title === undefined && props.dataIndex === undefined && props.render === undefined) {
      return
    }

    // 操作列的共同长相：没有 dataIndex，只有一个 render（按钮组）。
    const isActions = props.title === '操作' || (props.dataIndex === undefined && typeof props.render === 'function')
    columns.push({
      key: String(props.dataIndex ?? props.key ?? index),
      title: props.title,
      dataIndex: props.dataIndex,
      render: props.render as CardColumn<never>['render'],
      actions: isActions,
    })
  })

  return columns
}

/** 按 dataIndex 取值。支持 `a.b` 与数组路径，够覆盖现有列。 */
function readValue(record: object, dataIndex: string | readonly (string | number)[] | undefined): unknown {
  if (dataIndex === undefined) return undefined
  const path = Array.isArray(dataIndex) ? dataIndex.map(String) : String(dataIndex).split('.')
  let current: unknown = record
  for (const segment of path) {
    if (current === null || typeof current !== 'object') return undefined
    current = (current as Record<string, unknown>)[segment]
  }
  return current
}

export interface RecordCardsProps<T> {
  rows: T[]
  columns: CardColumn<T>[]
  rowKey: string
  loading?: boolean
  pager?: PaginationProps
  /** 空态：一般是「没有符合条件的记录」那一句。 */
  empty?: ReactNode
}

export function RecordCards<T extends object>({
  rows,
  columns,
  rowKey,
  loading,
  pager,
  empty,
}: RecordCardsProps<T>) {
  // 骨架屏的结构与真实卡片一致（标题 + 三行「标签：值」），数据到了不跳版。
  if (loading && rows.length === 0) {
    return (
      <div className="record-grid">
        {Array.from({ length: 6 }).map((_, i) => (
          <div className="record-card record-card--ghost" key={i}>
            <Skeleton active title={{ width: '60%' }} paragraph={{ rows: 3 }} />
          </div>
        ))}
      </div>
    )
  }

  if (rows.length === 0) return <>{empty ?? null}</>

  const titleColumn = columns.find((column) => !column.actions)
  const factColumns = columns.filter((column) => !column.actions && column !== titleColumn)
  const actionColumns = columns.filter((column) => column.actions)

  return (
    <>
      <div className="record-grid">
        {rows.map((record, index) => (
          <article
            className="record-card"
            key={String((record as Record<string, unknown>)[rowKey] ?? index)}
          >
            {titleColumn ? (
              <div className="record-card-title">{renderCell(titleColumn, record, index)}</div>
            ) : null}

            {factColumns.length > 0 ? (
              <div className="record-card-facts">
                {factColumns.map((column) => (
                  <div className="record-card-fact" key={column.key}>
                    <span className="record-card-fact-label">{column.title}</span>
                    <span className="record-card-fact-value">
                      {renderCell(column, record, index)}
                    </span>
                  </div>
                ))}
              </div>
            ) : null}

            {actionColumns.length > 0 ? (
              <div className="record-card-actions">
                {actionColumns.map((column) => (
                  <span key={column.key}>{renderCell(column, record, index)}</span>
                ))}
              </div>
            ) : null}
          </article>
        ))}
      </div>
      {pager ? (
        <div className="record-grid-pager">
          <Pagination {...pager} />
        </div>
      ) : null}
    </>
  )
}

function renderCell<T>(column: CardColumn<T>, record: T, index: number): ReactNode {
  const value = readValue(record as object, column.dataIndex)
  if (column.render) return column.render(value, record, index)
  // 没有 render 的列（少数纯字段列）：空值给「—」，不要留一片空白。
  return value === null || value === undefined || value === '' ? '—' : String(value)
}
