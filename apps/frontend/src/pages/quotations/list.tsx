import type { CrudFilters } from '@refinedev/core'
import { useSelect } from '@refinedev/core'
import {
  DeleteButton,
  EditButton,
  ShowButton,
  useTable,
} from '@refinedev/antd'
import { Form, Input, Select, Space, Table, Tag, theme } from 'antd'

import { Empty, Mono, Money, OptionTag } from '@/components/fields'
import { PageList } from '@/components/page'
import { QUOTATION_STATUS_OPTIONS, tradeTermLabel } from '@/constants/catalog'
import type { Partner, Quotation } from '@/types'
import { formatDate, formatPercent } from '@/utils/format'

interface QuotationSearch {
  keyword?: string
  partner_id?: number
  status?: number
}

/**
 * 报价单列表。
 *
 * 列表只给表头，不带明细：一页 20 张单、每张十几行，全都带出来只为显示一个毛利率不划算。
 * 金额与毛利率是保存时锁定的快照，这里直接展示，不再重算。
 */
export function QuotationList() {
  const { token } = theme.useToken()
  const { options: customerOptions, query: customerQuery } = useSelect<Partner>({
    resource: 'customers',
    optionLabel: (item) => `${item.name}（${item.code}）`,
    optionValue: 'id',
    pagination: { mode: 'off' },
  })

  const { tableProps, searchFormProps } = useTable<Quotation>({
    resource: 'quotations',
    syncWithLocation: true,
    onSearch: (values): CrudFilters => {
      const v = values as QuotationSearch
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
      createLabel="新建报价单"
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
            <Select placeholder="全部状态" allowClear options={QUOTATION_STATUS_OPTIONS} />
          </Form.Item>
        </>
      }
    >
      <Table.Column
        dataIndex="doc_no"
        title="单号"
        width={170}
        render={(value: string, record: Quotation) => (
          <div className="cell-stack">
            <Mono>{value}</Mono>
            <span className="cell-sub">
              v{record.version}
              {record.sales_order_id ? ' · 已转单' : ''}
            </span>
          </div>
        )}
      />
      <Table.Column
        dataIndex="partner"
        title="客户"
        width={190}
        render={(value: Quotation['partner']) => value?.name || <Empty />}
      />
      <Table.Column
        dataIndex="trade_term"
        title="贸易术语"
        width={120}
        render={(value: string) => <Tag bordered={false}>{tradeTermLabel(value)}</Tag>}
      />
      <Table.Column
        dataIndex="status"
        title="状态"
        width={100}
        render={(value: number) => <OptionTag value={value} options={QUOTATION_STATUS_OPTIONS} />}
      />
      <Table.Column
        dataIndex="valid_until"
        title="有效期至"
        width={120}
        render={(value: string) => formatDate(value)}
      />
      <Table.Column
        dataIndex="amount_total"
        title="报价额"
        width={140}
        align="right"
        render={(value: number, record: Quotation) => (
          <Space size={4}>
            <Money value={value} />
            <Tag bordered={false}>{record.currency_code}</Tag>
          </Space>
        )}
      />
      <Table.Column
        dataIndex="amount_base_total"
        title="折本币"
        width={120}
        align="right"
        render={(value: number) => <Money value={value} />}
      />
      <Table.Column
        dataIndex="gross_profit"
        title="毛利"
        width={110}
        align="right"
        render={(value: number) => (
          <span style={{ color: value < 0 ? token.colorError : undefined }}>
            <Money value={value} />
          </span>
        )}
      />
      <Table.Column
        dataIndex="margin_rate"
        title="毛利率"
        width={100}
        align="right"
        render={(value: number) => (
          <span style={{ color: value < 0 ? token.colorError : undefined }}>
            {formatPercent(value)}
          </span>
        )}
      />
      <Table.Column
        title="操作"
        width={200}
        render={(_, record: Quotation) => (
          <Space size={4}>
            <ShowButton size="small" recordItemId={record.id} resource="quotations" />
            {record.status === 10 && (
              <>
                <EditButton size="small" recordItemId={record.id} resource="quotations" />
                <DeleteButton size="small" recordItemId={record.id} resource="quotations" />
              </>
            )}
          </Space>
        )}
      />
    </PageList>
  )
}
