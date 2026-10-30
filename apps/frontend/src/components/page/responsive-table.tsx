import { Grid, Table } from 'antd'
import type { PaginationProps, TablePaginationConfig, TableProps } from 'antd'
import { useMemo } from 'react'
import type { ReactNode } from 'react'

import type { CardColumn, CardSummary } from './record-cards'
import { collectCardColumns, collectCardColumnsFromDefs, RecordCards } from './record-cards'

export interface ResponsiveTableProps<T> {
  rows: T[]
  rowKey: string | ((row: T) => string)
  loading?: boolean
  /** 宽屏的分页配置，原样交给 Table。 */
  pagination?: TablePaginationConfig | false
  /**
   * 窄屏卡片的分页。宽屏由 Table 自己管，两种布局的分页状态不是同一个：
   * 表格那份是 antd 的内部状态（客户端分页）或 useTable 的 current（服务端分页），
   * 卡片这份要自己接，所以两个 props 分开传，不要互相猜。
   */
  pager?: PaginationProps
  /**
   * `rows` 是**全部**数据、由两种布局各自切页（客户端分页）。
   *
   * 不传时表示 rows 已经是当前页（服务端分页的 useTable 就是这样，
   * 卡片只负责画页码条）。两者的区别必须显式说：靠"有没有 current"去猜，
   * 服务端分页在首屏 current 为空时就会被切成空列表。
   */
  clientPaged?: boolean
  scrollX?: number | string
  /** 空态文案。 */
  empty?: ReactNode
  /** 宽屏：合计行（`Table.Summary`）。 */
  summary?: (rows: readonly T[]) => ReactNode
  /** 窄屏：同一份合计，排成「合计卡」。 */
  cardSummary?: CardSummary
  /** 窄屏卡片的标题列（默认第一列）：第一列不是"这条是谁"时用它。 */
  cardTitleKey?: string
  /**
   * 显式指定哪些列是操作列（按列的 key：dataIndex，没有的话是 `key`）。
   *
   * 自动判定靠的是"最后一列 + 无 dataIndex + 只有 render"，猜错了要能改：
   * 给了就以它为准，`[]` 表示"这张表没有操作列"。
   */
  cardActionKeys?: string[]
  size?: 'small' | 'middle'
  /**
   * 原样铺进 Table 的那份 props（服务端分页的 useTable 把 dataSource / loading /
   * 分页回调都放在里面）。先铺再覆盖上面的显式项，所以行键、尺寸、分页、横向滚动
   * 由这里统一管，页面不会各写一套。
   */
  tableProps?: TableProps<T>
  /**
   * 列定义的另一种写法：`columns={[...]}`。
   * 与下面的 `children`（`<Table.Column>`）二选一，两种都能推导出卡片。
   */
  columns?: TableProps<T>['columns']
  /** `<Table.Column>` 们：宽屏交给 Table，窄屏从它们的 props 推出卡片。与 `columns` 二选一。 */
  children?: ReactNode
}

/**
 * 宽屏是表格、窄屏（< 768px）是卡片。
 *
 * 与 PageList 里那套是同一个判断，只是把"降级"这件事单独拎出来：
 * 报表页不走 PageList（它们不是 CRUD 资源，没有 useTable，数据来自 react-query，
 * 表上还有合计行），但窄屏一样读不了 1180px 宽的滚动表。
 * 抽成组件是为了让"什么时候降级、降级成什么样"只写一次 ——
 * 判断散在四个页面里，改一次断点就要改四处。
 *
 * 卡片内容仍然从列定义推导（见 record-cards.tsx），页面不用为窄屏再写一套视图。
 */
export function ResponsiveTable<T extends object>({
  rows,
  rowKey,
  loading,
  pagination,
  pager,
  clientPaged,
  scrollX,
  empty,
  summary,
  cardSummary,
  cardTitleKey,
  cardActionKeys,
  size,
  tableProps,
  columns: columnDefs,
  children,
}: ResponsiveTableProps<T>) {
  const narrow = Grid.useBreakpoint().md === false
  // 列定义只依赖 children / columns 的引用：页面每次渲染都新建这些元素，
  // 但推导本身很便宜，且它们变化时必须重算，所以只按这两个来源记忆。
  const columns = useMemo<CardColumn<T>[]>(() =>
    columnDefs
      ? collectCardColumnsFromDefs<T>(columnDefs, cardActionKeys)
      : (collectCardColumns(children, cardActionKeys) as CardColumn<T>[]),
  [columnDefs, children, cardActionKeys])

  // 推导不出列（条件渲染、包了不认识的节点）就退回表格：
  // 最坏情况是没有降级，不会白屏。
  if (narrow && columns.length > 0) {
    const start = ((pager?.current ?? 1) - 1) * (pager?.pageSize ?? rows.length)
    const pageRows =
      clientPaged && pager?.pageSize ? rows.slice(start, start + pager.pageSize) : rows

    return (
      <RecordCards<T>
        rows={pageRows}
        columns={columns}
        rowKey={rowKey}
        loading={loading}
        pager={pager}
        empty={empty}
        summary={cardSummary}
        titleKey={cardTitleKey}
      />
    )
  }

  return (
    <Table<T>
      {...tableProps}
      dataSource={rows}
      columns={columnDefs}
      rowKey={rowKey as string}
      loading={loading}
      size={size}
      pagination={pagination}
      scroll={{ x: scrollX ?? 'max-content' }}
      locale={{ emptyText: empty, ...tableProps?.locale }}
      summary={summary}
    >
      {columnDefs ? undefined : children}
    </Table>
  )
}
