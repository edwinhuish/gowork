import { useInvalidate, useNavigation, type CrudFilters } from '@refinedev/core'
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
  Typography,
} from 'antd'

import { partnerApi } from '@/api/partners'
import { Empty, EntityTypeTag, Mono, RoleTag, StatusTag } from '@/components/fields'
import { ENABLE_STATUS_OPTIONS } from '@/constants/catalog'
import type { Partner } from '@/types'

interface SupplierSearch {
  keyword?: string
  status?: number
  /** 含已删除：打开后已软删的主体也列出来，只能做一件事——恢复。 */
  include_deleted?: boolean
}

/** 供应商列表。与产品列表同一套骨架：筛选条 + 表格 + 新建入口。 */
export function SupplierList() {
  const { tableProps, searchFormProps } = useTable<Partner>({
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
      if (v.include_deleted) {
        filters.push({ field: 'include_deleted', operator: 'eq', value: true })
      }
      return filters
    },
  })

  const invalidate = useInvalidate()
  const { show } = useNavigation()

  const restore = async (record: Partner) => {
    try {
      await partnerApi.restore(record.id)
      message.success(`${record.code} 已恢复`)
      await invalidate({ resource: 'suppliers', invalidates: ['list'] })
    } catch (err) {
      message.error(err instanceof Error ? err.message : '恢复失败')
    }
  }

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

      <Table {...tableProps} rowKey="id" scroll={{ x: 990 }}>
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
        {/* 名称即入口：货源、附件与合并都在详情页，列表不给链接就只能手敲 URL。 */}
        <Table.Column
          dataIndex="name"
          title="名称"
          render={(value: string, record: Partner) => (
            <Typography.Link onClick={() => show('suppliers', record.id)}>
              <span className="cell-strong">{value}</span>
            </Typography.Link>
          )}
        />
        {/* 供应商与客户已合并为同一张表，这里会混进「客供两用」的主体。
            角色列把它标出来，免得运营以为是分录错了。 */}
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
        {/* 负责人是业务员看列表时最先找的一列：不是自己的供应商，询价前先想清楚该不该动。 */}
        <Table.Column
          dataIndex="owner_name"
          title="负责人"
          width={110}
          render={(v: string) => v || <span className="cell-sub">未分配</span>}
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
                resource="suppliers"
                recordItemId={record.id}
                confirmTitle="删除这个供应商？"
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
