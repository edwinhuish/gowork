import { Tooltip } from '@/components/ui/Tooltip'
import type { CrudFilters } from '@refinedev/core'
import { useSelect } from '@refinedev/core'
import {
  DeleteButton,
  EditButton,
  ShowButton,
} from '@refinedev/antd'
import { Form, Input, Select, Space, Table, Tag } from 'antd'

import { Empty, Mono, Money, OptionTag } from '@/components/fields'
import { PageList } from '@/components/page'
import { ORDER_STATUS_OPTIONS, tradeTermLabel } from '@/constants/catalog'
import { useInfiniteList } from '@/hooks/use-infinite-feed'
import type { Partner, SalesOrder } from '@/types'
import { formatDate, formatQty } from '@/utils/format'

interface OrderSearch {
  keyword?: string
  partner_id?: number
  status?: number
  /** =1 只看交期已过、还没出完的单（跟单视角）。 */
  overdue_unshipped?: number
}

/**
 * 履约列：已出 / 订购 + 未出。
 *
 * 三个数都是后端算的投影（`gorm:"-"`），前端只做显示——把"未出 = 数量 − 已出"
 * 在这里再算一遍，出货单作废或改量之后两边就会各说一个数。
 */
function FulfillmentCell({ record }: { record: SalesOrder }) {
  const total = Number(record.order_qty ?? 0)
  if (!total) return <Empty />
  const shipped = Number(record.shipped_qty ?? 0)
  const unshipped = Number(record.unshipped_qty ?? 0)
  const percent = Math.min(100, Math.round((shipped / total) * 100))
  return (
    <div className="cell-stack">
      <span>
        {formatQty(shipped)} / {formatQty(total)}
        <span className="cell-sub"> · {percent}%</span>
      </span>
      {/* 逾期由后端判（含状态条件），前端不自己拿交期拼一遍。 */}
      {unshipped > 0 ? (
        <Tag
          bordered={false}
          color={record.overdue_unshipped ? 'error' : 'default'}
          style={{ marginInlineEnd: 0 }}
        >
          未出 {formatQty(unshipped)}
        </Tag>
      ) : (
        <span className="cell-sub">已出完</span>
      )}
    </div>
  )
}

export function OrderList() {
  const { options: customerOptions, query: customerQuery } = useSelect<Partner>({
    resource: 'customers',
    optionLabel: (item) => `${item.name}（${item.code}）`,
    optionValue: 'id',
    pagination: { mode: 'off' },
  })

  const { tableProps, searchFormProps, feed } = useInfiniteList<SalesOrder>({
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
      if (v.overdue_unshipped) {
        filters.push({ field: 'overdue_unshipped', operator: 'eq', value: 1 })
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
      infinite={feed}
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
          <Form.Item name="overdue_unshipped">
            <Select
              placeholder="履约情况"
              allowClear
              options={[{ value: 1, label: '只看逾期未出' }]}
            />
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
        title="履约"
        width={150}
        render={(_, record: SalesOrder) => <FulfillmentCell record={record} />}
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
                                <Tooltip title="删除">
                  <DeleteButton size="small" recordItemId={record.id} resource="sales-orders" hideText />
                </Tooltip>
              </>
            )}
          </Space>
        )}
      />
    </PageList>
  )
}
