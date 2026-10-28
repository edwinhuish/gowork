import type { CrudFilters } from '@refinedev/core'
import { useSelect } from '@refinedev/core'
import {
  CreateButton,
  DeleteButton,
  EditButton,
  List,
  ShowButton,
  useTable,
} from '@refinedev/antd'
import { Button, Form, Input, Select, Space, Table, Tag } from 'antd'

import { Empty, Mono, Money } from '@/components/fields'
import { ORDER_STATUS_OPTIONS, orderStatusLabel, tradeTermLabel } from '@/constants/catalog'
import type { Partner, SalesOrder } from '@/types'
import { formatDate } from '@/utils/format'

interface OrderSearch {
  keyword?: string
  partner_id?: number
  status?: number
}

/** 状态色：草稿待办、已确认承诺、备货/出货进行中、已完成收敛、已取消留档。 */
function statusColor(value: number): string {
  if (value === 20) return 'success'
  if (value === 30) return 'processing'
  if (value === 40) return 'warning'
  if (value === 50) return 'default'
  if (value === 90) return 'error'
  return 'default'
}

export function OrderList() {
  const { options: customerOptions, query: customerQuery } = useSelect<Partner>({
    resource: 'customers',
    optionLabel: (item) => `${item.name}（${item.code}）`,
    optionValue: 'id',
    pagination: { mode: 'off' },
  })

  const { tableProps, searchFormProps } = useTable<SalesOrder>({
    resource: 'sales-orders',
    syncWithLocation: true,
    onSearch: (values): CrudFilters => {
      const v = values as OrderSearch
      const filters: CrudFilters = []
      if (v.keyword?.trim()) {
        filters.push({ field: 'keyword', operator: 'contains', value: v.keyword.trim() })
      }
      if (v.partner_id) {
        filters.push({ field: 'partner_id', operator: 'eq', value: v.partner_id })
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
    <List headerButtons={<CreateButton>新建订单</CreateButton>}>
      <Form {...searchFormProps} layout="inline" className="list-filters">
        <Form.Item name="keyword">
          <Input placeholder="搜索单号 / 备注" allowClear style={{ width: 200 }} />
        </Form.Item>
        <Form.Item name="partner_id">
          <Select
            placeholder="全部客户"
            allowClear
            showSearch
            style={{ width: 240 }}
            options={customerOptions}
            loading={customerQuery.isLoading}
            filterOption={(input, option) =>
              String(option?.label ?? '')
                .toLowerCase()
                .includes(input.toLowerCase())
            }
          />
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

      <Table {...tableProps} rowKey="id" scroll={{ x: 1100 }}>
        <Table.Column
          dataIndex="doc_no"
          title="单号"
          width={170}
          render={(value: string) => <Mono>{value}</Mono>}
        />
        <Table.Column
          dataIndex="partner"
          title="客户"
          width={190}
          render={(value: SalesOrder['partner']) => value?.name || <Empty />}
        />
        <Table.Column
          dataIndex="trade_term"
          title="贸易术语"
          width={110}
          render={(value: string) => <Tag>{tradeTermLabel(value)}</Tag>}
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
          dataIndex="order_date"
          title="下单日"
          width={120}
          render={(value: string) => formatDate(value)}
        />
        <Table.Column
          dataIndex="delivery_date"
          title="交期"
          width={120}
          render={(value?: string | null) => (value ? formatDate(value) : <Empty />)}
        />
        <Table.Column
          dataIndex="amount_total"
          title="订单金额"
          width={150}
          align="right"
          render={(value: number, record: SalesOrder) => (
            <Space size={4}>
              <Money value={value} />
              <Tag>{record.currency_code}</Tag>
            </Space>
          )}
        />
        <Table.Column
          dataIndex="amount_base_total"
          title="折本币"
          width={130}
          align="right"
          render={(value: number) => <Money value={value} />}
        />
        <Table.Column
          title="操作"
          width={200}
          render={(_, record: SalesOrder) => (
            <Space size={4}>
              <ShowButton size="small" recordItemId={record.id} resource="sales-orders" />
              {record.status === 10 && (
                <>
                  <EditButton size="small" recordItemId={record.id} resource="sales-orders" />
                  <DeleteButton size="small" recordItemId={record.id} resource="sales-orders" />
                </>
              )}
            </Space>
          )}
        />
      </Table>
    </List>
  )
}
