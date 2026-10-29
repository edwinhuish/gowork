import { useList, useShow } from '@refinedev/core'
import { Show } from '@refinedev/antd'
import { Button, Descriptions, Table, Tag, Typography } from 'antd'

import { Empty, Money } from '@/components/fields'
import { ACTIVITY_TYPE_OPTIONS, opportunityStageLabel } from '@/constants/catalog'
import type { Activity, Opportunity } from '@/types'
import { formatDate } from '@/utils/format'

/**
 * 商机详情（T-62）。
 *
 * 在这之前商机只有列表与编辑页——「这次询价谈了几轮」只能翻客户跟进列表去凑。
 * 这里把挂在**这个商机**上的跟进列出来，并给一个带好上下文的「记一条跟进」。
 */
export function OpportunityShow() {
  const { result: o } = useShow<Opportunity>({ resource: 'opportunities' })

  // 按商机筛跟进：后端 ?opportunity_id= 已支持，这里只取第一页足够——
  // 一次询价的往来不会超过二十条，再多的场景该去客户 360 里看。
  const { query, result } = useList<Activity>({
    resource: 'activities',
    filters: o?.id ? [{ field: 'opportunity_id', operator: 'eq', value: o.id }] : [],
    queryOptions: { enabled: !!o?.id },
    pagination: { mode: 'off' },
  })

  const activities: Activity[] = result?.data ?? []

  return (
    <Show
      title={o ? `商机 ${o.subject}` : '商机'}
      headerButtons={
        <Button
          href={`/activities/create?partner_id=${o?.partner_id ?? ''}&opportunity_id=${o?.id ?? ''}`}
        >
          记一条跟进
        </Button>
      }
    >
      <Descriptions bordered size="small" column={2}>
        <Descriptions.Item label="客户" span={2}>
          {o?.partner ? `${o.partner.name}（${o.partner.code}）` : <Empty />}
        </Descriptions.Item>
        <Descriptions.Item label="阶段">
          <Tag>{opportunityStageLabel(o?.stage)}</Tag>
        </Descriptions.Item>
        <Descriptions.Item label="赢率">{o ? `${o.probability}%` : <Empty />}</Descriptions.Item>
        <Descriptions.Item label="金额（原币）">
          {o ? <Money value={o.amount} /> : <Empty />}
        </Descriptions.Item>
        <Descriptions.Item label="金额（本币）">
          {o ? <Money value={o.amount_base} /> : <Empty />}
        </Descriptions.Item>
        <Descriptions.Item label="预计成交日">{formatDate(o?.expected_close_date)}</Descriptions.Item>
        <Descriptions.Item label="备注">{o?.remark || <Empty />}</Descriptions.Item>
      </Descriptions>

      <Typography.Title level={5} style={{ marginTop: 24 }}>
        跟进记录
      </Typography.Title>

      <Table<Activity>
        rowKey="id"
        size="small"
        loading={query.isLoading}
        dataSource={activities}
        pagination={false}
        locale={{ emptyText: '这个商机还没有跟进记录。谈单细节写在这里，客户详情里的时间线才完整。' }}
        columns={[
          {
            title: '日期',
            dataIndex: 'occurred_at',
            width: 110,
            render: (v: string) => formatDate(v),
          },
          {
            title: '方式',
            dataIndex: 'type',
            width: 100,
            render: (v: string) => (
              <Tag bordered={false}>
                {ACTIVITY_TYPE_OPTIONS.find((o2) => o2.value === v)?.label ?? v}
              </Tag>
            ),
          },
          { title: '主题', dataIndex: 'subject' },
          {
            title: '内容',
            dataIndex: 'content',
            render: (v: string) => v || <Empty />,
          },
        ]}
      />
    </Show>
  )
}
