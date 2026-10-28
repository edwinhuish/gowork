import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Alert, Card, Input, Radio, Space, Table, Tag, Typography } from 'antd'

import { reportApi } from '@/api/reports'
import { Empty, Mono, Money } from '@/components/fields'
import type { AgingRow } from '@/types'

/** 桶里的金额：有值才显示，0 一律给短横，避免整列看起来像满的。 */
function Bucket({ value, color }: { value: number; color?: string }) {
  if (value <= 0.0001) return <Empty />
  return (
    <span style={color ? { color } : undefined}>
      <Money value={value} />
    </span>
  )
}

/**
 * 账龄报表。
 *
 * 这是聚合查询而不是 CRUD 资源，所以没走 dataProvider，直接用 react-query 调
 * reportApi（与概览页同一个套路）。
 *
 * 分桶边界不在前端重算：后端给什么桶就显示什么桶。前端再按"今天"算一遍，
 * 一旦跨零点或者两边时区不同，同一批数据就会出现两个账龄，而客户只认其中一个。
 */
export function AgingPage() {
  const [direction, setDirection] = useState(10)
  // 输入与已生效的截止日分开：打字不该触发一次查询，回车或失焦才生效。
  const [asOfInput, setAsOfInput] = useState('')
  const [asOf, setAsOf] = useState('')

  const { data, isLoading, error } = useQuery({
    queryKey: ['report-aging', direction, asOf],
    queryFn: () => reportApi.aging({ direction, as_of: asOf || undefined }),
  })

  const rows = data?.rows ?? []
  const total = data?.total_base
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
          <Input
            value={asOfInput}
            onChange={(e) => setAsOfInput(e.target.value)}
            onPressEnter={() => setAsOf(asOfInput.trim())}
            onBlur={() => setAsOf(asOfInput.trim())}
            placeholder="截止日 YYYY-MM-DD（留空取今天）"
            style={{ width: 260 }}
            allowClear
            // 清空输入框后按回车即回到"今天"，否则会一直停在旧截止日。
            onClear={() => setAsOf('')}
          />
          <Typography.Text type="secondary">
            截止 {data?.as_of ?? '—'} · {isPayable ? '我们欠供应商' : '客户欠我们'}
          </Typography.Text>
        </Space>
      </Card>

      <Card variant="borderless" title={isPayable ? '按供应商的账龄' : '按客户的账龄'}>
        <Table<AgingRow>
          dataSource={rows}
          rowKey={(row) => `${row.partner_id}-${row.currency_code}`}
          loading={isLoading}
          pagination={false}
          scroll={{ x: 1200 }}
          locale={{ emptyText: '没有未结清的单据' }}
          summary={() =>
            rows.length > 0 ? (
              <Table.Summary fixed>
                <Table.Summary.Row>
                  <Table.Summary.Cell index={0} colSpan={2}>
                    <span className="cell-strong">本币合计</span>
                  </Table.Summary.Cell>
                  <Table.Summary.Cell index={2} align="right">
                    <Money value={total?.not_due} />
                  </Table.Summary.Cell>
                  <Table.Summary.Cell index={3} align="right">
                    <Money value={total?.days_1_30} />
                  </Table.Summary.Cell>
                  <Table.Summary.Cell index={4} align="right">
                    <Money value={total?.days_31_60} />
                  </Table.Summary.Cell>
                  <Table.Summary.Cell index={5} align="right">
                    <Money value={total?.days_61_90} />
                  </Table.Summary.Cell>
                  <Table.Summary.Cell index={6} align="right">
                    <Money value={total?.days_over_90} />
                  </Table.Summary.Cell>
                  {/* 原币合计跨币种没有意义，留空而不是给一个假数。 */}
                  <Table.Summary.Cell index={7} align="right">
                    <Empty />
                  </Table.Summary.Cell>
                  <Table.Summary.Cell index={8} align="right">
                    <span className="cell-strong">
                      <Money value={data?.grand_total_base} />
                    </span>
                  </Table.Summary.Cell>
                  <Table.Summary.Cell index={9} />
                </Table.Summary.Row>
              </Table.Summary>
            ) : null
          }
        >
          <Table.Column
            title={isPayable ? '供应商' : '客户'}
            width={200}
            render={(_, row: AgingRow) => (
              <Space direction="vertical" size={0}>
                <span className="cell-strong">{row.partner_name}</span>
                <Mono>{row.partner_code}</Mono>
              </Space>
            )}
          />
          <Table.Column
            dataIndex="currency_code"
            title="币种"
            width={80}
            render={(value: string) => <Tag>{value}</Tag>}
          />
          <Table.Column
            title="未到期"
            width={120}
            align="right"
            render={(_, row: AgingRow) => <Bucket value={row.amount.not_due} />}
          />
          <Table.Column
            title="逾期 1-30"
            width={120}
            align="right"
            render={(_, row: AgingRow) => <Bucket value={row.amount.days_1_30} />}
          />
          <Table.Column
            title="31-60"
            width={120}
            align="right"
            render={(_, row: AgingRow) => <Bucket value={row.amount.days_31_60} color="#d48806" />}
          />
          <Table.Column
            title="61-90"
            width={120}
            align="right"
            render={(_, row: AgingRow) => <Bucket value={row.amount.days_61_90} color="#d46b08" />}
          />
          <Table.Column
            title="90 天以上"
            width={130}
            align="right"
            render={(_, row: AgingRow) => <Bucket value={row.amount.days_over_90} color="#cf1322" />}
          />
          <Table.Column
            title="合计（原币）"
            width={140}
            align="right"
            render={(_, row: AgingRow) => (
              <span className="cell-strong">
                <Money value={row.total} />
              </span>
            )}
          />
          <Table.Column
            title="合计（本币）"
            width={140}
            align="right"
            render={(_, row: AgingRow) => (
              <span className="cell-strong">
                <Money value={row.total_base} />
              </span>
            )}
          />
          <Table.Column
            dataIndex="earliest_due_date"
            title="最早到期"
            width={120}
            render={(value: string) => (value ? value.slice(0, 10) : <Empty />)}
          />
        </Table>

        <Typography.Paragraph type="secondary" style={{ marginTop: 12, marginBottom: 0 }}>
          口径：逾期天数 = 截止日 − 到期日，到期当天不算逾期；作废与已结清的单据不计入。
          原币金额按「主体 + 币种」拆行，合计只给本币——跨币种的原币相加没有意义。
        </Typography.Paragraph>
      </Card>
    </Space>
  )
}
