import { useState } from 'react'
import { Link } from 'react-router'

import type { CrudFilters, BaseRecord } from '@refinedev/core'
import { useInvalidate, useSelect } from '@refinedev/core'
import { useTable } from '@refinedev/antd'
import { Button, Checkbox, Form, Input, message, Select, Space, Table } from 'antd'

import { AccentTag, Empty, Mono } from '@/components/fields'
import { PageList } from '@/components/page'
import { http } from '@/api/client'
import type { Contact, Partner } from '@/types'

interface ContactSearch {
  keyword?: string
  partner_id?: number
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

  const [showDeleted, setShowDeleted] = useState(false)
  const { tableProps, searchFormProps, setFilters } = useTable<Contact>({
    resource: 'contacts',
    syncWithLocation: true,
    // 筛选表单给的是「这一次的完整条件」，所以用 replace：refine 默认 merge
    // 会把上一次的条件留着 —— 清空关键字再点查询，keyword 仍旧跟着请求走，
    // 表现是「条件清不掉、列表也不动」。永久筛选仍会保留（见 PageList 的说明）。
    filters: { defaultBehavior: 'replace' },
    onSearch: (values): CrudFilters => {
      const v = values as ContactSearch
      const filters: CrudFilters = []
      if (v.keyword?.trim()) {
        filters.push({ field: 'keyword', operator: 'contains', value: v.keyword.trim() })
      }
      if (v.partner_id !== undefined && v.partner_id !== null) {
        filters.push({ field: 'partner_id', operator: 'eq', value: v.partner_id })
      }
      return filters
    },
  })

  // 「含已删除」不走 searchForm：它不是一个要手填的条件，
  // 开了就总能看见已删除的行，关了就一定看不见。
  //
  // 在**事件里**发而不是 useEffect 里：refine 的 setFilters 依赖
  // filters.permanent 这个数组，哪天这页加上 permanent（内联字面量，每次渲染
  // 都是新数组），setFilters 就每次都是新引用 —— 放进 effect 依赖里会自激成
  // "Maximum update depth exceeded"（产品货源面板实测踩过）。开关是事件。
  const toggleDeleted = (checked: boolean) => {
    setShowDeleted(checked)
    setFilters(
      checked ? [{ field: 'include_deleted', operator: 'eq', value: true }] : [],
      'replace',
    )
  }

  const invalidate = useInvalidate()
  const restore = async (id: number) => {
    try {
      await http.put<BaseRecord>(`/contacts/${id}/restore`)
      await invalidate({ resource: 'contacts', invalidates: ['list'] })
    } catch (err) {
      message.error(err instanceof Error ? err.message : '恢复失败')
    }
  }

  const resetFilters = () => {
    searchFormProps.form?.resetFields()
    searchFormProps.form?.submit()
  }

  return (
    <PageList
      createLabel="新建联系人"
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
          <Form.Item name="keyword">
            <Input placeholder="搜索姓名 / 电话 / 邮箱" allowClear />
          </Form.Item>
        </>
      }
      extra={
        <Checkbox
          checked={showDeleted}
          onChange={(e) => toggleDeleted(e.target.checked)}
        >
          含已删除
        </Checkbox>
      }
    >
      <Table.Column
        dataIndex="name"
        title="姓名"
        width={160}
        render={(value: string, record: Contact) => (
          <Space size={6}>
            <span className="cell-strong">{value}</span>
            {record.is_primary && <AccentTag>主</AccentTag>}
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
        title="操作"
        width={90}
        render={(_, record: Contact) =>
          // 联系人会换工作、换号码，改不了就只能删了重建 —— 而重建会丢掉
          // 它在单据上的关联。编辑页和路由早就有了，这里就是它的入口。
          record.deleted_at ? (
            <Button size="small" type="link" onClick={() => restore(record.id)}>
              恢复
            </Button>
          ) : (
            <Link to={`/contacts/edit/${record.id}`}>编辑</Link>
          )
        }
      />
    </PageList>
  )
}
