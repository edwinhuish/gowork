import { useQuery } from '@tanstack/react-query'
import { useList, useShow } from '@refinedev/core'
import { Show } from '@refinedev/antd'
import {
  Button,
  Descriptions,
  message,
  Popconfirm,
  Space,
  Tag,
  Timeline,
  Typography,
} from 'antd'

import { opportunityApi } from '@/api/opportunities'
import { Empty, Money, OptionTag } from '@/components/fields'
import { ResponsiveTable } from '@/components/page'
import {
  ACTIVITY_TYPE_OPTIONS,
  OPPORTUNITY_STAGE_OPTIONS,
  opportunityStageLabel,
} from '@/constants/catalog'
import type { Activity, Opportunity, OpportunityStageLog } from '@/types'
import { formatDate, formatTime } from '@/utils/format'

/**
 * 商机详情（T-62）。
 *
 * 在这之前商机只有列表与编辑页——「这次询价谈了几轮」只能翻客户跟进列表去凑。
 * 这里把挂在**这个商机**上的跟进列出来，并给一个带好上下文的「记一条跟进」。
 */
export function OpportunityShow() {
  const { result: o, query } = useShow<Opportunity>({ resource: 'opportunities' })

  // 按商机筛跟进：后端 ?opportunity_id= 已支持，这里只取第一页足够——
  // 一次询价的往来不会超过二十条，再多的场景该去客户 360 里看。
  const { query: activityQuery, result } = useList<Activity>({
    resource: 'activities',
    filters: o?.id ? [{ field: 'opportunity_id', operator: 'eq', value: o.id }] : [],
    queryOptions: { enabled: !!o?.id },
    pagination: { mode: 'off' },
  })

  const activities: Activity[] = result?.data ?? []
  // 赢单 / 输单是已发生的事，本币金额不再重估（后端同样会 409）。
  const isFinal = o?.stage === 50 || o?.stage === 60

  // 阶段变更历史（T-61）：这条接口不是 CRUD 资源，直接走 http 封装的 api。
  const logsQuery = useQuery({
    queryKey: ['opportunity-stage-logs', o?.id],
    queryFn: () => opportunityApi.stageLogs(o!.id),
    enabled: !!o?.id,
  })
  const logs: OpportunityStageLog[] = logsQuery.data ?? []

  // 按今日汇率重估（T-61）：保存商机不重算本币金额，
  // 否则汇率一漂，上一季的加权预测跟着漂，季度环比就没了。
  const revalue = async () => {
    if (!o) return
    try {
      await opportunityApi.revalue(o.id)
      message.success('已按今日汇率重估本币金额')
      await query.refetch()
    } catch (err) {
      message.error(err instanceof Error ? err.message : '重估失败')
    }
  }

  return (
    <Show
      title={o ? `商机 ${o.subject}` : '商机'}
      isLoading={query.isLoading}
      headerButtons={
        <Space wrap>
          {!isFinal && (
            <Popconfirm
              title="按今日汇率重估这笔商机？"
              description="本币金额会按今天的汇率重算，加权预测随之变化"
              okText="重估"
              cancelText="再想想"
              onConfirm={revalue}
            >
              <Button>按今日汇率重算</Button>
            </Popconfirm>
          )}
          <Button
            href={`/activities/create?partner_id=${o?.partner_id ?? ''}&opportunity_id=${o?.id ?? ''}`}
          >
            记一条跟进
          </Button>
        </Space>
      }
    >
      <Descriptions bordered size="small" column={2}>
        {/* 客户名带编号，占满整行（column 写死 2 列，与 'filled' 等价；
            哪天 column 改成响应式也不会超出列数）。 */}
        <Descriptions.Item label="客户" span="filled">
          {o?.partner ? `${o.partner.name}（${o.partner.code}）` : <Empty />}
        </Descriptions.Item>
        <Descriptions.Item label="阶段">
          <Tag>{opportunityStageLabel(o?.stage)}</Tag>
        </Descriptions.Item>
        <Descriptions.Item label="赢率">{o ? `${o.probability}%` : <Empty />}</Descriptions.Item>
        <Descriptions.Item label="金额（原币）">
          {o ? <Money value={o.amount} currency={o.currency_code} /> : <Empty />}
        </Descriptions.Item>
        <Descriptions.Item label="金额（本币）">
          {o ? <Money value={o.amount_base} /> : <Empty />}
        </Descriptions.Item>
        <Descriptions.Item label="汇率">{o?.exchange_rate || <Empty />}</Descriptions.Item>
        <Descriptions.Item label="折算日">
          {/* 折算日比汇率更能说明问题：同一个数配两个日期的汇率，谁也说不清它怎么来的。*/}
          {o?.rate_locked_on ? formatDate(o.rate_locked_on) : <Empty />}
        </Descriptions.Item>
        <Descriptions.Item label="预计成交日">{formatDate(o?.expected_close_date)}</Descriptions.Item>
        <Descriptions.Item label="备注">{o?.remark || <Empty />}</Descriptions.Item>
      </Descriptions>

      <Typography.Title level={5} style={{ marginTop: 24 }}>
        跟进记录
      </Typography.Title>

      <ResponsiveTable<Activity>
        rowKey="id"
        size="small"
        loading={activityQuery.isLoading}
        rows={activities}
        pagination={false}
        // 第一列是日期，卡上该写主题：认一条跟进靠的是"谈了什么"。
        cardTitleKey="subject"
        empty="这个商机还没有跟进记录。谈单细节写在这里，客户详情里的时间线才完整。"
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

      <Typography.Title level={5} style={{ marginTop: 24 }}>
        阶段变更
      </Typography.Title>

      {/* 时间线按时间正序（后端已排好）：读起来是"怎么走到今天的"，
          倒序只适合看"最近发生了什么"，而这条商机的重点在过程。 */}
      <Timeline
        pending={logsQuery.isLoading ? '载入中…' : undefined}
        items={
          logs.length > 0
            ? logs.map((l) => ({
                children: (
                  <Space direction="vertical" size={0}>
                    <Space size={6}>
                      {l.from_stage === 0 ? (
                        <Tag bordered={false}>新建</Tag>
                      ) : (
                        <OptionTag value={l.from_stage} options={OPPORTUNITY_STAGE_OPTIONS} />
                      )}
                      <span>→</span>
                      <OptionTag value={l.to_stage} options={OPPORTUNITY_STAGE_OPTIONS} />
                      <Typography.Text type="secondary">赢率 {l.probability}%</Typography.Text>
                    </Space>
                    <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                      {formatTime(l.created_at)}
                      {l.actor_name ? ` · ${l.actor_name}` : ' · 系统'}
                    </Typography.Text>
                  </Space>
                ),
              }))
            : []
        }
      />
      {!logsQuery.isLoading && logs.length === 0 && (
        <Typography.Text type="secondary">这条商机还没有阶段变更记录。</Typography.Text>
      )}
    </Show>
  )
}
