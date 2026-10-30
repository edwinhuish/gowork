import type { CrudFilters } from '@refinedev/core'
import { useSelect } from '@refinedev/core'
import {
  DeleteButton,
  EditButton,
  ShowButton,
  useTable,
} from '@refinedev/antd'
import { Form, Input, Select, Space, Table, Tag } from 'antd'

import { Empty, Mono, Money, OptionTag } from '@/components/fields'
import { PageList } from '@/components/page'
import { ORDER_STATUS_OPTIONS, tradeTermLabel } from '@/constants/catalog'
import type { Partner, SalesOrder } from '@/types'
import { formatDate } from '@/utils/format'

interface OrderSearch {
  keyword?: string
  partner_id?: number
  status?: number
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
    // 筛选表单给的是「这一次的完整条件」，所以用 replace：refine 默认 merge
    // 会把上一次的条件留着 —— 清空关键字再点查询，keyword 仍旧跟着请求走，
    // 表现是「条件清不掉、列表也不动」。永久筛选仍会保留（见 PageList 的说明）。
    filters: { defaultBehavior: 'replace' },
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
    <PageList
      createLabel="新建订单"
      searchFormProps={searchFormProps}
      onReset={resetFilters}
      tableProps={tableProps}
      rowKey="id"
      filters={
        <>
          <Form.Item name="keyword">
            <Input placeholder="搜索单号 / 备注" allowClear />
          </Form.Item>
          <Form.Item name="partner_id">
            <Select
              placeholder="全部客户"
              allowClear
              showSearch
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
        dataIndex="partner"
        title="客户"
        width={190}
        render={(value: SalesOrder['partner']) => value?.name || <Empty />}
      />
      <Table.Column
        dataIndex="trade_term"
        title="贸易术语"
        width={110}
        render={(value: string) => <Tag bordered={false}>{tradeTermLabel(value)}</Tag>}
      />
      <Table.Column
        dataIndex="status"
        title="状态"
        width={100}
        render={(value: number) => <OptionTag value={value} options={ORDER_STATUS_OPTIONS} />}
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
            <Tag bordered={false}>{record.currency_code}</Tag>
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
    </PageList>
  )
}
