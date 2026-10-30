import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import {
  Alert,
  Card,
  Flex,
  Input,
  Modal,
  Radio,
  Select,
  Space,
  Table,
  Tag,
  Tooltip,
  Typography,
  theme,
} from 'antd'

import { reportApi } from '@/api/reports'
import { Empty, Mono, Money } from '@/components/fields'
import { FilterBar, PageHeader } from '@/components/page'
import type { ProductProfitRow, SalesDashboardRow } from '@/types'

type Dimension = 'customer' | 'product' | 'category' | 'country' | 'month'

const DIMENSION_OPTIONS: { value: Dimension; label: string }[] = [
  { value: 'customer', label: '客户' },
  { value: 'product', label: '产品' },
  { value: 'category', label: '品类' },
  { value: 'country', label: '国家' },
  { value: 'month', label: '月份' },
]

const DIMENSION_COLUMN: Record<Dimension, string> = {
  customer: '客户',
  product: '产品',
  category: '品类',
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
  const { token } = theme.useToken()
  if (Number(row.revenue) === 0) return <Empty />
  const margin = Number(row.margin)
  return (
    <span
      style={{
        fontVariantNumeric: 'tabular-nums',
        color: margin < 0 ? token.colorError : margin > 0 ? token.colorSuccess : undefined,
      }}
    >
      {(margin * 100).toFixed(2)}%
    </span>
  )
}

/** 毛利：负数标红，正负在一列里必须一眼分得出。 */
function ProfitCell({ value, strong }: { value: string; strong?: boolean }) {
  const { token } = theme.useToken()
  const num = Number(value)
  return (
    <span
      className={strong ? 'cell-strong' : undefined}
      style={{
        color: num < -0.0001 ? token.colorError : undefined,
        fontVariantNumeric: 'tabular-nums',
      }}
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
  // 产品维度的下钻：点某个 SKU，看它逐票卖得怎么样。
  const [drill, setDrill] = useState<{ id: number; label: string } | null>(null)

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
    <Flex vertical gap={16}>
      <PageHeader
        title="销售看板"
        description="换维度不换数：收入、成本、运杂费先摊到出货行再分组，所以各维度合计一致；前端不做二次聚合"
      />

      {error && <Alert type="error" showIcon message={(error as Error).message} />}

      {/* T-36：成本与毛利在服务端就被清成了 0，这里必须说明，
          否则一屏的 0 会被读成"这批生意一分没赚"。 */}
      {data?.total.cost_hidden && (
        <Alert
          type="info"
          showIcon
          message="成本与毛利对当前账号不可见"
          description="订单额与收入照常显示；采购成本、运杂费、毛利与毛利率仅管理员与主管可见。"
        />
      )}

      <FilterBar
        extra={
          <Typography.Text type="secondary">
            {rows.length > 0
              ? `共 ${rows.length} 行 · ${data?.from || '不限'} ~ ${data?.to || '不限'}`
              : `${data?.from || '不限'} ~ ${data?.to || '不限'}`}
          </Typography.Text>
        }
      >
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
      </FilterBar>

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
                {dimension === 'product' ? (
                  // 只有产品维度能下钻：品类/客户/国家的"逐票"是另一个问题，
                  // 顺手都给会让人以为点进去能看到该品类每票的毛利，实际只能分摊。
                  <Typography.Link
                    onClick={() => setDrill({ id: Number(row.key), label: value })}
                  >
                    {value}
                  </Typography.Link>
                ) : (
                  <span className={dimension === 'month' ? 'cell-mono' : undefined}>{value}</span>
                )}
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
            render={(_, row: SalesDashboardRow) =>
              // T-36：成本被服务端清掉时显示「—」，不能显示 0——
              // 0 会被读成"这批货没成本"。
              row.cost_hidden ? <span className="cell-sub">—</span> : <Money value={row.cost} />
            }
          />
          <Table.Column
            title="运杂费"
            width={120}
            align="right"
            render={(_, row: SalesDashboardRow) =>
              row.cost_hidden ? (
                <span className="cell-sub">—</span>
              ) : Number(row.charge) === 0 ? (
                <Empty />
              ) : (
                <Money value={row.charge} />
              )
            }
          />
          <Table.Column
            title="毛利"
            width={140}
            align="right"
            sorter={(a: SalesDashboardRow, b: SalesDashboardRow) =>
              Number(a.profit) - Number(b.profit)
            }
            render={(_, row: SalesDashboardRow) =>
              row.cost_hidden ? (
                <span className="cell-sub">—</span>
              ) : (
                <ProfitCell value={row.profit} strong />
              )
            }
          />
          <Table.Column
            title="毛利率"
            width={100}
            align="right"
            sorter={(a: SalesDashboardRow, b: SalesDashboardRow) =>
              Number(a.margin) - Number(b.margin)
            }
            render={(_, row: SalesDashboardRow) =>
              row.cost_hidden ? <span className="cell-sub">—</span> : <MarginCell row={row} />
            }
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

      {drill && (
        <ProductDrillModal
          productId={drill.id}
          label={drill.label}
          range={range}
          onClose={() => setDrill(null)}
        />
      )}
    </Flex>
  )
}

/**
 * 产品毛利下钻：某个 SKU 逐票的收入、成本、摊到的运杂费与毛利。
 *
 * 行按**票**聚合（不是按订单行）：分批出货时同一产品在一票里可能出现两次，
 * 拆成两行就看不出"这一票赚了多少"。
 */
function ProductDrillModal({
  productId,
  label,
  range,
  onClose,
}: {
  productId: number
  label: string
  range: { from: string; to: string }
  onClose: () => void
}) {
  const { data, isLoading, error } = useQuery({
    queryKey: ['report-product-profit', productId, range],
    queryFn: () =>
      reportApi.productProfit({
        product_id: productId,
        from: range.from || undefined,
        to: range.to || undefined,
      }),
  })

  return (
    <Modal
      open
      title={`${label} · 逐票毛利`}
      footer={null}
      width={980}
      onCancel={onClose}
      destroyOnHidden
    >
      {error && <Alert type="error" showIcon message={(error as Error).message} />}

      <Table<ProductProfitRow>
        size="small"
        rowKey="shipment_id"
        loading={isLoading}
        dataSource={data?.rows ?? []}
        pagination={false}
        scroll={{ x: 900 }}
        locale={{ emptyText: '范围内没有这个产品的出货' }}
        summary={() =>
          data && data.rows.length > 0 ? (
            <Table.Summary.Row>
              <Table.Summary.Cell index={0}>
                <span className="cell-strong">合计</span>
              </Table.Summary.Cell>
              <Table.Summary.Cell index={1} />
              <Table.Summary.Cell index={2} align="right">
                <Money value={data.revenue} />
              </Table.Summary.Cell>
              <Table.Summary.Cell index={3} align="right">
                <Money value={data.cost} />
              </Table.Summary.Cell>
              <Table.Summary.Cell index={4} align="right">
                <Money value={data.charge} />
              </Table.Summary.Cell>
              <Table.Summary.Cell index={5} align="right">
                <ProfitCell value={data.profit} strong />
              </Table.Summary.Cell>
            </Table.Summary.Row>
          ) : null
        }
      >
        <Table.Column
          dataIndex="doc_no"
          title="出货单"
          width={170}
          render={(value: string) => <Mono>{value}</Mono>}
        />
        <Table.Column dataIndex="ship_date" title="出货日" width={110} />
        <Table.Column
          title="客户"
          width={200}
          render={(_, row: ProductProfitRow) => row.partner_name || <Empty />}
        />
        <Table.Column
          title="收入"
          width={130}
          align="right"
          render={(_, row: ProductProfitRow) => <Money value={row.revenue} />}
        />
        <Table.Column
          title="成本"
          width={130}
          align="right"
          render={(_, row: ProductProfitRow) => <Money value={row.cost} />}
        />
        <Table.Column
          title="运杂费"
          width={110}
          align="right"
          render={(_, row: ProductProfitRow) => <Money value={row.charge} />}
        />
        <Table.Column
          title="毛利"
          width={130}
          align="right"
          render={(_, row: ProductProfitRow) => <ProfitCell value={row.profit} strong />}
        />
      </Table>

      <Typography.Paragraph type="secondary" style={{ marginTop: 12, marginBottom: 0 }}>
        与看板同一份分摊：收入按本票数量占销售行的比例、成本按采购加权单价、
        运杂费按该产品在本票的收入占比摊。成本不全的票，毛利会偏高。
      </Typography.Paragraph>
    </Modal>
  )
}
