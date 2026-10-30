import type { CrudFilters } from '@refinedev/core'
import { useSelect } from '@refinedev/core'
import { DeleteButton, EditButton, useTable } from '@refinedev/antd'
import { Checkbox, Form, Input, Select, Space, Table, Tag } from 'antd'

import { Empty, Mono } from '@/components/fields'
import { PageList } from '@/components/page'
import { ACTIVITY_TYPE_OPTIONS, activityTypeLabel } from '@/constants/catalog'
import type { Activity, Partner } from '@/types'
import { formatDate } from '@/utils/format'

interface ActivitySearch {
  keyword?: string
  partner_id?: number
  type?: string
  due?: boolean
}

/**
 * 跟进记录列表 —— 时间线视图。
 *
 * 「只看已到期」走查询参数 `due=1`：判定在后端按整天算（今天到期不算超期），
 * 前端不重复实现一遍日期逻辑，否则两边的"超期"会差一天。
 */
export function ActivityList() {
  const { options: partnerOptions, query: partnerQuery } = useSelect<Partner>({
    resource: 'partners',
    optionLabel: (item) => `${item.name}（${item.code}）`,
    optionValue: 'id',
    pagination: { mode: 'off' },
  })

  const { tableProps, searchFormProps } = useTable<Activity>({
    resource: 'activities',
    syncWithLocation: true,
    // 筛选表单给的是「这一次的完整条件」，所以用 replace：refine 默认 merge
    // 会把上一次的条件留着 —— 清空关键字再点查询，keyword 仍旧跟着请求走，
    // 表现是「条件清不掉、列表也不动」。永久筛选仍会保留（见 PageList 的说明）。
    filters: { defaultBehavior: 'replace' },
    onSearch: (values): CrudFilters => {
      const v = values as ActivitySearch
      const filters: CrudFilters = []
      if (v.keyword?.trim()) {
        filters.push({ field: 'keyword', operator: 'contains', value: v.keyword.trim() })
      }
      if (v.partner_id) {
        filters.push({ field: 'partner_id', operator: 'eq', value: v.partner_id })
      }
      if (v.type) {
        filters.push({ field: 'type', operator: 'eq', value: v.type })
      }
      if (v.due) {
        filters.push({ field: 'due', operator: 'eq', value: 1 })
      }
      return filters
    },
  })

  const resetFilters = () => {
    searchFormProps.form?.resetFields()
    searchFormProps.form?.submit()
  }

  const today = new Date().toISOString().slice(0, 10)

  return (
    <PageList
      createLabel="记录跟进"
      searchFormProps={searchFormProps}
      onReset={resetFilters}
      tableProps={tableProps}
      rowKey="id"
      filters={
        <>
          <Form.Item name="partner_id">
            <Select
              placeholder="全部往来单位"
              allowClear
              showSearch
              options={partnerOptions}
              loading={partnerQuery.isLoading}
              filterOption={(input, option) =>
                String(option?.label ?? '')
                  .toLowerCase()
                  .includes(input.toLowerCase())
              }
            />
          </Form.Item>
          <Form.Item name="type">
            <Select placeholder="全部方式" allowClear options={ACTIVITY_TYPE_OPTIONS} />
          </Form.Item>
          <Form.Item name="due" valuePropName="checked" initialValue={false}>
            <Checkbox>只看已到期</Checkbox>
          </Form.Item>
          <Form.Item name="keyword">
            <Input placeholder="搜索主题 / 内容" allowClear />
          </Form.Item>
        </>
      }
    >
      <Table.Column
        dataIndex="occurred_at"
        title="发生日期"
        width={120}
        render={(value: string) => <Mono>{formatDate(value)}</Mono>}
      />
      <Table.Column
        dataIndex="type"
        title="方式"
        width={90}
        render={(value: string) => <Tag bordered={false}>{activityTypeLabel(value)}</Tag>}
      />
      <Table.Column dataIndex="subject" title="主题" width={220} />
      <Table.Column
        dataIndex="partner"
        title="往来单位"
        width={180}
        render={(value: Activity['partner']) => (value ? value.name : <Empty />)}
      />
      <Table.Column
        dataIndex="contact"
        title="联系人"
        width={120}
        render={(value: Activity['contact']) => (value ? value.name : <Empty />)}
      />
      <Table.Column
        dataIndex="next_follow_at"
        title="下次跟进"
        width={130}
        render={(value: string | null) => {
          if (!value) return <Empty />
          const overdue = value.slice(0, 10) < today
          return (
            <Tag bordered={false} color={overdue ? 'error' : 'default'}>
              {formatDate(value)}
            </Tag>
          )
        }}
      />
      <Table.Column
        title="操作"
        width={140}
        render={(_, record: Activity) => (
          <Space size={4}>
            <EditButton size="small" recordItemId={record.id} resource="activities" />
            <DeleteButton size="small" recordItemId={record.id} resource="activities" />
          </Space>
        )}
      />
    </PageList>
  )
}
