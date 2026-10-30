import { useQuery } from '@tanstack/react-query'
import { useNavigation } from '@refinedev/core'
import {
  Alert,
  Button,
  Card,
  Col,
  Progress,
  Row,
  Space,
  Statistic,
  Table,
  Tag,
  Typography,
} from 'antd'
import type { ReactNode } from 'react'

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
import type { SalesDashboardRow } from '@/types'

/**
 * 概览页 = 工作台。
 *
 * 这一页不新增任何接口：待办、回款、经营三块分别来自 /reports/todos、
 * /reports/collection、/reports/sales-dashboard，口径与各自的明细页完全同源，
 * 所以在概览上看到的数字点进去必然对得上。
 *
 * 每个数字都必须能点进源页面 —— 点不进去的汇总只会让人再搜一遍单号。
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

type KpiCardProps = {
  title: string
  value: ReactNode
  hint: ReactNode
  icon: ReactNode
  tone?: 'default' | 'warning' | 'danger'
  onClick?: () => void
}

/** 抬头数字卡：图标点出所属业务，底部一行口径说明。整卡可点。 */
function KpiCard({ title, value, hint, icon, tone = 'default', onClick }: KpiCardProps) {
  const color =
    tone === 'danger' ? '#dc2626' : tone === 'warning' ? '#d97706' : 'var(--ant-color-text, #1c2532)'
  return (
    <Card
      variant="borderless"
      className="stat-card"
      hoverable={!!onClick}
      onClick={onClick}
      style={{ cursor: onClick ? 'pointer' : 'default' }}
    >
      <div className="stat-row">
        <Statistic
          title={title}
          valueRender={() => <span style={{ color, fontVariantNumeric: 'tabular-nums' }}>{value}</span>}
        />
        <span className="stat-icon">{icon}</span>
      </div>
      <Typography.Text type="secondary" className="stat-hint">
        {hint}
      </Typography.Text>
    </Card>
  )
}

/** 待办清单的一条：类型 + 描述 + 逾期 + 金额，四个字段对齐成一行。 */
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

export function DashboardPage() {
  const { list, create, show, edit } = useNavigation()
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
    <Space direction="vertical" size={16} style={{ display: 'flex' }}>
      <div className="page-head">
        <div className="page-head-text">
          <h1 className="page-head-title">工作台</h1>
          <p className="page-head-desc">
            产品 {summary.data?.product_total ?? '—'} 个（启用 {summary.data?.enabled_products ?? '—'}）·
            供应商 {summary.data?.supplier_total ?? '—'} 家 · 客户 {summary.data?.customer_total ?? '—'} 家 ·
            数据截至 {todos.data?.as_of ?? '今天'}
          </p>
        </div>
        <div className="page-head-extra">
          <Button icon={<BellOutlined />} onClick={() => list('todos')}>
            全部待办
          </Button>
          <Button type="primary" icon={<PlusOutlined />} onClick={() => create('quotations')}>
            新建报价单
          </Button>
        </div>
      </div>

      {error && <Alert type="error" showIcon message={error.message} />}

      <Row gutter={[16, 16]}>
        <Col xs={24} sm={12} xl={6}>
          <KpiCard
            title="待跟进客户"
            value={todos.data?.followup_count ?? 0}
            hint={
              <>
                已过期未跟进；另有验货逾期 {todos.data?.inspection_count ?? 0} 条、合同待收{' '}
                {todos.data?.contract_term_count ?? 0} 条
              </>
            }
            icon={<CommentOutlined />}
            tone={(todos.data?.followup_count ?? 0) > 0 ? 'warning' : 'default'}
            onClick={() => list('activities')}
          />
        </Col>
        <Col xs={24} sm={12} xl={6}>
          <KpiCard
            title="应收待催"
            value={todos.data?.receivable_count ?? 0}
            hint="到期日 <= 今天且未结清（当天就算）"
            icon={<WarningOutlined />}
            tone={(todos.data?.receivable_count ?? 0) > 0 ? 'danger' : 'default'}
            onClick={() => list('collection')}
          />
        </Col>
        <Col xs={24} sm={12} xl={6}>
          <KpiCard
            title="本月出货收入"
            value={<Money value={sales.data?.total.revenue} />}
            hint={`${range.from} 至 ${range.to}，按出货日；毛利率 ${percent(sales.data?.total.margin)}`}
            icon={<RiseOutlined />}
            onClick={() => list('sales-dashboard')}
          />
        </Col>
        <Col xs={24} sm={12} xl={6}>
          <KpiCard
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
            onClick={() => list('sales-dashboard')}
          />
        </Col>
      </Row>

      <div className="workbench">
        <div className="span-7">
          <Card
            variant="borderless"
            title="今天该做的事"
            extra={<span className="cell-sub">按逾期天数排序，取最急的 8 条</span>}
            loading={todos.isLoading}
          >
            {todoEntries.length === 0 ? (
              <div className="empty-guide">
                <span className="empty-guide-title">没有到期的事</span>
                <span className="empty-guide-desc">
                  待跟进、应收待催、验货到期、合同分期这四类都清空了。有新的跟进或收款计划后，这里会自动出现。
                </span>
                <Button onClick={() => list('todos')}>看待办口径</Button>
              </div>
            ) : (
              <Space direction="vertical" size={0} style={{ display: 'flex' }}>
                {todoEntries.map((entry) => (
                  <div
                    key={entry.key}
                    role="button"
                    tabIndex={0}
                    onClick={entry.onClick}
                    onKeyDown={(e) => {
                      if (e.key === 'Enter' || e.key === ' ') entry.onClick()
                    }}
                    style={{
                      display: 'flex',
                      alignItems: 'center',
                      gap: 12,
                      padding: '10px 4px',
                      borderBottom: '1px solid var(--ant-color-border-secondary, #e3e8f0)',
                      cursor: 'pointer',
                    }}
                  >
                    <Tag color={entry.color} bordered={false} style={{ marginInlineEnd: 0 }}>
                      {entry.kind}
                    </Tag>
                    <div className="cell-stack" style={{ flex: 1, minWidth: 0 }}>
                      <span className="cell-strong">{entry.title}</span>
                      <span className="cell-sub">{entry.sub}</span>
                    </div>
                    {entry.amount ? (
                      <span className="cell-strong" style={{ whiteSpace: 'nowrap' }}>
                        <Money value={entry.amount} />
                      </span>
                    ) : null}
                    <Tag color={entry.overdue > 0 ? 'red' : 'gold'} bordered={false}>
                      {entry.overdue > 0 ? `逾期 ${entry.overdue} 天` : '今天'}
                    </Tag>
                  </div>
                ))}
              </Space>
            )}
          </Card>
        </div>

        <div className="span-5">
          <Card
            variant="borderless"
            title="应收账款"
            extra={
              <Button type="link" size="small" onClick={() => list('collection')}>
                回款与超期
              </Button>
            }
            loading={collection.isLoading}
          >
            <Space direction="vertical" size={12} style={{ display: 'flex' }}>
              <div>
                <div className="cell-sub">期间回款率（到期 / 回款，按到期日）</div>
                <Progress
                  percent={Math.min(100, Math.round(collectionRate))}
                  status={collectionRate >= 100 ? 'success' : 'active'}
                  format={() => percent(collection.data?.collection_rate)}
                  strokeColor={overdueBase > 0 ? '#d97706' : undefined}
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
                      <span style={{ color: overdueBase > 0 ? '#dc2626' : undefined }}>
                        <Money value={collection.data?.overdue_base} />
                      </span>
                    )}
                  />
                </Col>
              </Row>
              <Typography.Text type="secondary" style={{ fontSize: 12.5 }}>
                回款率可能超过 100%：那是本月在还上个月的账，不是算错。
              </Typography.Text>
            </Space>
          </Card>

          <Card variant="borderless" title="常用入口" style={{ marginTop: 16 }}>
            <div className="quick-grid">
              {[
                { title: '报价单', sub: '成本快照与转单', icon: <FileTextOutlined />, go: () => create('quotations') },
                { title: '销售订单', sub: '确认即校验额度', icon: <InboxOutlined />, go: () => create('sales-orders') },
                { title: '采购订单', sub: '可挂销售单行', icon: <ShoppingCartOutlined />, go: () => create('purchase-orders') },
                { title: '出货单', sub: '选订单行装箱', icon: <SendOutlined />, go: () => create('shipments') },
                { title: '收款登记', sub: '核销应收', icon: <TransactionOutlined />, go: () => create('payments') },
                { title: '客户 360', sub: '订单 / 应收 / 跟进', icon: <GlobalOutlined />, go: () => list('customers') },
              ].map((item) => (
                <div
                  key={item.title}
                  className="quick-card"
                  role="button"
                  tabIndex={0}
                  onClick={item.go}
                  onKeyDown={(e) => {
                    if (e.key === 'Enter' || e.key === ' ') item.go()
                  }}
                >
                  <span className="quick-card-icon">{item.icon}</span>
                  <span className="quick-card-text">
                    <span className="quick-card-title">{item.title}</span>
                    <span className="quick-card-sub">{item.sub}</span>
                  </span>
                </div>
              ))}
            </div>
          </Card>
        </div>

        <div className="span-12">
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
                    <Typography.Link onClick={() => show('customers', Number(row.key))}>{value}</Typography.Link>
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
        </div>
      </div>
    </Space>
  )
}
