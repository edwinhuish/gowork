import type { CrudFilters } from '@refinedev/core'
import { useTable } from '@refinedev/antd'
import { Form, Input, Select, Table, Tag } from 'antd'

import { Empty, Mono, StatusTag } from '@/components/fields'
import { PageList } from '@/components/page'
import { ENABLE_STATUS_OPTIONS } from '@/constants/catalog'
import type { Currency } from '@/types'

interface CurrencySearch {
  keyword?: string
  status?: number
}

/**
 * 币种列表。
 *
 * 「本币」单独一列而不是并进状态：本币不是"更重要的启用"，
 * 它是折算的基准，改了它所有新单据的本币金额口径都跟着变。
 */
export function CurrencyList() {
  const { tableProps, searchFormProps } = useTable<Currency>({
    resource: 'currencies',
    syncWithLocation: true,
    onSearch: (values): CrudFilters => {
      const v = values as CurrencySearch
      const filters: CrudFilters = []
      if (v.keyword?.trim()) {
        filters.push({ field: 'keyword', operator: 'contains', value: v.keyword.trim() })
      }
      if (v.status !== undefined && v.status !== null) {
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
      createLabel="新建币种"
      searchFormProps={searchFormProps}
      onReset={resetFilters}
      tableProps={tableProps}
      rowKey="id"
      filters={
        <>
          <Form.Item name="keyword">
            <Input placeholder="搜索代码 / 名称" allowClear />
          </Form.Item>
          <Form.Item name="status">
            <Select placeholder="全部状态" allowClear options={ENABLE_STATUS_OPTIONS} />
          </Form.Item>
        </>
      }
    >
      <Table.Column
        dataIndex="code"
        title="代码"
        width={100}
        render={(value: string) => <Mono>{value}</Mono>}
      />
      <Table.Column dataIndex="name" title="名称" width={160} />
      <Table.Column
        dataIndex="symbol"
        title="符号"
        width={80}
        render={(value: string) => value || <Empty />}
      />
      <Table.Column dataIndex="decimal_places" title="小数位" width={90} align="right" />
      <Table.Column
        dataIndex="is_base"
        title="本币"
        width={90}
        render={(value: boolean) => (value ? <Tag color="gold">本币</Tag> : <Empty />)}
      />
      <Table.Column
        dataIndex="status"
        title="状态"
        width={90}
        render={(value: number) => <StatusTag value={value} />}
      />
      <Table.Column
        dataIndex="remark"
        title="备注"
        render={(value: string) => value || <Empty />}
      />
    </PageList>
  )
}
