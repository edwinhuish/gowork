import { Link } from 'react-router'
import { AppstoreOutlined, BarsOutlined } from '@ant-design/icons'
import { useInvalidate, useNavigation, type CrudFilters } from '@refinedev/core'
import { CreateButton, useTable } from '@refinedev/antd'
import {
  Button,
  Checkbox,
  Form,
  Input,
  message,
  Segmented,
  Space,
  Table,
  Tag,
} from 'antd'

import { partnerApi } from '@/api/partners'
import { Empty, EntityTypeTag, Mono, RoleTag } from '@/components/fields'
import { EmptyGuide, PageList } from '@/components/page'
import { PartnerAvatar } from '@/components/partner-avatar'
import {
  PartnerCards,
  PartnerRowActions,
  PARTNER_CARD_PAGE_SIZES,
  toCardPager,
} from '@/components/partner-cards'
import { useListView } from '@/hooks/use-list-view'
import type { Partner } from '@/types'

interface ForwarderSearch {
  keyword?: string
  /** 含已删除：打开后已软删的主体也列出来，只能做一件事——恢复。 */
  include_deleted?: boolean
}

/**
 * 货代列表。与供应商列表同一套骨架：筛选条 + 表格 + 新建入口。
 *
 * 保留角色列：同一家既做货代又做客户是常见的（客户自己有进口资质、
 * 顺手帮我们订舱），行上不标出来会让人以为串了数据。
 *
 * 删除与恢复与其它两张主体列表同一套写法（同一个 `PartnerRowActions`）：
 * 主体没有启用状态了，不再来往的货代只能靠软删退出下拉，
 * 少给这张列表一个入口，就等于让它永远退不掉。
 */
export function ForwarderList() {
  const { show } = useNavigation()
  // 窄屏强制卡片视图（表格在这个宽度上只能横滑），切换控件随之隐藏。
  const { view, setView, narrow } = useListView('gowork:forwarders:view')
  const { tableProps, searchFormProps } = useTable<Partner>({
    resource: 'forwarders',
    syncWithLocation: true,
    pagination: { pageSize: Number(PARTNER_CARD_PAGE_SIZES[0]) },
    onSearch: (values): CrudFilters => {
      const v = values as ForwarderSearch
      const filters: CrudFilters = []
      if (v.keyword?.trim()) {
        filters.push({ field: 'keyword', operator: 'contains', value: v.keyword.trim() })
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
      await invalidate({ resource: 'forwarders', invalidates: ['list'] })
    } catch (err) {
      message.error(err instanceof Error ? err.message : '恢复失败')
    }
  }

  const resetFilters = () => {
    searchFormProps.form?.resetFields()
    searchFormProps.form?.submit()
  }

  const rows = (tableProps.dataSource ?? []) as Partner[]
  const cardView = (
    <PartnerCards
      rows={rows}
      loading={tableProps.loading}
      pager={toCardPager(tableProps, (total) => `共 ${total} 个货代`)}
      onShow={(record) => show('forwarders', record.id)}
      renderActions={(record) => (
        <PartnerRowActions
          record={record}
          resource="forwarders"
          entityLabel="货代"
          onRestore={(r) => void restore(r)}
        />
      )}
      empty={
        <EmptyGuide
          title="没有符合条件的货代"
          description="换个筛选条件，或者直接新建一个货代"
          action={
            <Space>
              <Button onClick={resetFilters}>重置筛选</Button>
              <CreateButton>新建货代</CreateButton>
            </Space>
          }
        />
      }
    />
  )

  return (
    <PageList
      createLabel="新建货代"
      searchFormProps={searchFormProps}
      onReset={resetFilters}
      tableProps={tableProps}
      rowKey="id"
      pageSizeOptions={PARTNER_CARD_PAGE_SIZES}
      table={view === 'card' ? cardView : undefined}
      extra={
        narrow ? null : (
          <Segmented
            value={view}
            onChange={(value) => setView(value as 'list' | 'card')}
            options={[
              { value: 'list', icon: <BarsOutlined />, title: '列表视图：一行一家，字段最全' },
              { value: 'card', icon: <AppstoreOutlined />, title: '卡片视图：按 Logo 与名称认人' },
            ]}
            aria-label="切换货代视图"
          />
        )
      }
      filters={
        <>
          <Form.Item name="keyword">
            <Input placeholder="搜索编码 / 名称 / 联系人" allowClear />
          </Form.Item>
          <Form.Item name="include_deleted" valuePropName="checked">
            <Checkbox>含已删除</Checkbox>
          </Form.Item>
        </>
      }
    >
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
      {/* 头像与卡片视图同组件同尺寸：换视图不能换长相。 */}
      <Table.Column
        dataIndex="name"
        title="名称"
        render={(value: string, record: Partner) => (
          <div className="cell-avatar">
            <PartnerAvatar name={record.name} logoUrl={record.logo_url} />
            {/* 见客户列表：真链接，中键 / 右键「在新标签打开」才可用。 */}
            <Link to={`/forwarders/show/${record.id}`}>
              <span className="cell-strong">{value}</span>
            </Link>
          </div>
        )}
      />
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
        title="操作"
        width={90}
        render={(_, record: Partner) => (
          <PartnerRowActions
            record={record}
            resource="forwarders"
            entityLabel="货代"
            onRestore={(r) => void restore(r)}
          />
        )}
      />
    </PageList>
  )
}
