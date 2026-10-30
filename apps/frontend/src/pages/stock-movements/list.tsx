import type { CrudFilters } from '@refinedev/core'
import { DeleteButton, useTable } from '@refinedev/antd'
import { Form, Input, Select, Table, Tag } from 'antd'

import { Empty, Mono } from '@/components/fields'
import { PageList } from '@/components/page'
import {
  MANUAL_STOCK_MOVEMENT_OPTIONS,
  STOCK_MOVEMENT_OPTIONS,
  stockMovementLabel,
} from '@/constants/catalog'
import type { StockMovement } from '@/types'
import { formatDate, formatQty } from '@/utils/format'

interface MovementSearch {
  keyword?: string
  type?: number
  /** 业务发生日，YYYY-MM-DD，含两端。 */
  from?: string
  to?: string
}

/** 只有手工录入的流水能删；单据产生的要回到单据上处理。 */
function isManual(type: number): boolean {
  return MANUAL_STOCK_MOVEMENT_OPTIONS.some((o) => o.value === type)
}

export function StockMovementList() {
  const { tableProps, searchFormProps } = useTable<StockMovement>({
    resource: 'stock-movements',
    syncWithLocation: true,
    // 筛选表单给的是「这一次的完整条件」，所以用 replace：refine 默认 merge
    // 会把上一次的条件留着 —— 清空关键字再点查询，keyword 仍旧跟着请求走，
    // 表现是「条件清不掉、列表也不动」。永久筛选仍会保留（见 PageList 的说明）。
    filters: { defaultBehavior: 'replace' },
    onSearch: (values): CrudFilters => {
      const v = values as MovementSearch
      const filters: CrudFilters = []
      if (v.keyword?.trim()) {
        filters.push({ field: 'keyword', operator: 'contains', value: v.keyword.trim() })
      }
      if (v.type) {
        filters.push({ field: 'type', operator: 'eq', value: v.type })
      }
      // 只写起始或只写截止也成立：对账时经常是"从这个月 1 号起的全部流水"。
      if (v.from?.trim()) {
        filters.push({ field: 'from', operator: 'eq', value: v.from.trim() })
      }
      if (v.to?.trim()) {
        filters.push({ field: 'to', operator: 'eq', value: v.to.trim() })
      }
      return filters
    },
  })

  const resetFilters = () => {
    searchFormProps.form?.resetFields()
    searchFormProps.form?.submit()
  }

  return (
    <PageList
      createLabel="手工录入"
      searchFormProps={searchFormProps}
      onReset={resetFilters}
      tableProps={tableProps}
      rowKey="id"
      filters={
        <>
          <Form.Item name="keyword">
            <Input placeholder="搜索单号 / SKU / 品名 / 备注" allowClear />
          </Form.Item>
          <Form.Item name="type">
            <Select placeholder="全部类型" allowClear options={STOCK_MOVEMENT_OPTIONS} />
          </Form.Item>
          {/* 发生日区间用文本框而不是 RangePicker：项目没有引入 dayjs 之外的日期依赖，
              而选择器要在 dayjs 与字符串之间来回转，对账时手输 YYYY-MM-DD 更快。 */}
          <Form.Item name="from">
            <Input placeholder="起日 YYYY-MM-DD" allowClear />
          </Form.Item>
          <Form.Item name="to">
            <Input placeholder="止日 YYYY-MM-DD" allowClear />
          </Form.Item>
        </>
      }
    >
      <Table.Column
        dataIndex="doc_no"
        title="流水号"
        width={160}
        render={(value: string) => <Mono>{value}</Mono>}
      />
      <Table.Column
        dataIndex="occurred_at"
        title="发生日"
        width={120}
        render={(value: string) => formatDate(value)}
      />
      <Table.Column
        dataIndex="sku_code"
        title="SKU"
        width={140}
        render={(value: string) => <Mono>{value}</Mono>}
      />
      <Table.Column
        dataIndex="name_cn"
        title="品名 / 规格"
        render={(value: string, record: StockMovement) => (
          <div className="cell-stack">
            <span className="cell-strong">{value || <Empty />}</span>
            {record.spec && <span className="cell-sub">{record.spec}</span>}
          </div>
        )}
      />
      <Table.Column
        dataIndex="type"
        title="类型"
        width={110}
        render={(value: number) => <Tag bordered={false}>{stockMovementLabel(value)}</Tag>}
      />
      <Table.Column
        dataIndex="qty"
        title="数量"
        width={110}
        align="right"
        render={(value: string, record: StockMovement) => (
          <span className="cell-mono">
            {record.direction === -1 ? '−' : '+'}
            {formatQty(value)}
            {record.unit ? ` ${record.unit}` : ''}
          </span>
        )}
      />
      <Table.Column
        dataIndex="remark"
        title="备注 / 来源"
        render={(value: string, record: StockMovement) => {
          const source = record.source_doc_no
          return (
            <div className="cell-stack">
              <span>{value || <Empty />}</span>
              {source && <span className="cell-sub">来源 {source}</span>}
            </div>
          )
        }}
      />
      <Table.Column
        title="操作"
        width={90}
        render={(_, record: StockMovement) =>
          isManual(record.type) ? (
            <DeleteButton
              size="small"
              resource="stock-movements"
              recordItemId={record.id}
              confirmTitle="删除这条库存流水？"
              confirmOkText="删除"
              confirmCancelText="返回"
            />
          ) : (
            <span className="cell-sub">由单据产生</span>
          )
        }
      />
    </PageList>
  )
}
