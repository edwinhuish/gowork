import type { CrudFilters } from '@refinedev/core'
import { useSelect } from '@refinedev/core'
import {
  DeleteButton,
  EditButton,
  ShowButton,
  useTable,
} from '@refinedev/antd'
import { Form, Input, Select, Space, Table, Tag, Tooltip } from 'antd'

import { Empty, Mono, Money, OptionTag } from '@/components/fields'
import { PageList } from '@/components/page'
import { ORDER_STATUS_OPTIONS, tradeTermLabel } from '@/constants/catalog'
import { PURCHASE_NODES, type Partner, type PurchaseOrder } from '@/types'
import { formatDate } from '@/utils/format'

interface PurchaseSearch {
  keyword?: string
  supplier_id?: number
  status?: number
  /** 只看某个跟单节点已逾期的采购单（T-38）。 */
  overdue_node?: number
}

export function PurchaseList() {
  const { options: supplierOptions, query: supplierQuery } = useSelect<Partner>({
    resource: 'suppliers',
    optionLabel: (item) => `${item.name}（${item.code}）`,
    optionValue: 'id',
    pagination: { mode: 'off' },
  })

  const { tableProps, searchFormProps } = useTable<PurchaseOrder>({
    resource: 'purchase-orders',
    syncWithLocation: true,
    onSearch: (values): CrudFilters => {
      const v = values as PurchaseSearch
      const filters: CrudFilters = []
      if (v.keyword?.trim()) {
        filters.push({ field: 'keyword', operator: 'contains', value: v.keyword.trim() })
      }
      if (v.supplier_id) {
        filters.push({ field: 'supplier_id', operator: 'eq', value: v.supplier_id })
      }
      if (v.status) {
        filters.push({ field: 'status', operator: 'eq', value: v.status })
      }
      if (v.overdue_node) {
        // 筛"这个跟单节点已逾期（计划日已过、还没完成）"的采购单（T-38）。
        filters.push({ field: 'overdue_node', operator: 'eq', value: v.overdue_node })
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
      createLabel="新建采购单"
      searchFormProps={searchFormProps}
      onReset={resetFilters}
      tableProps={tableProps}
      rowKey="id"
      filters={
        <>
          <Form.Item name="keyword">
            <Input placeholder="搜索单号 / 备注" allowClear />
          </Form.Item>
          <Form.Item name="supplier_id">
            <Select
              placeholder="全部供应商"
              allowClear
              showSearch
              options={supplierOptions}
              loading={supplierQuery.isLoading}
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
          <Form.Item name="overdue_node">
            <Select
              placeholder="跟单逾期节点"
              allowClear
              options={PURCHASE_NODES.map((n) => ({ value: n.value, label: `${n.label}逾期` }))}
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
        dataIndex="supplier"
        title="供应商"
        width={190}
        render={(value: PurchaseOrder['supplier']) => value?.name || <Empty />}
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
      {/* 跟单逾期：只显示最靠前的那个未完成节点，具体进度去详情看（T-38）。 */}
      <Table.Column
        dataIndex="overdue_node"
        title="跟单逾期"
        width={110}
        render={(value: string) =>
          value ? (
            <Tooltip title="这个节点的计划日已过但还没完成；进详情可补录">
              <Tag bordered={false} color="error">
                {value}
              </Tag>
            </Tooltip>
          ) : (
            <span className="cell-sub">—</span>
          )
        }
      />
      <Table.Column
        dataIndex="order_date"
        title="下单日"
        width={120}
        render={(value: string) => formatDate(value)}
      />
      <Table.Column
        dataIndex="delivery_date"
        title="到货日"
        width={120}
        render={(value?: string | null) => (value ? formatDate(value) : <Empty />)}
      />
      <Table.Column
        dataIndex="amount_total"
        title="采购金额"
        width={150}
        align="right"
        render={(value: number, record: PurchaseOrder) => (
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
        render={(_, record: PurchaseOrder) => (
          <Space size={4}>
            <ShowButton size="small" recordItemId={record.id} resource="purchase-orders" />
            {record.status === 10 && (
              <>
                <EditButton size="small" recordItemId={record.id} resource="purchase-orders" />
                <DeleteButton size="small" recordItemId={record.id} resource="purchase-orders" />
              </>
            )}
          </Space>
        )}
      />
    </PageList>
  )
}
