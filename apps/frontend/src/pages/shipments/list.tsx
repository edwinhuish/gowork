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
import { ORDER_STATUS_OPTIONS } from '@/constants/catalog'
import type { Shipment } from '@/types'
import { formatDate, formatQty } from '@/utils/format'

interface ShipmentSearch {
  keyword?: string
  status?: number
}

export function ShipmentList() {
  const { tableProps, searchFormProps } = useTable<Shipment>({
    resource: 'shipments',
    syncWithLocation: true,
    onSearch: (values): CrudFilters => {
      const v = values as ShipmentSearch
      const filters: CrudFilters = []
      if (v.keyword?.trim()) {
        filters.push({ field: 'keyword', operator: 'contains', value: v.keyword.trim() })
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
      createLabel="新建出货单"
      searchFormProps={searchFormProps}
      onReset={resetFilters}
      tableProps={tableProps}
      rowKey="id"
      filters={
        <>
          <Form.Item name="keyword">
            <Input placeholder="搜索单号 / 备注" allowClear />
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
        dataIndex="ship_date"
        title="出货日"
        width={120}
        render={(value: string) => formatDate(value)}
      />
      <Table.Column
        dataIndex="allocations"
        title="装了哪些货"
        render={(_, record: Shipment) => {
          const allocations = record.allocations ?? []
          if (allocations.length === 0) return <Empty />
          // 数量是定点字符串：这里的合计只用于列表提示，算术在服务端。
          const totalQty = allocations.reduce((sum, a) => sum + Number(a.qty), 0)
          const docNos = [...new Set(allocations.map((a) => a.sales_order_doc_no))]
          return (
            <div className="cell-stack">
              <span className="cell-strong">
                {allocations.length} 行 · 共 {formatQty(totalQty)}
              </span>
              <span className="cell-sub">{docNos.filter(Boolean).join('、') || '—'}</span>
            </div>
          )
        }}
      />
      <Table.Column
        dataIndex="status"
        title="状态"
        width={100}
        render={(value: number) => <OptionTag value={value} options={ORDER_STATUS_OPTIONS} />}
      />
      <Table.Column
        dataIndex="remark"
        title="备注"
        render={(value: string) => value || <Empty />}
      />
      <Table.Column
        title="操作"
        width={200}
        render={(_, record: Shipment) => (
          <Space size={4}>
            <ShowButton size="small" recordItemId={record.id} resource="shipments" />
            {record.status === 10 && (
              <>
                <EditButton size="small" recordItemId={record.id} resource="shipments" />
                <DeleteButton size="small" recordItemId={record.id} resource="shipments" />
              </>
            )}
          </Space>
        )}
      />
    </PageList>
  )
}
