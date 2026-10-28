import type { CrudFilters } from '@refinedev/core'
import { useSelect } from '@refinedev/core'
import { CreateButton, List, useTable } from '@refinedev/antd'
import { Button, Form, Input, Select, Space, Table, Tag } from 'antd'

import { Empty, Mono, StatusTag } from '@/components/fields'
import { ENABLE_STATUS_OPTIONS } from '@/constants/catalog'
import type { Contact, Partner } from '@/types'

interface ContactSearch {
  keyword?: string
  partner_id?: number
  status?: number
}

/**
 * 联系人列表。
 *
 * 主体筛选放在第一位：联系人的第一层归属永远是"哪家的谁"，
 * 按姓名搜只在已知名字时有用，而更多时候是"这家客户有哪些人".
 */
export function ContactList() {
  const { options: partnerOptions, query: partnerQuery } = useSelect<Partner>({
    resource: 'partners',
    optionLabel: (item) => `${item.name}（${item.code}）`,
    optionValue: 'id',
    pagination: { mode: 'off' },
  })

  const { tableProps, searchFormProps } = useTable<Contact>({
    resource: 'contacts',
    syncWithLocation: true,
    onSearch: (values): CrudFilters => {
      const v = values as ContactSearch
      const filters: CrudFilters = []
      if (v.keyword?.trim()) {
        filters.push({ field: 'keyword', operator: 'contains', value: v.keyword.trim() })
      }
      if (v.partner_id !== undefined && v.partner_id !== null) {
        filters.push({ field: 'partner_id', operator: 'eq', value: v.partner_id })
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
    <List headerButtons={<CreateButton>新建联系人</CreateButton>}>
      <Form {...searchFormProps} layout="inline" className="list-filters">
        <Form.Item name="partner_id">
          <Select
            placeholder="全部往来单位"
            allowClear
            showSearch
            style={{ width: 260 }}
            options={partnerOptions}
            loading={partnerQuery.isLoading}
            filterOption={(input, option) =>
              String(option?.label ?? '')
                .toLowerCase()
                .includes(input.toLowerCase())
            }
          />
        </Form.Item>
        <Form.Item name="keyword">
          <Input placeholder="搜索姓名 / 电话 / 邮箱" allowClear style={{ width: 220 }} />
        </Form.Item>
        <Form.Item name="status">
          <Select placeholder="全部状态" allowClear options={ENABLE_STATUS_OPTIONS} style={{ width: 120 }} />
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

      <Table {...tableProps} rowKey="id" scroll={{ x: 1000 }}>
        <Table.Column
          dataIndex="name"
          title="姓名"
          width={160}
          render={(value: string, record: Contact) => (
            <Space size={6}>
              <span className="cell-strong">{value}</span>
              {record.is_primary && <Tag color="gold">主</Tag>}
            </Space>
          )}
        />
        <Table.Column
          dataIndex="partner"
          title="往来单位"
          width={220}
          render={(value: Contact['partner']) =>
            value ? (
              <div className="cell-stack">
                <span>{value.name}</span>
                <span className="cell-sub cell-mono">{value.code}</span>
              </div>
            ) : (
              <Empty />
            )
          }
        />
        <Table.Column dataIndex="title" title="职位" width={120} render={(v: string) => v || <Empty />} />
        <Table.Column dataIndex="phone" title="电话" width={150} render={(v: string) => <Mono>{v}</Mono>} />
        <Table.Column dataIndex="email" title="邮箱" render={(v: string) => v || <Empty />} />
        <Table.Column dataIndex="language" title="语言" width={110} render={(v: string) => v || <Empty />} />
        <Table.Column dataIndex="channel" title="偏好渠道" width={110} render={(v: string) => v || <Empty />} />
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
