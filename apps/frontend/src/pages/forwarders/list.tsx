import { AppstoreOutlined, BarsOutlined } from '@ant-design/icons'
import { useNavigation, type CrudFilters } from '@refinedev/core'
import { CreateButton, useTable } from '@refinedev/antd'
import { Button, Form, Input, Segmented, Space, Table, Typography } from 'antd'

import { Empty, EntityTypeTag, Mono, RoleTag } from '@/components/fields'
import { EmptyGuide, PageList } from '@/components/page'
import { PartnerAvatar } from '@/components/partner-avatar'
import { PartnerCards, PARTNER_CARD_PAGE_SIZES, toCardPager } from '@/components/partner-cards'
import { useListView } from '@/hooks/use-list-view'
import type { Partner } from '@/types'

interface ForwarderSearch {
  keyword?: string
}

/**
 * 货代列表。与供应商列表同一套骨架：筛选条 + 表格 + 新建入口。
 *
 * 保留角色列：同一家既做货代又做客户是常见的（客户自己有进口资质、
 * 顺手帮我们订舱），行上不标出来会让人以为串了数据。
 */
export function ForwarderList() {
  const { show } = useNavigation()
  const { view, setView } = useListView('gowork:forwarders:view')
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
      return filters
    },
  })

  const resetFilters = () => {
    searchFormProps.form?.resetFields()
    searchFormProps.form?.submit()
  }

  const rows = (tableProps.dataSource ?? []) as Partner[]
  // 货代卡不给增删改入口：这张列表本来也就没有操作列（货代只被单据引用，
  // 不存在"还没有单据就该删掉"的场景），别为了对齐另外两张卡凭空加出来。
  const cardView = (
    <PartnerCards
      rows={rows}
      loading={tableProps.loading}
      pager={toCardPager(tableProps.pagination, (total) => `共 ${total} 个货代`)}
      onShow={(record) => show('forwarders', record.id)}
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
        <Segmented
          value={view}
          onChange={(value) => setView(value as 'list' | 'card')}
          options={[
            { value: 'list', icon: <BarsOutlined />, title: '列表视图：一行一家，字段最全' },
            { value: 'card', icon: <AppstoreOutlined />, title: '卡片视图：按 Logo 与名称认人' },
          ]}
          aria-label="切换货代视图"
        />
      }
      filters={
        <>
          <Form.Item name="keyword">
            <Input placeholder="搜索编码 / 名称 / 联系人" allowClear />
          </Form.Item>
        </>
      }
    >
      <Table.Column
        dataIndex="code"
        title="编码"
        width={120}
        render={(value: string) => <Mono>{value}</Mono>}
      />
      {/* 头像与卡片视图同组件同尺寸：换视图不能换长相。 */}
      <Table.Column
        dataIndex="name"
        title="名称"
        render={(value: string, record: Partner) => (
          <div className="cell-avatar">
            <PartnerAvatar name={record.name} logoUrl={record.logo_url} />
            <Typography.Link onClick={() => show('forwarders', record.id)}>
              <span className="cell-strong">{value}</span>
            </Typography.Link>
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
    </PageList>
  )
}
