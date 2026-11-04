import { Link } from 'react-router'
import { AppstoreOutlined, BarsOutlined } from '@ant-design/icons'
import { useInvalidate, useNavigation, type CrudFilters } from '@refinedev/core'
import { CreateButton } from '@refinedev/antd'
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
import { EntityTypeTag, Mono, RoleTag } from '@/components/fields'
import { EmptyGuide, PageList } from '@/components/page'
import { PartnerContactsCell } from '@/components/partner-contacts-cell'
import { PartnerCards, PartnerRowActions } from '@/components/partner-cards'
import { CARD_PAGE_SIZE, useInfiniteList } from '@/hooks/use-infinite-feed'
import { useListView } from '@/hooks/use-list-view'
import type { Partner } from '@/types'

interface SupplierSearch {
  keyword?: string
  /** 含已删除：打开后已软删的主体也列出来，只能做一件事——恢复。 */
  include_deleted?: boolean
}

/**
 * 供应商列表。
 *
 * 支持列表 / 卡片两种视图，与产品列表同一套骨架：
 * 筛选条 + 表格或卡片网格 + 新建入口，两种视图共用一个 useTable，
 * 切换时筛选、排序、页码都不动。
 */
export function SupplierList() {
  // 窄屏强制卡片视图（表格在这个宽度上只能横滑），切换控件随之隐藏。
  const { view, setView, narrow } = useListView('gowork:suppliers:view')
  const { tableProps, searchFormProps, feed } = useInfiniteList<Partner>({
    resource: 'suppliers',
    syncWithLocation: true,
    // 筛选表单给的是「这一次的完整条件」，所以用 replace：refine 默认 merge
    // 会把上一次的条件留着 —— 清空关键字再点查询，keyword 仍旧跟着请求走，
    // 表现是「条件清不掉、列表也不动」。永久筛选仍会保留（见 PageList 的说明）。
    filters: { defaultBehavior: 'replace' },
    // 一批 12 条：卡片在常见宽度下一行 3~4 张，凑得满一行。
    pagination: { pageSize: CARD_PAGE_SIZE },
    onSearch: (values): CrudFilters => {
      const v = values as SupplierSearch
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

  const rows = (tableProps.dataSource ?? []) as Partner[]
  const cardView = (
    <PartnerCards
      rows={rows}
      loading={tableProps.loading}
      onShow={(record) => show('suppliers', record.id)}
      renderActions={(record) => (
        <PartnerRowActions
          record={record}
          resource="suppliers"
          entityLabel="供应商"
          onRestore={(r) => void restore(r)}
        />
      )}
      empty={
        <EmptyGuide
          title="没有符合条件的供应商"
          description="换个筛选条件，或者直接新建一个供应商"
          action={
            <Space>
              <Button onClick={resetFilters}>重置筛选</Button>
              <CreateButton>新建供应商</CreateButton>
            </Space>
          }
        />
      }
    />
  )

  return (
    <PageList
      createLabel="新建供应商"
      searchFormProps={searchFormProps}
      onReset={resetFilters}
      tableProps={tableProps}
      infinite={feed}
      noun="个供应商"
      rowKey="id"
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
            aria-label="切换供应商视图"
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
      {/* 名称即入口：货源、附件与合并都在详情页，列表不给链接就只能手敲 URL。
          名称列不再带 Logo 头像：认人交给联系人头像组，这一列只放名称。 */}
      <Table.Column
        dataIndex="name"
        title="名称"
        render={(value: string, record: Partner) => (
          /* 见客户列表：真链接，中键 / 右键「在新标签打开」才可用。 */
          <Link to={`/suppliers/show/${record.id}`}>
            <span className="cell-strong">{value}</span>
          </Link>
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
      {/* 联系人是一对多：平铺的「姓名 / 电话 / 邮箱」三列只能装下第一个人。
          这里只给头像组（最多 3 个，其余 +N），详情与 /contacts 看完整名单。 */}
      <Table.Column
        title="联系人"
        width={140}
        render={(_, record: Partner) => <PartnerContactsCell contacts={record.contacts} />}
      />
      <Table.Column
        title="操作"
        width={90}
        render={(_, record: Partner) => (
          <PartnerRowActions
            record={record}
            resource="suppliers"
            entityLabel="供应商"
            onRestore={(r) => void restore(r)}
          />
        )}
      />
    </PageList>
  )
}
