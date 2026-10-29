import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import {
  Alert,
  Card,
  Col,
  Input,
  Radio,
  Row,
  Select,
  Space,
  Statistic,
  Table,
  Tag,
  Typography,
} from 'antd'

import { reportApi } from '@/api/reports'
import { Empty, Mono, Money } from '@/components/fields'
import type { CollectionRow } from '@/types'

function pad(n: number) {
  return String(n).padStart(2, '0')
}

/** 默认看本月：回款率是期间指标，默认给"全历史"等于给一个读不出来的数。 */
function defaultPeriod() {
  const today = new Date()
  return {
    from: `${today.getFullYear()}-${pad(today.getMonth() + 1)}-01`,
    to: `${today.getFullYear()}-${pad(today.getMonth() + 1)}-${pad(today.getDate())}`,
  }
}

/** 逾期天数按严重程度上色：90 天以上在这行生意里基本等于坏账信号。 */
function overdueColor(days: number) {
  if (days > 90) return '#cf1322'
  if (days > 60) return '#d46b08'
  if (days > 30) return '#d48806'
  return undefined
}

/**
 * 回款与超期（T-29）。
 *
 * 页面上有两组数，含义不同，别互相校验：
 *   1 回款率是**期间流量**：分子是这段时间到账并核销的钱（按到账日），
 *     分母是这段时间到期的单据金额（按到期日）。收的钱可能在还上个月的账，
 *     所以比率大于 100% 是正常的。
 *   2 超期明细是**截至今天（as_of）的存量**：还没核销、已经过了到期日的单据，
 *     逐笔列出——催款是按单催的。
 */
export function CollectionPage() {
  const initial = defaultPeriod()
  const [direction, setDirection] = useState(10)
  const [fromInput, setFromInput] = useState(initial.from)
  const [toInput, setToInput] = useState(initial.to)
  const [period, setPeriod] = useState(initial)
  const [asOfInput, setAsOfInput] = useState('')
  const [asOf, setAsOf] = useState('')
  const [limit, setLimit] = useState(50)

  const { data, isLoading, error } = useQuery({
    queryKey: ['report-collection', direction, period, asOf, limit],
    queryFn: () =>
      reportApi.collection({
        direction,
        from: period.from || undefined,
        to: period.to || undefined,
        as_of: asOf || undefined,
        limit,
      }),
  })

  const rows = data?.overdue_rows ?? []
  const rate = Number(data?.collection_rate ?? 0)
  const isPayable = direction === 20

  return (
    <Space direction="vertical" size={16} style={{ display: 'flex' }}>
      {error && <Alert type="error" showIcon message={(error as Error).message} />}

      <Card variant="borderless">
        <Space wrap size={16} align="center">
          <Radio.Group
            optionType="button"
            value={direction}
            onChange={(e) => setDirection(e.target.value as number)}
            options={[
              { value: 10, label: '应收' },
              { value: 20, label: '应付' },
            ]}
          />
          <Space size={4}>
            <Input
              value={fromInput}
              onChange={(e) => setFromInput(e.target.value)}
              onPressEnter={() => setPeriod({ from: fromInput.trim(), to: toInput.trim() })}
              onBlur={() => setPeriod({ from: fromInput.trim(), to: toInput.trim() })}
              placeholder="起 YYYY-MM-DD"
              style={{ width: 150 }}
              allowClear
              onClear={() => {
                setFromInput('')
                setPeriod((prev) => ({ ...prev, from: '' }))
              }}
            />
            <Typography.Text type="secondary">至</Typography.Text>
            <Input
              value={toInput}
              onChange={(e) => setToInput(e.target.value)}
              onPressEnter={() => setPeriod({ from: fromInput.trim(), to: toInput.trim() })}
              onBlur={() => setPeriod({ from: fromInput.trim(), to: toInput.trim() })}
              placeholder="止 YYYY-MM-DD"
              style={{ width: 150 }}
              allowClear
              onClear={() => {
                setToInput('')
                setPeriod((prev) => ({ ...prev, to: '' }))
              }}
            />
          </Space>
          <Input
            value={asOfInput}
            onChange={(e) => setAsOfInput(e.target.value)}
            onPressEnter={() => setAsOf(asOfInput.trim())}
            onBlur={() => setAsOf(asOfInput.trim())}
            placeholder="超期截至日 YYYY-MM-DD（留空取今天）"
            style={{ width: 280 }}
            allowClear
            onClear={() => setAsOf('')}
          />
          <Select
            value={limit}
            onChange={setLimit}
            style={{ width: 150 }}
            options={[
              { value: 50, label: '前 50 张单' },
              { value: 100, label: '前 100 张单' },
              { value: 500, label: '前 500 张单' },
            ]}
          />
        </Space>
      </Card>

      <Card variant="borderless">
        <Row gutter={[24, 16]}>
          <Col xs={12} md={8} xl={4}>
            <Statistic
              title={`期间到期（${data?.from || '不限'} 起）`}
              value={Number(data?.due_amount_base ?? 0)}
              precision={2}
            />
          </Col>
          <Col xs={12} md={8} xl={4}>
            <Statistic
              title="期间回款"
              value={Number(data?.received_base ?? 0)}
              precision={2}
              valueStyle={{ color: '#389e0d' }}
            />
          </Col>
          <Col xs={12} md={8} xl={4}>
            <Statistic
              title="回款率"
              value={rate * 100}
              precision={2}
              suffix="%"
              valueStyle={{ color: rate < 0.8 ? '#d46b08' : '#389e0d' }}
            />
          </Col>
          <Col xs={12} md={8} xl={4}>
            <Statistic
              title="当前未结清"
              value={Number(data?.outstanding_base ?? 0)}
              precision={2}
            />
          </Col>
          <Col xs={12} md={8} xl={4}>
            <Statistic
              title="其中已超期"
              value={Number(data?.overdue_base ?? 0)}
              precision={2}
              valueStyle={{ color: Number(data?.overdue_base ?? 0) > 0 ? '#cf1322' : undefined }}
            />
          </Col>
          <Col xs={24} md={8} xl={4}>
            <Typography.Text type="secondary">截止 {data?.as_of || '今天'}（本币）</Typography.Text>
          </Col>
        </Row>

        <Typography.Paragraph type="secondary" style={{ marginTop: 16, marginBottom: 0 }}>
          回款率 = 期间<b>到账并核销</b>的金额 ÷ 期间<b>到期</b>的单据金额。
          分子按到账日、分母按到期日，收的钱可能是在还上个月的账，所以比率超过 100% 是正常的。
          分母为 0 的那一期没有比率可言，显示 0。
        </Typography.Paragraph>
      </Card>

      <Card
        variant="borderless"
        title={isPayable ? '超期未付明细' : '超期未收明细'}
        extra={
          <Space size={12}>
            <Tag color="orange">1-30 天 <Money value={data?.overdue_buckets.days_1_30} /></Tag>
            <Tag color="gold">31-60 天 <Money value={data?.overdue_buckets.days_31_60} /></Tag>
            <Tag color="volcano">61-90 天 <Money value={data?.overdue_buckets.days_61_90} /></Tag>
            <Tag color="red">90 天以上 <Money value={data?.overdue_buckets.days_over_90} /></Tag>
          </Space>
        }
      >
        {data?.truncated && (
          <Alert
            type="info"
            showIcon
            style={{ marginBottom: 12 }}
            message="只显示了一部分单据"
            description="清单按逾期天数排序截断；上面的「其中已超期」与分桶标签仍是全量。"
          />
        )}

        <Table<CollectionRow>
          dataSource={rows}
          rowKey="id"
          loading={isLoading}
          pagination={false}
          scroll={{ x: 1000 }}
          locale={{ emptyText: isPayable ? '没有超期未付的单据' : '没有超期未收的单据' }}
        >
          <Table.Column
            dataIndex="doc_no"
            title="单据号"
            width={160}
            render={(value: string) => <Mono>{value}</Mono>}
          />
          <Table.Column
            title={isPayable ? '供应商' : '客户'}
            width={220}
            render={(_, row: CollectionRow) => (
              <Space direction="vertical" size={0}>
                <span className="cell-strong">{row.partner_name || <Empty />}</span>
                <Mono>{row.partner_code}</Mono>
              </Space>
            )}
          />
          <Table.Column
            dataIndex="due_date"
            title="到期日"
            width={110}
            render={(value: string) => (value ? value.slice(0, 10) : <Empty />)}
          />
          <Table.Column
            dataIndex="overdue_days"
            title="逾期天数"
            width={100}
            align="right"
            defaultSortOrder="descend"
            sorter={(a: CollectionRow, b: CollectionRow) => a.overdue_days - b.overdue_days}
            render={(value: number) => (
              <span style={{ color: overdueColor(value), fontVariantNumeric: 'tabular-nums' }}>
                {value} 天
              </span>
            )}
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
            render={(_, row: CollectionRow) => <Money value={row.outstanding} />}
          />
          <Table.Column
            title="未结清（本币）"
            width={140}
            align="right"
            render={(_, row: CollectionRow) => (
              <span className="cell-strong">
                <Money value={row.outstanding_base} />
              </span>
            )}
          />
        </Table>

        <Typography.Paragraph type="secondary" style={{ marginTop: 12, marginBottom: 0 }}>
          只列<b>已过到期日且尚未结清</b>的单据，未到期的只计入上面的「当前未结清」。
          逾期天数 = 截止日 − 到期日，到期当天不算逾期；作废单据不计入。
          本币金额按单据上锁定的汇率折算，与账龄报表同一口径。
        </Typography.Paragraph>
      </Card>
    </Space>
  )
}
