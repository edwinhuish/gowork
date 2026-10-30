import { useEffect, useState } from 'react'
import { FundOutlined, PercentageOutlined, TeamOutlined } from '@ant-design/icons'
import type { CrudFilters } from '@refinedev/core'
import { useSelect } from '@refinedev/core'
import { DeleteButton, EditButton, useTable } from '@refinedev/antd'
import { Col, Form, Input, Row, Select, Space, Table } from 'antd'

import { opportunityApi } from '@/api/opportunities'
import { Empty, Money, OptionTag } from '@/components/fields'
import { PageList, StatCard } from '@/components/page'
import { OPPORTUNITY_STAGE_OPTIONS } from '@/constants/catalog'
import type { Opportunity, OpportunitySummary, Partner } from '@/types'
import { formatDate } from '@/utils/format'

interface OpportunitySearch {
  keyword?: string
  partner_id?: number
  stage?: number
  open_only?: number
}

/**
 * 商机列表。
 *
 * 顶部汇总单独取一次 `/opportunities-summary`，不从当前页算：
 * 分页之后的这一页只能代表这一页，用它算"总额"会随页码变化。
 */
export function OpportunityList() {
  const [summary, setSummary] = useState<OpportunitySummary | null>(null)

  useEffect(() => {
    let cancelled = false
    opportunityApi
      .summary()
      .then((data) => {
        if (!cancelled) setSummary(data)
      })
      .catch(() => {
        // 汇总失败不影响列表使用，错误由 http 客户端统一提示。
      })
    return () => {
      cancelled = true
    }
  }, [])

  const { options: partnerOptions, query: partnerQuery } = useSelect<Partner>({
    resource: 'partners',
    optionLabel: (item) => `${item.name}（${item.code}）`,
    optionValue: 'id',
    pagination: { mode: 'off' },
  })

  const { tableProps, searchFormProps } = useTable<Opportunity>({
    resource: 'opportunities',
    syncWithLocation: true,
    // 筛选表单给的是「这一次的完整条件」，所以用 replace：refine 默认 merge
    // 会把上一次的条件留着 —— 清空关键字再点查询，keyword 仍旧跟着请求走，
    // 表现是「条件清不掉、列表也不动」。永久筛选仍会保留（见 PageList 的说明）。
    filters: { defaultBehavior: 'replace' },
    onSearch: (values): CrudFilters => {
      const v = values as OpportunitySearch
      const filters: CrudFilters = []
      if (v.keyword?.trim()) {
        filters.push({ field: 'keyword', operator: 'contains', value: v.keyword.trim() })
      }
      if (v.partner_id) {
        filters.push({ field: 'partner_id', operator: 'eq', value: v.partner_id })
      }
      if (v.stage) {
        filters.push({ field: 'stage', operator: 'eq', value: v.stage })
      }
      if (v.open_only) {
        filters.push({ field: 'open_only', operator: 'eq', value: 1 })
      }
      return filters
    },
  })

  const resetFilters = () => {
    searchFormProps.form?.resetFields()
    searchFormProps.form?.submit()
  }

  return (
    <PageList
      createLabel="新建商机"
      searchFormProps={searchFormProps}
      onReset={resetFilters}
      tableProps={tableProps}
      rowKey="id"
      summary={
        <Row gutter={[12, 12]}>
          <Col xs={24} sm={8}>
            <StatCard title="商机数（全部）" value={summary?.total_count ?? 0} icon={<TeamOutlined />} />
          </Col>
          <Col xs={24} sm={8}>
            <StatCard
              title="预计总额（本币）"
              value={summary?.total_amount_base ?? 0}
              precision={2}
              icon={<FundOutlined />}
            />
          </Col>
          <Col xs={24} sm={8}>
            <StatCard
              title="加权金额（本币）"
              value={summary?.total_weighted ?? 0}
              precision={2}
              // 加权 = 金额 × 概率。它是"按现状能指望多少"，不是承诺收入。
              hint="按阶段概率加权，不等于承诺收入"
              icon={<PercentageOutlined />}
              tone="info"
            />
          </Col>
        </Row>
      }
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
          <Form.Item name="stage">
            <Select placeholder="全部阶段" allowClear options={OPPORTUNITY_STAGE_OPTIONS} />
          </Form.Item>
          <Form.Item name="open_only">
            <Select placeholder="全部状态" allowClear options={[{ value: 1, label: '只看在谈' }]} />
          </Form.Item>
          <Form.Item name="keyword">
            <Input placeholder="搜索商机名称" allowClear />
          </Form.Item>
        </>
      }
    >
      <Table.Column dataIndex="subject" title="商机" width={220} />
      <Table.Column
        dataIndex="partner"
        title="往来单位"
        width={170}
        render={(value: Opportunity['partner']) => (value ? value.name : <Empty />)}
      />
      <Table.Column
        dataIndex="stage"
        title="阶段"
        width={110}
        render={(value: number) => <OptionTag value={value} options={OPPORTUNITY_STAGE_OPTIONS} />}
      />
      <Table.Column
        dataIndex="amount"
        title="原币金额"
        width={140}
        align="right"
        render={(value: number, record: Opportunity) => (
          <span className="cell-mono">
            {value} {record.currency_code}
          </span>
        )}
      />
      <Table.Column
        dataIndex="amount_base"
        title="本币金额"
        width={130}
        align="right"
        render={(value: number) => <Money value={value} />}
      />
      <Table.Column
        dataIndex="probability"
        title="概率"
        width={80}
        align="right"
        render={(value: number) => `${value}%`}
      />
      <Table.Column
        dataIndex="expected_close_date"
        title="预计成交"
        width={120}
        render={(value: string) => <span className="cell-mono">{formatDate(value)}</span>}
      />
      <Table.Column
        title="操作"
        width={140}
        render={(_, record: Opportunity) => (
          <Space size={4}>
            <EditButton size="small" recordItemId={record.id} resource="opportunities" />
            <DeleteButton size="small" recordItemId={record.id} resource="opportunities" />
          </Space>
        )}
      />
    </PageList>
  )
}
