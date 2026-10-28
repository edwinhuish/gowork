import type { CrudFilters } from '@refinedev/core'
import { useSelect } from '@refinedev/core'
import { CreateButton, DeleteButton, List, useTable } from '@refinedev/antd'
import { Button, Form, Select, Space, Table } from 'antd'

import { Empty, Mono } from '@/components/fields'
import type { Currency, ExchangeRate } from '@/types'
import { formatDate } from '@/utils/format'

interface RateSearch {
  currency_code?: string
}

/**
 * 汇率列表。
 *
 * 必须带币种筛选：不分币种地列全部汇率是一屏几十条噪声，
 * 而且不同币种的数字量级差一个数量级，并排看毫无参照意义。
 * 没有编辑按钮 —— 历史汇率不可改，录错删掉重录。
 */
export function ExchangeRateList() {
  const { options: currencyOptions, query: currencyQuery } = useSelect<Currency>({
    resource: 'currencies',
    optionLabel: (item) => `${item.code} · ${item.name}`,
    optionValue: 'code',
    pagination: { mode: 'off' },
  })

  const { tableProps, searchFormProps } = useTable<ExchangeRate>({
    resource: 'exchange-rates',
    syncWithLocation: true,
    onSearch: (values): CrudFilters => {
      const v = values as RateSearch
      const filters: CrudFilters = []
      if (v.currency_code) {
        filters.push({ field: 'currency_code', operator: 'eq', value: v.currency_code })
      }
      return filters
    },
  })

  const resetFilters = () => {
    searchFormProps.form?.resetFields()
    searchFormProps.form?.submit()
  }

  return (
    <List headerButtons={<CreateButton>登记汇率</CreateButton>}>
      <Form {...searchFormProps} layout="inline" className="list-filters">
        <Form.Item name="currency_code">
          <Select
            placeholder="全部币种"
            allowClear
            style={{ width: 200 }}
            options={currencyOptions}
            loading={currencyQuery.isLoading}
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

      <Table {...tableProps} rowKey="id" scroll={{ x: 720 }}>
        <Table.Column
          dataIndex="effective_date"
          title="生效日期"
          width={130}
          render={(value: string) => <Mono>{formatDate(value)}</Mono>}
        />
        <Table.Column
          dataIndex="currency_code"
          title="折算"
          width={240}
          render={(value: string, record: ExchangeRate) => (
            <span className="cell-mono">
              1 {value} = {record.rate} {record.base_code}
            </span>
          )}
        />
        <Table.Column
          dataIndex="rate"
          title="汇率"
          width={140}
          align="right"
          render={(value: number) => (
            <span style={{ fontVariantNumeric: 'tabular-nums' }}>{value}</span>
          )}
        />
        <Table.Column
          dataIndex="source"
          title="来源"
          width={140}
          render={(value: string) => value || <Empty />}
        />
        <Table.Column
          dataIndex="remark"
          title="备注"
          render={(value: string) => value || <Empty />}
        />
        <Table.Column
          title="操作"
          width={110}
          render={(_, record: ExchangeRate) => (
            <DeleteButton size="small" recordItemId={record.id} resource="exchange-rates" />
          )}
        />
      </Table>
    </List>
  )
}
