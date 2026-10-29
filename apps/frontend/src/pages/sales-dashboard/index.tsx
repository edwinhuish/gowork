import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Alert, Card, Input, Radio, Select, Space, Table, Tag, Tooltip, Typography } from 'antd'

import { reportApi } from '@/api/reports'
import { Empty, Money } from '@/components/fields'
import type { SalesDashboardRow } from '@/types'

type Dimension = 'customer' | 'product' | 'country' | 'month'

const DIMENSION_OPTIONS: { value: Dimension; label: string }[] = [
  { value: 'customer', label: '客户' },
  { value: 'product', label: '产品' },
  { value: 'country', label: '国家' },
  { value: 'month', label: '月份' },
]

const DIMENSION_COLUMN: Record<Dimension, string> = {
  customer: '客户',
  product: '产品',
  country: '国家',
  month: '月份',
}

function pad(n: number) {
  return String(n).padStart(2, '0')
}

/** 默认看今年：把全部历史塞进第一屏，只会得到一个谁也读不出信息的总数。 */
function defaultRange() {
  const today = new Date()
  return {
    from: `${today.getFullYear()}-01-01`,
    to: `${today.getFullYear()}-${pad(today.getMonth() + 1)}-${pad(today.getDate())}`,
  }
}

/**
 * 毛利率：0.2254 → 22.54%。
 *
 * 收入为 0 的行给短横而不是 0.00%：那种行的毛利率算不出来（分母是 0），
 * 写成 0% 会被读成"这票没赚钱"。
 */
function MarginCell({ row }: { row: SalesDashboardRow }) {
  if (Number(row.revenue) === 0) return <Empty />
  const margin = Number(row.margin)
  return (
    <span
      style={{
        fontVariantNumeric: 'tabular-nums',
        color: margin < 0 ? '#cf1322' : margin > 0 ? '#389e0d' : undefined,
      }}
    >
      {(margin * 100).toFixed(2)}%
    </span>
  )
}

/** 毛利：负数标红，正负在一列里必须一眼分得出。 */
function ProfitCell({ value, strong }: { value: string; strong?: boolean }) {
  const num = Number(value)
  return (
    <span
      className={strong ? 'cell-strong' : undefined}
      style={{ color: num < -0.0001 ? '#cf1322' : undefined, fontVariantNumeric: 'tabular-nums' }}
    >
      <Money value={value} />
    </span>
  )
}

/**
 * 销售看板（T-28）。
 *
 * 这张表的价值全在"换维度不换数"：四个维度的合计必须一模一样，
 * 因为后端先把收入、成本、运杂费摊到**出货行**上、再换分组键。
 * 所以这里不做任何二次聚合，也不自己算毛利率——前端重算一遍就会出现两个毛利率。
 *
 * 订单额与收入是**两个口径**：前者按下单日、后者按出货日，行内相减没有意义，
 * 因此两列都保留、都不合并成"未出货"。
 */
export function SalesDashboardPage() {
  const initial = defaultRange()
  const [dimension, setDimension] = useState<Dimension>('customer')
  // 输入与已生效值分开：打字不触发查询，回车或失焦才生效。
  const [fromInput, setFromInput] = useState(initial.from)
  const [toInput, setToInput] = useState(initial.to)
  const [range, setRange] = useState(initial)
  const [limit, setLimit] = useState(20)

  const { data, isLoading, error } = useQuery({
    queryKey: ['report-sales-dashboard', dimension, range, limit],
    queryFn: () =>
      reportApi.salesDashboard({
        dimension,
        from: range.from || undefined,
        to: range.to || undefined,
        limit,
      }),
  })

  const rows = data?.rows ?? []
  const total = data?.total

  const applyRange = () => {
    setRange({ from: fromInput.trim(), to: toInput.trim() })
  }

  return (
    <Space direction="vertical" size={16} style={{ display: 'flex' }}>
      {error && <Alert type="error" showIcon message={(error as Error).message} />}

      <Card variant="borderless">
        <Space wrap size={16} align="center">
          <Radio.Group
            optionType="button"
            value={dimension}
            onChange={(e) => setDimension(e.target.value as Dimension)}
            options={DIMENSION_OPTIONS}
          />
          <Space size={4}>
            <Input
              value={fromInput}
              onChange={(e) => setFromInput(e.target.value)}
              onPressEnter={applyRange}
              onBlur={applyRange}
              placeholder="起 YYYY-MM-DD"
              style={{ width: 150 }}
              allowClear
              onClear={() => {
                setFromInput('')
                setRange((prev) => ({ ...prev, from: '' }))
              }}
            />
            <Typography.Text type="secondary">至</Typography.Text>
            <Input
              value={toInput}
              onChange={(e) => setToInput(e.target.value)}
              onPressEnter={applyRange}
              onBlur={applyRange}
              placeholder="止 YYYY-MM-DD"
              style={{ width: 150 }}
              allowClear
              onClear={() => {
                setToInput('')
                setRange((prev) => ({ ...prev, to: '' }))
              }}
            />
          </Space>
          <Select
            value={limit}
            onChange={setLimit}
            style={{ width: 140 }}
            options={[
              { value: 20, label: '前 20 行' },
              { value: 50, label: '前 50 行' },
              { value: 100, label: '前 100 行' },
              { value: 500, label: '前 500 行' },
            ]}
          />
          <Typography.Text type="secondary">
            {rows.length > 0
              ? `共 ${rows.length} 行 · ${data?.from || '不限'} ~ ${data?.to || '不限'}`
              : `${data?.from || '不限'} ~ ${data?.to || '不限'}`}
          </Typography.Text>
        </Space>
      </Card>

      {data?.truncated && (
        <Alert
          type="info"
          showIcon
          message="只显示了一部分行"
          description="表格按收入排序截断，底部的合计是全量——拿当前几十行自己加会少算。"
        />
      )}

      <Card variant="borderless" title={`按${DIMENSION_COLUMN[dimension]}看`}>
        <Table<SalesDashboardRow>
          dataSource={rows}
          rowKey="key"
          loading={isLoading}
          pagination={false}
          scroll={{ x: 1180 }}
          locale={{ emptyText: '范围内没有出货或订单' }}
          summary={() =>
            rows.length > 0 && total ? (
              <Table.Summary fixed>
                <Table.Summary.Row>
                  <Table.Summary.Cell index={0}>
                    <span className="cell-strong">全量合计</span>
                  </Table.Summary.Cell>
                  <Table.Summary.Cell index={1} align="right">
                    <Money value={total.order_amount} />
                  </Table.Summary.Cell>
                  <Table.Summary.Cell index={2} align="right">
                    <span className="cell-strong">
                      <Money value={total.revenue} />
                    </span>
                  </Table.Summary.Cell>
                  <Table.Summary.Cell index={3} align="right">
                    <Money value={total.cost} />
                  </Table.Summary.Cell>
                  <Table.Summary.Cell index={4} align="right">
                    <Money value={total.charge} />
                  </Table.Summary.Cell>
                  <Table.Summary.Cell index={5} align="right">
                    <ProfitCell value={total.profit} strong />
                  </Table.Summary.Cell>
                  <Table.Summary.Cell index={6} align="right">
                    <MarginCell row={total} />
                  </Table.Summary.Cell>
                  <Table.Summary.Cell index={7} align="right">
                    {total.shipments}
                  </Table.Summary.Cell>
                </Table.Summary.Row>
              </Table.Summary>
            ) : null
          }
        >
          <Table.Column
            dataIndex="label"
            title={DIMENSION_COLUMN[dimension]}
            width={240}
            render={(value: string, row: SalesDashboardRow) => (
              <Space size={4}>
                <span className={dimension === 'month' ? 'cell-mono' : undefined}>{value}</span>
                {row.cost_missing && (
                  <Tooltip title="这一行里有出货行没挂采购单，成本偏低，毛利因此偏高">
                    <Tag color="warning">成本不全</Tag>
                  </Tooltip>
                )}
              </Space>
            )}
          />
          <Table.Column
            title="订单额（按下单日）"
            width={150}
            align="right"
            sorter={(a: SalesDashboardRow, b: SalesDashboardRow) =>
              Number(a.order_amount) - Number(b.order_amount)
            }
            render={(_, row: SalesDashboardRow) =>
              Number(row.order_amount) === 0 ? <Empty /> : <Money value={row.order_amount} />
            }
          />
          <Table.Column
            title="收入（已出货）"
            width={150}
            align="right"
            defaultSortOrder="descend"
            sorter={(a: SalesDashboardRow, b: SalesDashboardRow) =>
              Number(a.revenue) - Number(b.revenue)
            }
            render={(_, row: SalesDashboardRow) => (
              <span className="cell-strong">
                <Money value={row.revenue} />
              </span>
            )}
          />
          <Table.Column
            title="采购成本"
            width={140}
            align="right"
            sorter={(a: SalesDashboardRow, b: SalesDashboardRow) => Number(a.cost) - Number(b.cost)}
            render={(_, row: SalesDashboardRow) => <Money value={row.cost} />}
          />
          <Table.Column
            title="运杂费"
            width={120}
            align="right"
            render={(_, row: SalesDashboardRow) =>
              Number(row.charge) === 0 ? <Empty /> : <Money value={row.charge} />
            }
          />
          <Table.Column
            title="毛利"
            width={140}
            align="right"
            sorter={(a: SalesDashboardRow, b: SalesDashboardRow) =>
              Number(a.profit) - Number(b.profit)
            }
            render={(_, row: SalesDashboardRow) => <ProfitCell value={row.profit} strong />}
          />
          <Table.Column
            title="毛利率"
            width={100}
            align="right"
            sorter={(a: SalesDashboardRow, b: SalesDashboardRow) =>
              Number(a.margin) - Number(b.margin)
            }
            render={(_, row: SalesDashboardRow) => <MarginCell row={row} />}
          />
          <Table.Column
            dataIndex="shipments"
            title="票数"
            width={80}
            align="right"
          />
        </Table>

        <Typography.Paragraph type="secondary" style={{ marginTop: 12, marginBottom: 0 }}>
          口径与单票毛利同源：收入按「本票数量 ÷ 销售行数量」分摊销售行本币金额，
          成本按采购加权单价 × 本票数量，运杂费按收入占比摊到各行，
          毛利 = 收入 − 采购成本 − 运杂费。汇率取单据上锁定的那个，不回头查汇率表。
          <br />
          订单额按<b>下单日</b>、其余按<b>出货日</b>，两者不是同一批单据，行内相减没有意义。
          草稿与已取消的单据不计入；汇兑损益单独成表，不混进毛利。
        </Typography.Paragraph>
      </Card>
    </Space>
  )
}
