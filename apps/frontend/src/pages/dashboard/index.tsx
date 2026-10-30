import { useQuery } from '@tanstack/react-query'
import { useNavigation } from '@refinedev/core'
import {
  Alert,
  Button,
  Card,
  Col,
  Flex,
  List,
  Progress,
  Row,
  Space,
  Statistic,
  Table,
  Tag,
  Typography,
  theme,
} from 'antd'

import {
  BellOutlined,
  CheckCircleOutlined,
  CommentOutlined,
  FileTextOutlined,
  GlobalOutlined,
  InboxOutlined,
  PlusOutlined,
  RiseOutlined,
  SendOutlined,
  ShoppingCartOutlined,
  TransactionOutlined,
  WarningOutlined,
} from '@ant-design/icons'

import { dashboardApi } from '@/api/dashboard'
import { reportApi } from '@/api/reports'
import { Money } from '@/components/fields'
import { EmptyGuide, PageHeader, StatCard } from '@/components/page'
import type { SalesDashboardRow } from '@/types'

/**
 * 概览页 = 工作台。
 *
 * 这一页不新增任何接口：待办、回款、经营三块分别来自 /reports/todos、
 * /reports/collection、/reports/sales-dashboard，口径与各自的明细页完全同源，
 * 所以在概览上看到的数字点进去必然对得上。
 *
 * 每个数字都必须能点进源页面 —— 点不进去的汇总只会让人再搜一遍单号。
 * 布局只用 Row/Col 与 antd 的 Card/List：改造前是一套只有这一页在用的
 * 手写 grid（.workbench / .span-7 / .quick-card），窄屏下只能叠成一条。
 */

/** 当月第一天与今天，YYYY-MM-DD。用本地时间而不是 UTC：跨时区时 UTC 会退回上月。 */
function currentRange() {
  const now = new Date()
  const y = now.getFullYear()
  const m = String(now.getMonth() + 1).padStart(2, '0')
  const d = String(now.getDate()).padStart(2, '0')
  return { from: `${y}-${m}-01`, to: `${y}-${m}-${d}` }
}

/** 0.6 → 60.0%。后端给的是小数，界面上不重算，只换单位。 */
function percent(rate?: string) {
  const n = Number(rate ?? 0)
  if (!Number.isFinite(n)) return '—'
  return `${(n * 100).toFixed(1)}%`
}

/** 待办清单的一条：类型 + 描述 + 逾期 + 金额。 */
type TodoEntry = {
  key: string
  kind: string
  color: string
  title: string
  sub: string
  overdue: number
  amount?: string
  onClick: () => void
}

/** 常用入口：图标 + 一句"这一步做什么"。 */
const QUICK_ENTRIES = [
  { key: 'quotations', title: '报价单', sub: '成本快照与转单', icon: <FileTextOutlined /> },
  { key: 'sales-orders', title: '销售订单', sub: '确认即校验额度', icon: <InboxOutlined /> },
  { key: 'purchase-orders', title: '采购订单', sub: '可挂销售单行', icon: <ShoppingCartOutlined /> },
  { key: 'shipments', title: '出货单', sub: '选订单行装箱', icon: <SendOutlined /> },
  { key: 'payments', title: '收款登记', sub: '核销应收', icon: <TransactionOutlined /> },
  { key: 'customers', title: '客户 360', sub: '订单 / 应收 / 跟进', icon: <GlobalOutlined /> },
] as const

export function DashboardPage() {
  const { list, create, show, edit } = useNavigation()
  const { token } = theme.useToken()
  const range = currentRange()

  const summary = useQuery({ queryKey: ['dashboard-summary'], queryFn: dashboardApi.summary })
  const todos = useQuery({
    queryKey: ['report-todos', 5],
    queryFn: () => reportApi.todos({ limit: 5 }),
  })
  const collection = useQuery({
    queryKey: ['report-collection', 'ar', 'overview'],
    queryFn: () => reportApi.collection({ direction: 10, limit: 5 }),
  })
  const sales = useQuery({
    queryKey: ['report-sales-dashboard', 'customer', range.from, range.to],
    queryFn: () => reportApi.salesDashboard({ dimension: 'customer', ...range, limit: 5 }),
  })

  const error =
    (summary.error as Error | null) ??
    (todos.error as Error | null) ??
    (collection.error as Error | null) ??
    (sales.error as Error | null)

  const todoEntries: TodoEntry[] = [
    ...(todos.data?.followups ?? []).map((row) => ({
      key: `f-${row.id}`,
      kind: '跟进',
      color: 'blue',
      title: row.partner_name || '未命名客户',
      sub: row.subject || '跟进记录',
      overdue: row.overdue_days,
      onClick: () => edit('activities', row.id),
    })),
    ...(todos.data?.receivables ?? []).map((row) => ({
      key: `r-${row.id}`,
      kind: '催款',
      color: 'red',
      title: row.partner_name || '未命名客户',
      sub: row.doc_no,
      overdue: row.overdue_days,
      amount: row.outstanding_base,
      onClick: () => show('receivables', row.id),
    })),
    ...(todos.data?.inspections ?? []).map((row) => ({
      key: `i-${row.id}`,
      kind: '验货',
      color: 'gold',
      title: row.partner_name || '未命名供应商',
      sub: `${row.doc_no} · ${row.sku_code}`,
      overdue: row.overdue_days,
      onClick: () => show('purchase-orders', row.purchase_order_id),
    })),
    ...(todos.data?.contract_terms ?? []).map((row) => ({
      key: `t-${row.id}`,
      kind: '收款',
      color: 'purple',
      title: row.partner_name || '未命名客户',
      sub: `${row.doc_no} · ${row.name}`,
      overdue: row.overdue_days,
      amount: row.amount_base,
      onClick: () => show('contracts', row.contract_id),
    })),
  ]
    .sort((a, b) => b.overdue - a.overdue)
    .slice(0, 8)

  const collectionRate = Number(collection.data?.collection_rate ?? 0) * 100
  const overdueBase = Number(collection.data?.overdue_base ?? 0)

  return (
    <Flex vertical gap={16}>
      <PageHeader
        title="工作台"
        description={
          <>
            产品 {summary.data?.product_total ?? '—'} 个 · 供应商{' '}
            {summary.data?.supplier_total ?? '—'} 家 · 客户 {summary.data?.customer_total ?? '—'}{' '}
            家 · 数据截至 {todos.data?.as_of ?? '今天'}
          </>
        }
        extra={
          <>
            <Button icon={<BellOutlined />} onClick={() => list('todos')}>
              全部待办
            </Button>
            <Button type="primary" icon={<PlusOutlined />} onClick={() => create('quotations')}>
              新建报价单
            </Button>
          </>
        }
      />

      {error && <Alert type="error" showIcon message={error.message} />}

      <Row gutter={[16, 16]}>
        <Col xs={24} sm={12} xl={6}>
          <StatCard
            title="待跟进客户"
            value={todos.data?.followup_count ?? 0}
            hint={
              <>
                已过期未跟进；另有验货逾期 {todos.data?.inspection_count ?? 0} 条、合同待收{' '}
                {todos.data?.contract_term_count ?? 0} 条
              </>
            }
            icon={<CommentOutlined />}
            tone={(todos.data?.followup_count ?? 0) > 0 ? 'warning' : 'primary'}
            onClick={() => list('activities')}
          />
        </Col>
        <Col xs={24} sm={12} xl={6}>
          <StatCard
            title="应收待催"
            value={todos.data?.receivable_count ?? 0}
            hint="到期日 <= 今天且未结清（当天就算）"
            icon={<WarningOutlined />}
            tone={(todos.data?.receivable_count ?? 0) > 0 ? 'danger' : 'primary'}
            onClick={() => list('collection')}
          />
        </Col>
        <Col xs={24} sm={12} xl={6}>
          <StatCard
            title="本月出货收入"
            value={<Money value={sales.data?.total.revenue} />}
            hint={`${range.from} 至 ${range.to}，按出货日；毛利率 ${percent(sales.data?.total.margin)}`}
            icon={<RiseOutlined />}
            onClick={() => list('sales-dashboard')}
          />
        </Col>
        <Col xs={24} sm={12} xl={6}>
          <StatCard
            title="本月毛利"
            value={
              sales.data?.total.cost_hidden ? (
                <span className="cell-empty">—</span>
              ) : (
                <Money value={sales.data?.total.profit} />
              )
            }
            hint={
              sales.data?.total.cost_hidden
                ? '当前角色看不到成本类金额'
                : sales.data?.total.cost_missing
                  ? '部分出货行缺采购成本，毛利不完整'
                  : '收入 − 采购成本 − 运杂费'
            }
            icon={<CheckCircleOutlined />}
            tone={sales.data?.total.cost_missing ? 'warning' : 'primary'}
            onClick={() => list('sales-dashboard')}
          />
        </Col>
      </Row>

      <Row gutter={[16, 16]}>
        <Col xs={24} xl={14}>
          <Card
            variant="borderless"
            title="今天该做的事"
            extra={<span className="cell-sub">按逾期天数排序，取最急的 8 条</span>}
          >
            <List<TodoEntry>
              dataSource={todoEntries}
              loading={todos.isLoading}
              split
              locale={{
                emptyText: (
                  <EmptyGuide
                    title="没有到期的事"
                    description="待跟进、应收待催、验货到期、合同分期这四类都清空了。有新的跟进或收款计划后，这里会自动出现。"
                    action={<Button onClick={() => list('todos')}>看待办口径</Button>}
                  />
                ),
              }}
              renderItem={(entry) => (
                <List.Item
                  className="todo-row"
                  onClick={entry.onClick}
                  extra={
                    <Space size={8}>
                      {entry.amount ? (
                        <span className="cell-strong" style={{ whiteSpace: 'nowrap' }}>
                          <Money value={entry.amount} />
                        </span>
                      ) : null}
                      <Tag bordered={false} color={entry.overdue > 0 ? 'error' : 'warning'}>
                        {entry.overdue > 0 ? `逾期 ${entry.overdue} 天` : '今天'}
                      </Tag>
                    </Space>
                  }
                >
                  <List.Item.Meta
                    avatar={
                      <Tag bordered={false} color={entry.color}>
                        {entry.kind}
                      </Tag>
                    }
                    title={<span className="cell-strong">{entry.title}</span>}
                    description={<span className="cell-sub">{entry.sub}</span>}
                  />
                </List.Item>
              )}
            />
          </Card>
        </Col>

        <Col xs={24} xl={10}>
          <Flex vertical gap={16}>
            <Card
              variant="borderless"
              title="应收账款"
              loading={collection.isLoading}
              extra={
                <Button type="link" size="small" onClick={() => list('collection')}>
                  回款与超期
                </Button>
              }
            >
              <Flex vertical gap={12}>
                <div>
                  <Typography.Text type="secondary" className="cell-sub">
                    期间回款率（到期 / 回款，按到期日）
                  </Typography.Text>
                  <Progress
                    percent={Math.min(100, Math.round(collectionRate))}
                    status={collectionRate >= 100 ? 'success' : 'active'}
                    format={() => percent(collection.data?.collection_rate)}
                    strokeColor={overdueBase > 0 ? token.colorWarning : undefined}
                  />
                </div>
                <Row gutter={12}>
                  <Col span={12}>
                    <Statistic
                      title="未结清（含未到期）"
                      valueRender={() => <Money value={collection.data?.outstanding_base} />}
                    />
                  </Col>
                  <Col span={12}>
                    <Statistic
                      title="其中已逾期"
                      valueRender={() => (
                        <span style={{ color: overdueBase > 0 ? token.colorError : undefined }}>
                          <Money value={collection.data?.overdue_base} />
                        </span>
                      )}
                    />
                  </Col>
                </Row>
                <Typography.Text type="secondary" style={{ fontSize: 12.5 }}>
                  回款率可能超过 100%：那是本月在还上个月的账，不是算错。
                </Typography.Text>
              </Flex>
            </Card>

            <Card variant="borderless" title="常用入口">
              <Row gutter={[8, 8]}>
                {QUICK_ENTRIES.map((item) => (
                  <Col xs={12} key={item.key}>
                    <Card
                      size="small"
                      hoverable
                      className="quick-entry"
                      onClick={() => create(item.key)}
                    >
                      <Flex align="center" gap={10}>
                        <span className="quick-entry-icon">{item.icon}</span>
                        <span className="cell-stack">
                          <span className="cell-strong">{item.title}</span>
                          <span className="cell-sub">{item.sub}</span>
                        </span>
                      </Flex>
                    </Card>
                  </Col>
                ))}
              </Row>
            </Card>
          </Flex>
        </Col>

        <Col span={24}>
          <Card
            variant="borderless"
            title="本月经营（按客户）"
            extra={
              <Space size={8}>
                <span className="cell-sub">
                  {range.from} 至 {range.to}
                </span>
                <Button type="link" size="small" onClick={() => list('sales-dashboard')}>
                  销售看板
                </Button>
              </Space>
            }
            loading={sales.isLoading}
          >
            <Table<SalesDashboardRow>
              size="small"
              rowKey="key"
              dataSource={sales.data?.rows ?? []}
              pagination={false}
              scroll={{ x: 720 }}
              locale={{ emptyText: '本月还没有出货记录' }}
            >
              <Table.Column
                dataIndex="label"
                title="客户"
                render={(value: string, row: SalesDashboardRow) =>
                  row.key === 'unknown' ? (
                    <span className="cell-sub">未指定客户</span>
                  ) : (
                    <Typography.Link onClick={() => show('customers', Number(row.key))}>
                      {value}
                    </Typography.Link>
                  )
                }
              />
              <Table.Column
                dataIndex="order_amount"
                title="订单额（下单日）"
                width={150}
                align="right"
                render={(value: string) => <Money value={value} />}
              />
              <Table.Column
                dataIndex="revenue"
                title="收入（出货日）"
                width={150}
                align="right"
                render={(value: string) => <Money value={value} />}
              />
              <Table.Column
                dataIndex="profit"
                title="毛利"
                width={130}
                align="right"
                render={(value: string, row: SalesDashboardRow) =>
                  row.cost_hidden ? <span className="cell-empty">—</span> : <Money value={value} />
                }
              />
              <Table.Column
                dataIndex="margin"
                title="毛利率"
                width={100}
                align="right"
                render={(value: string, row: SalesDashboardRow) =>
                  row.cost_hidden ? <span className="cell-empty">—</span> : percent(value)
                }
              />
            </Table>
            {sales.data?.truncated ? (
              <Typography.Text type="secondary" style={{ fontSize: 12.5 }}>
                只列前 5 个客户，合计与毛利率仍是全量。
              </Typography.Text>
            ) : null}
          </Card>
        </Col>
      </Row>
    </Flex>
  )
}
