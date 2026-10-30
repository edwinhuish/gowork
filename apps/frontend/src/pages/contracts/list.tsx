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
import {
  CONTRACT_STATUS_OPTIONS,
  CONTRACT_TYPE_OPTIONS,
  contractTypeLabel,
} from '@/constants/catalog'
import type { Contract, Partner } from '@/types'
import { formatDate } from '@/utils/format'

interface ContractSearch {
  keyword?: string
  type?: number
  partner_id?: number
  status?: number
}

/**
 * 合同列表：PI 与销售合同同一张表，按 type 区分。
 *
 * 列表不带分期（后端有意不预加载）：一页几十份、每份几期，
 * 带出来只为显示"分几期"不值得。合计是生成时锁定的快照，这里直接展示。
 */
export function ContractList() {
  const { options: customerOptions, query: customerQuery } = useSelect<Partner>({
    resource: 'customers',
    optionLabel: (item) => `${item.name}（${item.code}）`,
    optionValue: 'id',
    pagination: { mode: 'off' },
  })

  const { tableProps, searchFormProps } = useTable<Contract>({
    resource: 'contracts',
    syncWithLocation: true,
    // 筛选表单给的是「这一次的完整条件」，所以用 replace：refine 默认 merge
    // 会把上一次的条件留着 —— 清空关键字再点查询，keyword 仍旧跟着请求走，
    // 表现是「条件清不掉、列表也不动」。永久筛选仍会保留（见 PageList 的说明）。
    filters: { defaultBehavior: 'replace' },
    onSearch: (values): CrudFilters => {
      const v = values as ContractSearch
      const filters: CrudFilters = []
      if (v.keyword?.trim()) {
        filters.push({ field: 'keyword', operator: 'contains', value: v.keyword.trim() })
      }
      if (v.type) {
        filters.push({ field: 'type', operator: 'eq', value: v.type })
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
      createLabel="生成合同"
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
            <Select placeholder="全部类型" allowClear options={CONTRACT_TYPE_OPTIONS} />
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
            <Select placeholder="全部状态" allowClear options={CONTRACT_STATUS_OPTIONS} />
          </Form.Item>
        </>
      }
    >
      <Table.Column
        dataIndex="doc_no"
        title="单号"
        width={170}
        render={(value: string, record: Contract) => (
          <div className="cell-stack">
            <Mono>{value}</Mono>
            <span className="cell-sub">{contractTypeLabel(record.type)}</span>
          </div>
        )}
      />
      <Table.Column
        dataIndex="partner"
        title="客户"
        width={180}
        render={(value: Contract['partner']) => value?.name || <Empty />}
      />
      <Table.Column
        dataIndex="source_doc_no"
        title="来源单据"
        width={170}
        render={(value: string, record: Contract) => (
          <div className="cell-stack">
            <Mono>{value}</Mono>
            <span className="cell-sub">
              {record.source_type === 20 ? '销售订单' : '报价单'}
            </span>
          </div>
        )}
      />
      <Table.Column
        dataIndex="status"
        title="状态"
        width={100}
        render={(value: number) => <OptionTag value={value} options={CONTRACT_STATUS_OPTIONS} />}
      />
      <Table.Column
        dataIndex="sign_date"
        title="签订日"
        width={120}
        render={(value: string) => formatDate(value)}
      />
      <Table.Column
        dataIndex="amount_total"
        title="合同额"
        width={150}
        align="right"
        render={(value: number, record: Contract) => (
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
        title="操作"
        width={200}
        render={(_, record: Contract) => (
          <Space size={4}>
            <ShowButton size="small" recordItemId={record.id} resource="contracts" />
            {record.status === 10 && (
              <>
                <EditButton size="small" recordItemId={record.id} resource="contracts" />
                <DeleteButton size="small" recordItemId={record.id} resource="contracts" />
              </>
            )}
          </Space>
        )}
      />
    </PageList>
  )
}
