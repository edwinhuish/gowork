import { useQuery } from '@tanstack/react-query'
import { useNavigation } from '@refinedev/core'
import { Alert, Card, Col, Row, Space, Statistic, Table, Tag, Typography } from 'antd'

import { reportApi } from '@/api/reports'
import { Empty, Mono, Money } from '@/components/fields'
import type { TodoCurrency, TodoFollowup, TodoReceivable } from '@/types'
import { formatTime } from '@/utils/format'

/**
 * 待办清单（T-34）。
 *
 * 三类各有一个明确口径，页面上都写出来：
 *   - 待跟进：下次跟进日**早于今天**（当天不算——早上就先标红只会让人学会无视）；
 *   - 应收待催：到期日 <= 今天且未结清（当天就算——催款是当天的事）；
 *   - 缺当日汇率：外币今天还没有汇率（报价与开单都要锁当天汇率）。
 *
 * 每块都显示全量条数：清单是"先做哪几件"，条数是"还剩多少"。
 * 每行都能点进源单据——提醒点不进去，就还得再搜一遍单号。
 */
export function TodoPage() {
  const { show, edit, create } = useNavigation()
  const { data, isLoading, error } = useQuery({
    queryKey: ['report-todos'],
    queryFn: () => reportApi.todos({ limit: 20 }),
  })

  return (
    <Space direction="vertical" size={16} style={{ display: 'flex' }}>
      {error && <Alert type="error" showIcon message={(error as Error).message} />}

      <Card variant="borderless">
        <Row gutter={[24, 16]}>
          <Col xs={12} md={8} xl={6}>
            <Statistic
              title="待跟进"
              value={data?.followup_count ?? 0}
              valueStyle={{ color: (data?.followup_count ?? 0) > 0 ? '#d46b08' : undefined }}
            />
          </Col>
          <Col xs={12} md={8} xl={6}>
            <Statistic
              title="应收待催"
              value={data?.receivable_count ?? 0}
              valueStyle={{ color: (data?.receivable_count ?? 0) > 0 ? '#cf1322' : undefined }}
            />
          </Col>
          <Col xs={12} md={8} xl={6}>
            <Statistic title="缺当日汇率" value={data?.missing_rates.length ?? 0} />
          </Col>
          <Col xs={24} md={8} xl={6}>
            <Typography.Text type="secondary">截至 {data?.as_of ?? '今天'}</Typography.Text>
          </Col>
        </Row>
      </Card>

      <Card
        variant="borderless"
        title="待跟进"
        extra={<span className="cell-sub">下次跟进日早于今天（当天不算）</span>}
      >
        <Table<TodoFollowup>
          size="small"
          rowKey="id"
          loading={isLoading}
          dataSource={data?.followups ?? []}
          pagination={false}
          locale={{ emptyText: '没有到期的跟进' }}
        >
          <Table.Column
            title="客户"
            width={200}
            render={(_, row: TodoFollowup) => row.partner_name || <Empty />}
          />
          <Table.Column
            dataIndex="subject"
            title="跟进主题"
            render={(value: string, row: TodoFollowup) => (
              <Typography.Link onClick={() => edit('activities', row.id)}>
                {value || <Empty />}
              </Typography.Link>
            )}
          />
          <Table.Column dataIndex="next_follow_at" title="约定日期" width={120} />
          <Table.Column
            dataIndex="overdue_days"
            title="逾期"
            width={90}
            align="right"
            render={(value: number) => <Tag color="orange">{value} 天</Tag>}
          />
        </Table>
      </Card>

      {/* 口径里的 "<=" 写成表达式：JSX 文本里的小于号会被当成标签开头。 */}
      <Card
        variant="borderless"
        title="应收待催"
        extra={<span className="cell-sub">{'到期日 <= 今天且未结清（当天就算）'}</span>}
      >
        <Table<TodoReceivable>
          size="small"
          rowKey="id"
          loading={isLoading}
          dataSource={data?.receivables ?? []}
          pagination={false}
          locale={{ emptyText: '没有到期的应收' }}
        >
          <Table.Column
            dataIndex="doc_no"
            title="单号"
            width={160}
            render={(value: string, row: TodoReceivable) => (
              <Typography.Link onClick={() => show('receivables', row.id)}>
                <Mono>{value}</Mono>
              </Typography.Link>
            )}
          />
          <Table.Column
            title="客户"
            width={220}
            render={(_, row: TodoReceivable) => row.partner_name || <Empty />}
          />
          <Table.Column dataIndex="due_date" title="到期日" width={120} />
          <Table.Column
            dataIndex="overdue_days"
            title="逾期"
            width={100}
            align="right"
            render={(value: number) =>
              value > 0 ? <Tag color="red">{value} 天</Tag> : <Tag color="gold">今天到期</Tag>
            }
          />
          <Table.Column
            title="未结清（本币）"
            width={150}
            align="right"
            render={(_, row: TodoReceivable) => (
              <span className="cell-strong">
                <Money value={row.outstanding_base} />
              </span>
            )}
          />
        </Table>
      </Card>

      <Card
        variant="borderless"
        title="缺当日汇率"
        extra={<span className="cell-sub">报价与开单都要锁当天汇率</span>}
      >
        {data && data.missing_rates.length > 0 ? (
          <Space wrap size={8}>
            {data.missing_rates.map((c: TodoCurrency) => (
              <Tag
                key={c.code}
                color="warning"
                style={{ cursor: 'pointer' }}
                onClick={() => create('exchange-rates')}
              >
                {c.code} · {c.name}（补录）
              </Tag>
            ))}
          </Space>
        ) : (
          <Typography.Text type="secondary">今天的外币汇率都齐了</Typography.Text>
        )}
      </Card>

      <Typography.Text type="secondary" style={{ fontSize: 12 }}>
        生成于 {formatTime(data?.as_of ?? '')}；「待确认单据」暂不做——
        什么算"停太久"还没有口径，随手定一个天数只会天天报警然后被无视。
      </Typography.Text>
    </Space>
  )
}
