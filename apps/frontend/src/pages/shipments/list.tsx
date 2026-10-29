import type { CrudFilters } from '@refinedev/core'
import {
  CreateButton,
  DeleteButton,
  EditButton,
  List,
  ShowButton,
  useTable,
} from '@refinedev/antd'
import { Button, Form, Input, Select, Space, Table, Tag } from 'antd'

import { Empty, Mono } from '@/components/fields'
import { ORDER_STATUS_OPTIONS, orderStatusLabel } from '@/constants/catalog'
import type { Shipment } from '@/types'
import { formatDate, formatQty } from '@/utils/format'

interface ShipmentSearch {
  keyword?: string
  status?: number
}

/** 状态色与订单保持一致：草稿、已确认、进行中、已出货、收口、取消。 */
function statusColor(value: number): string {
  if (value === 20) return 'success'
  if (value === 30) return 'processing'
  if (value === 40) return 'warning'
  if (value === 50) return 'default'
  if (value === 90) return 'error'
  return 'default'
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
    <List headerButtons={<CreateButton>新建出货单</CreateButton>}>
      <Form {...searchFormProps} layout="inline" className="list-filters">
        <Form.Item name="keyword">
          <Input placeholder="搜索单号 / 备注" allowClear style={{ width: 200 }} />
        </Form.Item>
        <Form.Item name="status">
          <Select
            placeholder="全部状态"
            allowClear
            style={{ width: 130 }}
            options={ORDER_STATUS_OPTIONS}
          />
        </Form.Item>
        <Form.Item>
          <Space>
            <Button type="primary" htmlType="submit">
              查询
            </Button>
            <Button onClick={resetFilters}>重置</Button>
          </Space>
        </Form.Item>
      </Form>

      <Table {...tableProps} rowKey="id" scroll={{ x: 1000 }}>
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
          render={(value: number) => (
            <Tag color={statusColor(value)}>{orderStatusLabel(value)}</Tag>
          )}
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
      </Table>
    </List>
  )
}
