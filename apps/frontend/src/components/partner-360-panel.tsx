import { useQuery } from '@tanstack/react-query'
import { useNavigation } from '@refinedev/core'
import { Alert, Card, Col, Row, Space, Statistic, Table, Tabs, Tag, Typography } from 'antd'

import { reportApi } from '@/api/reports'
import { Empty, Mono, Money } from '@/components/fields'
import { activityTypeLabel, opportunityStageLabel, orderStatusLabel } from '@/constants/catalog'
import type {
  Partner360Activity,
  Partner360Opportunity,
  Partner360Order,
  Partner360Receivable,
  Partner360Shipment,
} from '@/types'

/**
 * 客户 360（T-30）。
 *
 * 为什么挂在详情页而不是做成一个独立页面：这些数字是"看这家客户时顺手要知道的"，
 * 让它跳到另一页，等于每看一家客户都要重新找一遍他在哪。
 *
 * 五块明细各自带 id / 单号，点一下回源单据；抬头合计是全量，
 * 不受每块 10 条的截断影响——截断只影响"最近几条"。
 */
export function Partner360Panel({ partnerId }: { partnerId: number }) {
  const { show, edit } = useNavigation()
  const { data, isLoading, error } = useQuery({
    queryKey: ['report-partner-360', partnerId],
    queryFn: () => reportApi.partner360({ partner_id: partnerId }),
  })

  const totals = data?.totals
  const overdue = Number(totals?.receivable_overdue_base ?? 0)

  return (
    <Card
      variant="borderless"
      title="客户 360"
      style={{ marginTop: 16 }}
      extra={<span className="cell-sub">截止 {data?.as_of ?? '今天'}（本币）</span>}
    >
      {error && <Alert type="error" showIcon message={(error as Error).message} />}

      <Row gutter={[24, 16]}>
        <Col xs={12} md={8} xl={4}>
          <Statistic
            title={`累计订单额（${totals?.order_count ?? 0} 张）`}
            value={Number(totals?.order_amount_base ?? 0)}
            precision={2}
          />
        </Col>
        <Col xs={12} md={8} xl={4}>
          <Statistic
            title="未收应收"
            value={Number(totals?.receivable_outstanding_base ?? 0)}
            precision={2}
          />
        </Col>
        <Col xs={12} md={8} xl={4}>
          <Statistic
            title="其中已超期"
            value={overdue}
            precision={2}
            valueStyle={{ color: overdue > 0 ? 'var(--ant-color-error)' : undefined }}
          />
        </Col>
        <Col xs={12} md={8} xl={4}>
          <Statistic
            title={`在谈商机（${totals?.open_opportunity_count ?? 0} 条）`}
            value={Number(totals?.open_opportunity_amount_base ?? 0)}
            precision={2}
          />
        </Col>
        <Col xs={12} md={8} xl={4}>
          <Statistic title="出货票数" value={totals?.shipment_count ?? 0} />
        </Col>
        <Col xs={12} md={8} xl={4}>
          <Statistic
            title={`跟进（${totals?.activity_count ?? 0} 条）`}
            value={totals?.last_activity_at || '—'}
            valueStyle={{ fontSize: 16 }}
          />
        </Col>
      </Row>

      <Tabs
        style={{ marginTop: 8 }}
        items={[
          {
            key: 'orders',
            label: '最近订单',
            children: (
              <Table<Partner360Order>
                size="small"
                rowKey="id"
                loading={isLoading}
                dataSource={data?.orders ?? []}
                pagination={false}
                locale={{ emptyText: '还没有订单' }}
              >
                <Table.Column
                  dataIndex="doc_no"
                  title="单号"
                  width={160}
                  render={(value: string, row: Partner360Order) => (
                    <Typography.Link onClick={() => show('sales-orders', row.id)}>
                      {value}
                    </Typography.Link>
                  )}
                />
                <Table.Column dataIndex="order_date" title="下单日" width={110} />
                <Table.Column
                  dataIndex="status"
                  title="状态"
                  width={100}
                  render={(value: number) => <Tag>{orderStatusLabel(value)}</Tag>}
                />
                <Table.Column
                  dataIndex="currency_code"
                  title="币种"
                  width={80}
                  render={(value: string) => <Tag>{value}</Tag>}
                />
                <Table.Column
                  title="本币金额"
                  align="right"
                  render={(_, row: Partner360Order) => <Money value={row.amount_base_total} />}
                />
              </Table>
            ),
          },
          {
            key: 'receivables',
            label: '未结清应收',
            children: (
              <Table<Partner360Receivable>
                size="small"
                rowKey="id"
                loading={isLoading}
                dataSource={data?.receivables ?? []}
                pagination={false}
                locale={{ emptyText: '没有未结清的应收' }}
              >
                <Table.Column
                  dataIndex="doc_no"
                  title="单号"
                  width={160}
                  render={(value: string, row: Partner360Receivable) => (
                    <Typography.Link onClick={() => show('receivables', row.id)}>
                      {value}
                    </Typography.Link>
                  )}
                />
                <Table.Column dataIndex="due_date" title="到期日" width={110} />
                <Table.Column
                  dataIndex="overdue_days"
                  title="逾期"
                  width={90}
                  render={(value: number) =>
                    value > 0 ? <Tag color="red">{value} 天</Tag> : <Tag>未到期</Tag>
                  }
                />
                <Table.Column
                  dataIndex="currency_code"
                  title="币种"
                  width={80}
                  render={(value: string) => <Tag>{value}</Tag>}
                />
                <Table.Column
                  title="未结清（原币）"
                  width={140}
                  align="right"
                  render={(_, row: Partner360Receivable) => <Money value={row.outstanding} />}
                />
                <Table.Column
                  title="未结清（本币）"
                  width={140}
                  align="right"
                  render={(_, row: Partner360Receivable) => (
                    <span className="cell-strong">
                      <Money value={row.outstanding_base} />
                    </span>
                  )}
                />
              </Table>
            ),
          },
          {
            key: 'activities',
            label: '最近跟进',
            children: (
              <Table<Partner360Activity>
                size="small"
                rowKey="id"
                loading={isLoading}
                dataSource={data?.activities ?? []}
                pagination={false}
                locale={{ emptyText: '还没有跟进记录' }}
              >
                <Table.Column dataIndex="occurred_at" title="日期" width={110} />
                <Table.Column
                  dataIndex="type"
                  title="方式"
                  width={90}
                  render={(value: string) => <Tag>{activityTypeLabel(value)}</Tag>}
                />
                <Table.Column
                  dataIndex="subject"
                  title="主题"
                  render={(value: string, row: Partner360Activity) => (
                    <Typography.Link onClick={() => edit('activities', row.id)}>
                      {value || <Empty />}
                    </Typography.Link>
                  )}
                />
                <Table.Column
                  dataIndex="next_follow_at"
                  title="下次跟进"
                  width={120}
                  render={(value: string) => value || <Empty />}
                />
              </Table>
            ),
          },
          {
            key: 'opportunities',
            label: '在谈商机',
            children: (
              <Table<Partner360Opportunity>
                size="small"
                rowKey="id"
                loading={isLoading}
                dataSource={data?.opportunities ?? []}
                pagination={false}
                locale={{ emptyText: '没有在谈的商机' }}
              >
                <Table.Column
                  dataIndex="subject"
                  title="商机"
                  render={(value: string, row: Partner360Opportunity) => (
                    <Typography.Link onClick={() => edit('opportunities', row.id)}>
                      {value || <Empty />}
                    </Typography.Link>
                  )}
                />
                <Table.Column
                  dataIndex="stage"
                  title="阶段"
                  width={110}
                  render={(value: number) => <Tag>{opportunityStageLabel(value)}</Tag>}
                />
                <Table.Column
                  dataIndex="probability"
                  title="胜率"
                  width={80}
                  align="right"
                  render={(value: number) => `${value}%`}
                />
                <Table.Column
                  title="本币金额"
                  width={140}
                  align="right"
                  render={(_, row: Partner360Opportunity) => <Money value={row.amount_base} />}
                />
                <Table.Column dataIndex="expected_close_date" title="预计成交" width={120} />
              </Table>
            ),
          },
          {
            key: 'shipments',
            label: '最近出货',
            children: (
              <Table<Partner360Shipment>
                size="small"
                rowKey="id"
                loading={isLoading}
                dataSource={data?.shipments ?? []}
                pagination={false}
                locale={{ emptyText: '还没有出货' }}
              >
                <Table.Column
                  dataIndex="doc_no"
                  title="出货单"
                  width={180}
                  render={(value: string, row: Partner360Shipment) => (
                    <Space size={4}>
                      <Typography.Link onClick={() => show('shipments', row.id)}>
                        <Mono>{value}</Mono>
                      </Typography.Link>
                    </Space>
                  )}
                />
                <Table.Column dataIndex="ship_date" title="出货日" width={120} />
                <Table.Column
                  dataIndex="status"
                  title="状态"
                  width={110}
                  render={(value: number) => <Tag>{orderStatusLabel(value)}</Tag>}
                />
              </Table>
            ),
          },
        ]}
      />
    </Card>
  )
}
