import { useState } from 'react'
import { AppstoreOutlined, BarsOutlined } from '@ant-design/icons'
import { useInvalidate, useNavigation, type CrudFilters } from '@refinedev/core'
import { PartnerImportModal } from './import-modal'
import { CreateButton, useTable } from '@refinedev/antd'
import {
  Button,
  Checkbox,
  Form,
  Input,
  message,
  Segmented,
  Select,
  Space,
  Table,
  Tag,
  Tooltip,
  Typography,
} from 'antd'

import { exportApi } from '@/api/exports'
import { partnerApi } from '@/api/partners'
import { Empty, EntityTypeTag, Mono, RoleTag, StatusTag } from '@/components/fields'
import { EmptyGuide, PageList } from '@/components/page'
import { PartnerAvatar } from '@/components/partner-avatar'
import {
  PartnerCards,
  PartnerRowActions,
  PARTNER_CARD_PAGE_SIZES,
  toCardPager,
} from '@/components/partner-cards'
import { ENABLE_STATUS_OPTIONS, PARTNER_GRADE_FILTER_OPTIONS } from '@/constants/catalog'
import { useListView } from '@/hooks/use-list-view'
import type { Partner } from '@/types'

interface CustomerSearch {
  keyword?: string
  country?: string
  status?: number
  /** 分组维度都是后端精确匹配，输入框留空即不筛。 */
  customer_group?: string
  region?: string
  /** A / B / C，或 `none` 表示未分级。 */
  grade?: string
  /** 含已删除：打开后已软删的主体也列出来，只能做一件事——恢复。 */
  include_deleted?: boolean
}

/**
 * 客户列表。
 *
 * 国家是自由文本（后端也是 string），所以给输入框而不是下拉 ——
 * 做成下拉就得先维护一份国家字典，当前阶段没必要。
 */
/** 导出客户（角色位 1）。失败时给一句人话，不要把浏览器控制台的异常丢给用户。 */
async function exportPartners() {
  try {
    await exportApi.partners(1)
  } catch (err) {
    message.error(err instanceof Error ? err.message : '导出失败')
  }
}

export function CustomerList() {
  const { view, setView } = useListView('gowork:customers:view')
  const { tableProps, searchFormProps } = useTable<Partner>({
    resource: 'customers',
    syncWithLocation: true,
    pagination: { pageSize: Number(PARTNER_CARD_PAGE_SIZES[0]) },
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
      if (v.customer_group?.trim()) {
        filters.push({ field: 'customer_group', operator: 'eq', value: v.customer_group.trim() })
      }
      if (v.region?.trim()) {
        filters.push({ field: 'region', operator: 'eq', value: v.region.trim() })
      }
      if (v.grade) {
        filters.push({ field: 'grade', operator: 'eq', value: v.grade })
      }
      if (v.include_deleted) {
        filters.push({ field: 'include_deleted', operator: 'eq', value: true })
      }
      return filters
    },
  })

  const invalidate = useInvalidate()
  const { show } = useNavigation()
  const [importOpen, setImportOpen] = useState(false)

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

  const rows = (tableProps.dataSource ?? []) as Partner[]
  const cardView = (
    <PartnerCards
      rows={rows}
      loading={tableProps.loading}
      showCustomerFacts
      pager={toCardPager(tableProps.pagination, (total) => `共 ${total} 个客户`)}
      onShow={(record) => show('customers', record.id)}
      renderActions={(record) => (
        <PartnerRowActions
          record={record}
          resource="customers"
          entityLabel="客户"
          onRestore={(r) => void restore(r)}
        />
      )}
      empty={
        <EmptyGuide
          title="没有符合条件的客户"
          description="换个筛选条件，或者直接新建一个客户"
          action={
            <Space>
              <Button onClick={resetFilters}>重置筛选</Button>
              <CreateButton>新建客户</CreateButton>
            </Space>
          }
        />
      }
    />
  )

  return (
    <PageList
      createLabel="新建客户"
      searchFormProps={searchFormProps}
      onReset={resetFilters}
      tableProps={tableProps}
      rowKey="id"
      pageSizeOptions={PARTNER_CARD_PAGE_SIZES}
      table={view === 'card' ? cardView : undefined}
      extra={
        <>
          <Segmented
            value={view}
            onChange={(value) => setView(value as 'list' | 'card')}
            options={[
              { value: 'list', icon: <BarsOutlined />, title: '列表视图：一行一家，字段最全' },
              { value: 'card', icon: <AppstoreOutlined />, title: '卡片视图：按 Logo 与名称认人' },
            ]}
            aria-label="切换客户视图"
          />
          {/* 导出的是"这一角色下的全部"，不走页面筛选——筛选条件在表格内部，
              拿不到就不假装能拿到。行数上限 10000（后端另有拦截）。 */}
          <Tooltip title="导出全部客户（CSV，忽略当前筛选，上限 10000 行）">
            <Button onClick={() => void exportPartners()}>导出 CSV</Button>
          </Tooltip>
          {/* 导入先出报告再落库：一份几百行的文件里错两三行是常态。 */}
          <Button onClick={() => setImportOpen(true)}>导入 CSV</Button>
        </>
      }
      filters={
        <>
          <Form.Item name="keyword">
            <Input placeholder="搜索编码 / 名称 / 联系人" allowClear />
          </Form.Item>
          <Form.Item name="country">
            <Input placeholder="国家 / 地区" allowClear />
          </Form.Item>
          <Form.Item name="customer_group">
            <Input placeholder="客户分组" allowClear />
          </Form.Item>
          <Form.Item name="region">
            <Input placeholder="销售区域" allowClear />
          </Form.Item>
          <Form.Item name="grade">
            <Select placeholder="全部分级" allowClear options={PARTNER_GRADE_FILTER_OPTIONS} />
          </Form.Item>
          <Form.Item name="status">
            <Select placeholder="全部状态" allowClear options={ENABLE_STATUS_OPTIONS} />
          </Form.Item>
          <Form.Item name="include_deleted" valuePropName="checked">
            <Checkbox>含已删除</Checkbox>
          </Form.Item>
        </>
      }
      footer={
        <PartnerImportModal
          open={importOpen}
          onClose={() => setImportOpen(false)}
          onImported={() => invalidate({ resource: 'customers', invalidates: ['list'] })}
        />
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
      {/* 名称是进详情页的入口：客户 360、附件、合并、转移负责人都在详情里，
          列表上不给这个链接，等于这些能力只能靠手敲 URL。
          头像与卡片视图调用同一个组件、同一个默认尺寸，换视图不能换长相。 */}
      <Table.Column
        dataIndex="name"
        title="名称"
        render={(value: string, record: Partner) => (
          <div className="cell-avatar">
            <PartnerAvatar name={record.name} logoUrl={record.logo_url} />
            <Typography.Link onClick={() => show('customers', record.id)}>
              <span className="cell-strong">{value}</span>
            </Typography.Link>
          </div>
        )}
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
      {/* 分级只做标签：不接权限也不接折扣，所以这里不给颜色等级额外的业务含义。 */}
      <Table.Column
        dataIndex="grade"
        title="分级"
        width={80}
        render={(v: string) =>
          v ? <Tag bordered={false}>{v}</Tag> : <span className="cell-sub">未分级</span>
        }
      />
      <Table.Column dataIndex="region" title="区域" width={110} render={(v: string) => v || <Empty />} />
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
        render={(_, record: Partner) => (
          <PartnerRowActions
            record={record}
            resource="customers"
            entityLabel="客户"
            onRestore={(r) => void restore(r)}
          />
        )}
      />
    </PageList>
  )
}
