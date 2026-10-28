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
import { QUOTATION_STATUS_OPTIONS, quotationStatusLabel, tradeTermLabel } from '@/constants/catalog'
import type { Partner, Quotation } from '@/types'
import { formatDate, formatPercent } from '@/utils/format'

interface QuotationSearch {
  keyword?: string
  partner_id?: number
  status?: number
}

/** 状态色：草稿是待办、已确认是承诺、已作废只是留档。 */
function statusColor(value: number): string {
  if (value === 20) return 'success'
  if (value === 90) return 'default'
  return 'processing'
}

/**
 * 报价单列表。
 *
 * 列表只给表头，不带明细：一页 20 张单、每张十几行，全都带出来只为显示一个毛利率不划算。
 * 金额与毛利率是保存时锁定的快照，这里直接展示，不再重算。
 */
export function QuotationList() {
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
    <List headerButtons={<CreateButton>新建报价单</CreateButton>}>
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
          <Select placeholder="全部状态" allowClear style={{ width: 130 }} options={QUOTATION_STATUS_OPTIONS} />
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

      <Table {...tableProps} rowKey="id" scroll={{ x: 1180 }}>
        <Table.Column
          dataIndex="doc_no"
          title="单号"
          width={170}
          render={(value: string, record: Quotation) => (
            <div className="cell-stack">
              <Mono>{value}</Mono>
              <span className="cell-sub">v{record.version}</span>
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
          render={(value: string) => <Tag>{tradeTermLabel(value)}</Tag>}
        />
        <Table.Column
          dataIndex="status"
          title="状态"
          width={100}
          render={(value: number) => (
            <Tag color={statusColor(value)}>{quotationStatusLabel(value)}</Tag>
          )}
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
              <Tag>{record.currency_code}</Tag>
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
            <span style={{ color: value < 0 ? '#cf1322' : undefined }}>
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
            <span style={{ color: value < 0 ? '#cf1322' : undefined }}>{formatPercent(value)}</span>
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
      </Table>
    </List>
  )
}
