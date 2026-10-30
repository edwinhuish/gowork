import { useQuery } from '@tanstack/react-query'
import { useNavigation } from '@refinedev/core'
import {
  Alert,
  Card,
  Col,
  Flex,
  Row,
  Space,
  Table,
  Tag,
  Typography,
} from 'antd'

import { reportApi } from '@/api/reports'
import { Empty, Mono, Money } from '@/components/fields'
import { PageHeader, StatCard } from '@/components/page'
import { ResponsiveTable } from '@/components/page'
import type { TodoCurrency, TodoFollowup, TodoReceivable, TodoInspection, TodoContractTerm } from '@/types'
import { formatTime } from '@/utils/format'

/**
 * 待办清单（T-34）。
 *
 * 四类各有一个明确口径，页面上都写出来：
 *   - 待跟进：下次跟进日**早于今天**（当天不算——早上就先标红只会让人学会无视）；
 *   - 应收待催：到期日 <= 今天且未结清（当天就算——催款是当天的事）；
 *   - 验货到期：计划验货日 <= 今天且未完成；
 *   - 合同分期待收：约定付款日 <= 今天且未收到。
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

  const followup = data?.followup_count ?? 0
  const receivable = data?.receivable_count ?? 0
  const inspection = data?.inspection_count ?? 0
  const contractTerm = data?.contract_term_count ?? 0

  return (
    <Flex vertical gap={16}>
      <PageHeader
        title="待办"
        description="四类各有明确口径，每行都能点进源单据；条数是全量，清单只列最急的 20 条"
        tags={<Tag bordered={false}>截至 {data?.as_of ?? '今天'}</Tag>}
      />

      {error && <Alert type="error" showIcon message={(error as Error).message} />}

      <Row gutter={[16, 16]}>
        <Col xs={12} md={6}>
          <StatCard
            title="待跟进"
            value={followup}
            hint="下次跟进日早于今天"
            tone={followup > 0 ? 'warning' : 'primary'}
          />
        </Col>
        <Col xs={12} md={6}>
          <StatCard
            title="应收待催"
            value={receivable}
            hint="到期日 <= 今天且未结清"
            tone={receivable > 0 ? 'danger' : 'primary'}
          />
        </Col>
        <Col xs={12} md={6}>
          <StatCard title="验货到期" value={inspection} hint="计划日 <= 今天且未完成" />
        </Col>
        <Col xs={12} md={6}>
          <StatCard title="合同待收" value={contractTerm} hint="约定付款日 <= 今天且未收到" />
        </Col>
      </Row>

      <Card
        variant="borderless"
        title="待跟进"
        extra={<span className="cell-sub">下次跟进日早于今天（当天不算）</span>}
      >
        <ResponsiveTable<TodoFollowup>
          size="small"
          rowKey="id"
          loading={isLoading}
          rows={data?.followups ?? []}
          pagination={false}
          empty="没有到期的跟进"
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
            render={(value: number) => (
              <Tag bordered={false} color="warning">
                {value} 天
              </Tag>
            )}
          />
        </ResponsiveTable>
      </Card>

      {/* 口径里的 "<=" 写成表达式：JSX 文本里的小于号会被当成标签开头。 */}
      <Card
        variant="borderless"
        title="应收待催"
        extra={<span className="cell-sub">{'到期日 <= 今天且未结清（当天就算）'}</span>}
      >
        <ResponsiveTable<TodoReceivable>
          size="small"
          rowKey="id"
          loading={isLoading}
          rows={data?.receivables ?? []}
          pagination={false}
          empty="没有到期的应收"
          // 最后一列「未结清（本币）」无 dataIndex 只有 render，会被当成操作列
          // 挪到卡片底部、把标签弄丢。待办这几张表都没有操作列。
          cardActionKeys={[]}
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
              value > 0 ? (
                <Tag bordered={false} color="error">
                  {value} 天
                </Tag>
              ) : (
                <Tag bordered={false} color="warning">
                  今天到期
                </Tag>
              )
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
        </ResponsiveTable>
      </Card>

      {/* 验货到期（T-62）：代理采购唯一能对客户承诺的事，逾期没验意味着交期要出问题。
          点单号跳到采购详情——那里能直接补录验货完成日。 */}
      <Card
        variant="borderless"
        title="验货到期"
        extra={<span className="cell-sub">{'计划日 <= 今天且未完成'}</span>}
      >
        <ResponsiveTable<TodoInspection>
          size="small"
          rowKey="id"
          loading={isLoading}
          rows={data?.inspections ?? []}
          pagination={false}
          empty="没有逾期未验的采购行"
        >
          <Table.Column
            dataIndex="doc_no"
            title="采购单"
            width={160}
            render={(value: string, row: TodoInspection) => (
              <Typography.Link onClick={() => show('purchase-orders', row.purchase_order_id)}>
                <Mono>{value}</Mono>
              </Typography.Link>
            )}
          />
          <Table.Column dataIndex="sku_code" title="SKU" width={140} />
          <Table.Column
            title="供应商"
            width={200}
            render={(_, row: TodoInspection) => row.partner_name || <Empty />}
          />
          <Table.Column dataIndex="plan_date" title="计划验货日" width={120} />
          <Table.Column
            dataIndex="overdue_days"
            title="逾期"
            width={100}
            align="right"
            render={(value: number) =>
              value > 0 ? (
                <Tag bordered={false} color="error">
                  {value} 天
                </Tag>
              ) : (
                <Tag bordered={false} color="warning">
                  今天
                </Tag>
              )
            }
          />
        </ResponsiveTable>
      </Card>

      {/* 合同分期待收（T-61）：订金没到账就先开工，是这类生意最常见的现金流断裂。 */}
      <Card
        variant="borderless"
        title="合同分期待收"
        extra={<span className="cell-sub">{'约定付款日 <= 今天且未收到'}</span>}
      >
        <ResponsiveTable<TodoContractTerm>
          size="small"
          rowKey="id"
          loading={isLoading}
          rows={data?.contract_terms ?? []}
          pagination={false}
          empty="没有逾期未收的合同分期"
          cardActionKeys={[]}
        >
          <Table.Column
            dataIndex="doc_no"
            title="合同"
            width={160}
            render={(value: string, row: TodoContractTerm) => (
              <Typography.Link onClick={() => show('contracts', row.contract_id)}>
                <Mono>{value}</Mono>
              </Typography.Link>
            )}
          />
          <Table.Column
            title="分期"
            width={140}
            render={(_, row: TodoContractTerm) => `${row.seq}. ${row.name}`}
          />
          <Table.Column
            title="客户"
            width={200}
            render={(_, row: TodoContractTerm) => row.partner_name || <Empty />}
          />
          <Table.Column dataIndex="due_date" title="约定付款日" width={120} />
          <Table.Column
            dataIndex="overdue_days"
            title="逾期"
            width={100}
            align="right"
            render={(value: number) =>
              value > 0 ? (
                <Tag bordered={false} color="error">
                  {value} 天
                </Tag>
              ) : (
                <Tag bordered={false} color="warning">
                  今天
                </Tag>
              )
            }
          />
          <Table.Column
            title="金额（本币）"
            width={150}
            align="right"
            render={(_, row: TodoContractTerm) => (
              <span className="cell-strong">
                <Money value={row.amount_base} />
              </span>
            )}
          />
        </ResponsiveTable>
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
    </Flex>
  )
}
