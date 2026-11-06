import type { CrudFilters } from '@refinedev/core'
import {
  DeleteButton,
  EditButton,
  ShowButton,
} from '@refinedev/antd'
import { Form, Input, Select, Space, Table } from 'antd'

import { Empty, Mono, OptionTag } from '@/components/fields'
import { PageList } from '@/components/page'
import { ORDER_STATUS_OPTIONS } from '@/constants/catalog'
import { useInfiniteList } from '@/hooks/use-infinite-feed'
import type { Shipment } from '@/types'
import { formatDate, formatQtyByUnit } from '@/utils/format'

interface ShipmentSearch {
  keyword?: string
  status?: number
}

export function ShipmentList() {
  const { tableProps, searchFormProps, feed } = useInfiniteList<Shipment>({
    resource: 'shipments',
    syncWithLocation: true,
    // 筛选表单给的是「这一次的完整条件」，所以用 replace：refine 默认 merge
    // 会把上一次的条件留着 —— 清空关键字再点查询，keyword 仍旧跟着请求走，
    // 表现是「条件清不掉、列表也不动」。永久筛选仍会保留（见 PageList 的说明）。
    filters: { defaultBehavior: 'replace' },
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
      infinite={feed}
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
        width={160}
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
        title="客户 / 来源订单"
        width={200}
        render={(_, record: Shipment) => {
          const allocations = record.allocations ?? []
          if (allocations.length === 0) return <Empty />
          // 一票通常只装一个客户的货；多客户时按出现顺序去重罗列，
          // 不缩写成「等 2 家」——认人靠的就是名字。
          const partners = [...new Set(allocations.map((a) => a.partner_name).filter(Boolean))]
          const docNos = [...new Set(allocations.map((a) => a.sales_order_doc_no).filter(Boolean))]
          return (
            <div className="cell-stack">
              <span
                className={partners.length > 0 ? 'cell-strong ellipsis-line' : 'cell-sub'}
                title={partners.join('、')}
              >
                {partners.length > 0 ? partners.join('、') : '无客户'}
              </span>
              <span className="cell-sub ellipsis-line" title={docNos.join('、')}>
                {docNos.join('、') || '—'}
              </span>
            </div>
          )
        }}
      />
      <Table.Column
        dataIndex="allocations"
        title="装了哪些货"
        render={(_, record: Shipment) => {
          const allocations = record.allocations ?? []
          if (allocations.length === 0) return <Empty />
          return (
            <span>
              {allocations.length} 行 · {formatQtyByUnit(allocations)}
            </span>
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
