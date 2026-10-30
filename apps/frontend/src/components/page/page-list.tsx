import { ReloadOutlined, SearchOutlined } from '@ant-design/icons'
import { CreateButton, List } from '@refinedev/antd'
import { Button, Empty, Form, Grid, Space, Table } from 'antd'
import type { FormProps, TableProps } from 'antd'
import type { ReactNode } from 'react'

import { useUiPrefs } from '@/providers/uiPrefs'

/**
 * 列表页外壳。
 *
 * 改造前 21 个列表页各写一遍：筛选项手写宽度、查询/重置按钮各排一次、
 * 表格「暂无数据」各写一遍、横向滚动宽度 990~1200 各不相同、表头额外按钮
 * 有的裸放有的包 Space。统一到这里之后，页面只剩「哪些筛选项、哪些列」。
 *
 * 不吞掉 refine 的能力：`tableProps` / `searchFormProps` 原样透传，
 * 页面仍然能用 useTable 的排序、筛选、分页与 URL 同步。
 */

export interface PageListProps<T> {
  /** 覆盖 refine 从资源推导的标题；不传就用资源名。 */
  title?: ReactNode
  /** 标题下方一句话：这页的口径是什么。可选，写了才占位。 */
  description?: ReactNode
  /** 有值才渲染「新建」按钮；只读资源不传即可。 */
  createLabel?: ReactNode
  /** 表头额外动作（导出、重算、视图切换等），自动排在新建按钮左侧。 */
  extra?: ReactNode
  /** 筛选项：一组 Form.Item。查询/重置由外壳统一渲染，页面不用再写。 */
  filters?: ReactNode
  searchFormProps?: FormProps
  onReset?: () => void
  tableProps: TableProps<T>
  rowKey: string
  /** 表格最小宽度。不传按内容自适应（max-content），不再让各页手写数值。 */
  scrollX?: number | string
  /** 自定义空态文案：默认按「还没数据」与「筛选后没有结果」区分。 */
  emptyText?: ReactNode
  /** 完全自定义表格区域（如产品卡片视图）。给了就不渲染内置 Table。 */
  table?: ReactNode
  children?: ReactNode
}

export function PageList<T extends object>({
  title,
  description,
  createLabel,
  extra,
  filters,
  searchFormProps,
  onReset,
  tableProps,
  rowKey,
  scrollX,
  emptyText,
  table,
  children,
}: PageListProps<T>) {
  const { compact } = useUiPrefs()
  const screens = Grid.useBreakpoint()
  const narrow = screens.md === false

  const headerButtons =
    extra || createLabel ? (
      <Space wrap size={8}>
        {extra}
        {createLabel ? <CreateButton>{createLabel}</CreateButton> : null}
      </Space>
    ) : undefined

  // 分页统一：页大小可选、总数按「当前区间 / 总数」显示。
  // 页面显式传 pagination={false} 时保留 false，不要偷偷加回分页条。
  const pagination =
    tableProps.pagination === false
      ? (false as const)
      : {
          showSizeChanger: true,
          pageSizeOptions: ['10', '20', '50', '100'],
          showTotal: (total: number, range: [number, number]) =>
            total > 0 ? `第 ${range[0]}-${range[1]} 条 / 共 ${total} 条` : '',
          ...(typeof tableProps.pagination === 'object' ? tableProps.pagination : {}),
        }

  const empty = emptyText ?? (
    <Empty
      image={Empty.PRESENTED_IMAGE_SIMPLE}
      description={filters ? '没有符合条件的记录，换个条件再试' : '还没有数据'}
    />
  )

  return (
    <List title={title} headerButtons={headerButtons}>
      {description ? <div className="list-caption">{description}</div> : null}

      {filters ? (
        <Form
          {...searchFormProps}
          layout="inline"
          className={narrow ? 'list-filters is-narrow' : 'list-filters'}
        >
          {filters}
          <Form.Item className="list-filters-actions">
            <Space size={8}>
              <Button type="primary" htmlType="submit" icon={<SearchOutlined />}>
                查询
              </Button>
              {onReset ? (
                <Button onClick={onReset} icon={<ReloadOutlined />}>
                  重置
                </Button>
              ) : null}
            </Space>
          </Form.Item>
        </Form>
      ) : null}

      {table ?? (
        <Table<T>
          {...tableProps}
          rowKey={rowKey}
          size={compact ? 'small' : 'middle'}
          pagination={pagination}
          scroll={{ x: scrollX ?? 'max-content' }}
          locale={{ emptyText: empty, ...tableProps.locale }}
        >
          {children}
        </Table>
      )}
    </List>
  )
}


