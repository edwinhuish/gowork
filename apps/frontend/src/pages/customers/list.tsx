import { useInvalidate, type CrudFilters } from '@refinedev/core'
import { CreateButton, DeleteButton, List, useTable } from '@refinedev/antd'
import {
  Button,
  Checkbox,
  Form,
  Input,
  message,
  Popconfirm,
  Select,
  Space,
  Table,
  Tag,
} from 'antd'

import { partnerApi } from '@/api/partners'
import { Empty, EntityTypeTag, Mono, RoleTag, StatusTag } from '@/components/fields'
import { ENABLE_STATUS_OPTIONS } from '@/constants/catalog'
import type { Partner } from '@/types'

interface CustomerSearch {
  keyword?: string
  country?: string
  status?: number
  /** 含已删除：打开后已软删的主体也列出来，只能做一件事——恢复。 */
  include_deleted?: boolean
}

/**
 * 客户列表。
 *
 * 国家是自由文本（后端也是 string），所以给输入框而不是下拉 ——
 * 做成下拉就得先维护一份国家字典，当前阶段没必要。
 */
export function CustomerList() {
  const { tableProps, searchFormProps } = useTable<Partner>({
    resource: 'customers',
    syncWithLocation: true,
    onSearch: (values): CrudFilters => {
      const v = values as CustomerSearch
      const filters: CrudFilters = []
      if (v.keyword?.trim()) {
        filters.push({ field: 'keyword', operator: 'contains', value: v.keyword.trim() })
      }
      if (v.country?.trim()) {
        filters.push({ field: 'country', operator: 'contains', value: v.country.trim() })
      }
      if (v.status !== undefined && v.status !== null) {
        filters.push({ field: 'status', operator: 'eq', value: v.status })
      }
      if (v.include_deleted) {
        filters.push({ field: 'include_deleted', operator: 'eq', value: true })
      }
      return filters
    },
  })

  const invalidate = useInvalidate()

  const restore = async (record: Partner) => {
    try {
      await partnerApi.restore(record.id)
      message.success(`${record.code} 已恢复`)
      await invalidate({ resource: 'customers', invalidates: ['list'] })
    } catch (err) {
      message.error(err instanceof Error ? err.message : '恢复失败')
    }
  }

  const resetFilters = () => {
    searchFormProps.form?.resetFields()
    searchFormProps.form?.submit()
  }

  return (
    <List headerButtons={<CreateButton>新建客户</CreateButton>}>
      <Form {...searchFormProps} layout="inline" className="list-filters">
        <Form.Item name="keyword">
          <Input placeholder="搜索编码 / 名称 / 联系人" allowClear style={{ width: 240 }} />
        </Form.Item>
        <Form.Item name="country">
          <Input placeholder="国家 / 地区" allowClear style={{ width: 150 }} />
        </Form.Item>
        <Form.Item name="status">
          <Select placeholder="全部状态" allowClear options={ENABLE_STATUS_OPTIONS} style={{ width: 120 }} />
        </Form.Item>
        <Form.Item name="include_deleted" valuePropName="checked">
          <Checkbox>含已删除</Checkbox>
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

      <Table {...tableProps} rowKey="id" scroll={{ x: 1210 }}>
        <Table.Column
          dataIndex="code"
          title="编码"
          width={120}
          render={(value: string, record: Partner) => (
            <Space size={4}>
              <Mono>{value}</Mono>
              {record.deleted_at ? <Tag bordered={false}>已删除</Tag> : null}
            </Space>
          )}
        />
        <Table.Column
          dataIndex="name"
          title="名称"
          render={(value: string) => <span className="cell-strong">{value}</span>}
        />
        {/* 客户列表里也会出现「客供两用」的主体，角色列让它不至于看起来像脏数据。 */}
        <Table.Column
          dataIndex="role"
          title="角色"
          width={140}
          render={(value: number) => <RoleTag value={value} />}
        />
        <Table.Column
          dataIndex="entity_type"
          title="类型"
          width={80}
          render={(value: number) => <EntityTypeTag value={value} />}
        />
        <Table.Column dataIndex="country" title="国家/地区" width={130} render={(v: string) => v || <Empty />} />
        {/* 负责人是业务员看列表时最先找的一列：不是自己的客户，跟进前先想清楚该不该动。 */}
        <Table.Column
          dataIndex="owner_name"
          title="负责人"
          width={110}
          render={(v: string) => v || <span className="cell-sub">未分配</span>}
        />
        <Table.Column dataIndex="contact" title="联系人" width={120} render={(v: string) => v || <Empty />} />
        <Table.Column dataIndex="phone" title="电话" width={140} render={(v: string) => v || <Empty />} />
        {/* 结算方式与账期直接影响"这单能不能赊"，放在列表里便于接单前先看一眼。 */}
        <Table.Column
          dataIndex="settlement_method"
          title="结算方式"
          width={110}
          render={(v: string) => v || <Empty />}
        />
        <Table.Column
          dataIndex="credit_days"
          title="账期"
          width={90}
          align="right"
          render={(v: number) => (v ? `${v} 天` : '款到发货')}
        />
        <Table.Column
          dataIndex="status"
          title="状态"
          width={90}
          render={(value: number) => <StatusTag value={value} />}
        />
        <Table.Column
          title="操作"
          width={90}
          render={(_, record: Partner) =>
            record.deleted_at ? (
              <Popconfirm
                title="恢复这个主体？"
                description="只去掉删除标记，删除期间改过的引用不会自动指回来。"
                okText="恢复"
                cancelText="返回"
                onConfirm={() => restore(record)}
              >
                <Button size="small" type="link">
                  恢复
                </Button>
              </Popconfirm>
            ) : (
              <DeleteButton
                size="small"
                resource="customers"
                recordItemId={record.id}
                confirmTitle="删除这个客户？"
                confirmOkText="删除"
                confirmCancelText="返回"
              />
            )
          }
        />
      </Table>
    </List>
  )
}
