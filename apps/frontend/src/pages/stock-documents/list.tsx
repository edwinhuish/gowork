import type { CrudFilters } from '@refinedev/core'
import {
  DeleteButton,
  EditButton,
  ShowButton,
  useTable,
} from '@refinedev/antd'
import { Form, Input, Select, Space, Table } from 'antd'

import { Empty, Mono, OptionTag } from '@/components/fields'
import { PageList } from '@/components/page'
import {
  ORDER_STATUS_OPTIONS,
  STOCK_DOC_COUNT,
  STOCK_DOC_TYPE_OPTIONS,
} from '@/constants/catalog'
import type { StockDocument } from '@/types'
import { formatDate, formatQty } from '@/utils/format'

interface DocSearch {
  keyword?: string
  type?: number
  status?: number
}

export function StockDocumentList() {
  const { tableProps, searchFormProps } = useTable<StockDocument>({
    resource: 'stock-documents',
    syncWithLocation: true,
    onSearch: (values): CrudFilters => {
      const v = values as DocSearch
      const filters: CrudFilters = []
      if (v.keyword?.trim()) {
        filters.push({ field: 'keyword', operator: 'contains', value: v.keyword.trim() })
      }
      if (v.type) {
        filters.push({ field: 'type', operator: 'eq', value: v.type })
      }
      if (v.status) {
        filters.push({ field: 'status', operator: 'eq', value: v.status })
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
      createLabel="新建出入库单"
      searchFormProps={searchFormProps}
      onReset={resetFilters}
      tableProps={tableProps}
      rowKey="id"
      filters={
        <>
          <Form.Item name="keyword">
            <Input placeholder="搜索单号 / 来源单号 / 备注" allowClear />
          </Form.Item>
          <Form.Item name="type">
            <Select placeholder="全部类型" allowClear options={STOCK_DOC_TYPE_OPTIONS} />
          </Form.Item>
          <Form.Item name="status">
            <Select placeholder="全部状态" allowClear options={ORDER_STATUS_OPTIONS} />
          </Form.Item>
        </>
      }
    >
      <Table.Column
        dataIndex="doc_no"
        title="单号"
        width={170}
        render={(value: string) => <Mono>{value}</Mono>}
      />
      <Table.Column
        dataIndex="type"
        title="类型"
        width={110}
        // 出入库类型是身份（方向不同），不是状态，所以字典里没有 tone：
        // 与详情页同一个 OptionTag，两处不会再各挑一套色。
        render={(value: number) => <OptionTag value={value} options={STOCK_DOC_TYPE_OPTIONS} />}
      />
      <Table.Column
        dataIndex="status"
        title="状态"
        width={90}
        render={(value: number) => <OptionTag value={value} options={ORDER_STATUS_OPTIONS} />}
      />
      <Table.Column
        dataIndex="doc_date"
        title="单据日期"
        width={120}
        render={(value: string) => formatDate(value)}
      />
      <Table.Column
        dataIndex="partner"
        title="往来单位"
        width={180}
        render={(value: StockDocument['partner']) => value?.name || <Empty />}
      />
      <Table.Column
        dataIndex="items"
        title="明细"
        render={(_, record: StockDocument) => {
          const items = record.items ?? []
          if (items.length === 0) return <Empty />
          // 数量是定点字符串：合计只用于列表提示。
          const total = items.reduce((sum, it) => sum + Number(it.qty), 0)
          return (
            <div className="cell-stack">
              <span className="cell-strong">
                {items.length} 行
                {/* 盘点单的数量是实盘数，加起来没有意义，所以不显示合计。 */}
                {record.type !== STOCK_DOC_COUNT && ` · 共 ${formatQty(total)}`}
              </span>
              <span className="cell-sub">{items.map((it) => it.sku_code).join('、')}</span>
            </div>
          )
        }}
      />
      <Table.Column
        dataIndex="source_doc_no"
        title="来源单号"
        width={160}
        render={(value: string) => (value ? <Mono>{value}</Mono> : <Empty />)}
      />
      <Table.Column
        title="操作"
        width={200}
        render={(_, record: StockDocument) => (
          <Space size={4}>
            <ShowButton size="small" recordItemId={record.id} resource="stock-documents" />
            {record.status === 10 && (
              <>
                <EditButton size="small" recordItemId={record.id} resource="stock-documents" />
                <DeleteButton size="small" recordItemId={record.id} resource="stock-documents" />
              </>
            )}
          </Space>
        )}
      />
    </PageList>
  )
}
