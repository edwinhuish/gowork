import type { CrudFilters } from '@refinedev/core'
import { CreateButton, List, useTable } from '@refinedev/antd'
import { Button, Form, Input, Select, Space, Table } from 'antd'

import { Empty, Mono, StatusTag } from '@/components/fields'
import { STATUS_OPTIONS } from '@/constants/catalog'
import type { Supplier } from '@/types'

interface SupplierSearch {
  keyword?: string
  status?: number
}

/** 供应商列表。与产品列表同一套骨架：筛选条 + 表格 + 新建入口。 */
export function SupplierList() {
  const { tableProps, searchFormProps } = useTable<Supplier>({
    resource: 'suppliers',
    syncWithLocation: true,
    onSearch: (values): CrudFilters => {
      const v = values as SupplierSearch
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
    <List headerButtons={<CreateButton>新建供应商</CreateButton>}>
      <Form {...searchFormProps} layout="inline" className="list-filters">
        <Form.Item name="keyword">
          <Input placeholder="搜索编码 / 名称 / 联系人" allowClear style={{ width: 260 }} />
        </Form.Item>
        <Form.Item name="status">
          <Select placeholder="全部状态" allowClear options={STATUS_OPTIONS} style={{ width: 120 }} />
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

      <Table {...tableProps} rowKey="id" scroll={{ x: 900 }}>
        <Table.Column
          dataIndex="code"
          title="编码"
          width={120}
          render={(value: string) => <Mono>{value}</Mono>}
        />
        <Table.Column
          dataIndex="name"
          title="名称"
          render={(value: string) => <span className="cell-strong">{value}</span>}
        />
        <Table.Column dataIndex="contact" title="联系人" width={120} render={(v: string) => v || <Empty />} />
        <Table.Column dataIndex="phone" title="电话" width={140} render={(v: string) => v || <Empty />} />
        <Table.Column dataIndex="email" title="邮箱" width={200} render={(v: string) => v || <Empty />} />
        <Table.Column
          dataIndex="status"
          title="状态"
          width={90}
          render={(value: number) => <StatusTag value={value} />}
        />
      </Table>
    </List>
  )
}
