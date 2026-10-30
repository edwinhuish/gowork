import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import {
  Alert,
  Button,
  Card,
  Flex,
  Input,
  Modal,
  Radio,
  Space,
  Table,
  Tag,
  Typography,
  message,
  theme,
} from 'antd'

import { reportApi } from '@/api/reports'
import { Empty, Mono, Money } from '@/components/fields'
import { FilterBar, PageHeader } from '@/components/page'
import type { AgingRow } from '@/types'

/** 桶里的金额：有值才显示，0 一律给短横，避免整列看起来像满的。 */
function Bucket({ value, tone }: { value: string; tone?: 'warning' | 'error' }) {
  const { token } = theme.useToken()
  // 金额是定点字符串，这里的"有没有值"只决定显示短横还是数字。
  if (Number(value) <= 0.0001) return <Empty />
  const color =
    tone === 'error' ? token.colorError : tone === 'warning' ? token.colorWarning : undefined
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

  // 对账单的账期单独存：它是"要发给客户的那张纸"的参数，
  // 与上面那个"看账龄的截止日"不是同一件事，混用会让人以为改了其中一个另一个也变。
  const [stmt, setStmt] = useState<{ row: AgingRow; from: string; to: string; currency: string } | null>(
    null,
  )
  const [downloading, setDownloading] = useState(false)

  const openStatement = (row: AgingRow) => {
    // 截止日跟当前账龄对齐：屏幕上看到多少，纸上就该是多少。
    setStmt({ row, from: '', to: asOf || (data?.as_of ?? ''), currency: row.currency_code })
  }

  const downloadStatement = async () => {
    if (!stmt) return
    setDownloading(true)
    try {
      await reportApi.statementPdf({
        partner_id: stmt.row.partner_id,
        currency_code: stmt.currency.trim() || undefined,
        from: stmt.from.trim() || undefined,
        to: stmt.to.trim() || undefined,
      })
      setStmt(null)
    } catch (err) {
      message.error(err instanceof Error ? err.message : '生成对账单失败')
    } finally {
      setDownloading(false)
    }
  }

  return (
    <Flex vertical gap={16}>
      <PageHeader
        title="账龄"
        description="逾期天数 = 截止日 − 到期日，到期当天不算逾期；原币按主体 + 币种拆行，合计只给本币"
      />

      {error && <Alert type="error" showIcon message={(error as Error).message} />}

      <FilterBar
        extra={
          <Typography.Text type="secondary">
            截止 {data?.as_of ?? '—'} · {isPayable ? '我们欠供应商' : '客户欠我们'}
          </Typography.Text>
        }
      >
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
      </FilterBar>

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
            render={(_, row: AgingRow) => <Bucket value={row.amount.days_31_60} tone="warning" />}
          />
          <Table.Column
            title="61-90"
            width={120}
            align="right"
            render={(_, row: AgingRow) => <Bucket value={row.amount.days_61_90} tone="warning" />}
          />
          <Table.Column
            title="90 天以上"
            width={130}
            align="right"
            render={(_, row: AgingRow) => <Bucket value={row.amount.days_over_90} tone="error" />}
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
          <Table.Column
            title="对账单"
            width={96}
            render={(_, row: AgingRow) =>
              // 应付侧的对账单还没做（后端只有客户对账单）。摆一个点了才知道
              // 没有的按钮，比不摆更烦人。
              isPayable ? (
                <Empty />
              ) : (
                <Button size="small" type="link" onClick={() => openStatement(row)}>
                  对账单
                </Button>
              )
            }
          />
        </Table>

        <Typography.Paragraph type="secondary" style={{ marginTop: 12, marginBottom: 0 }}>
          口径：逾期天数 = 截止日 − 到期日，到期当天不算逾期；作废与已结清的单据不计入。
          原币金额按「主体 + 币种」拆行，合计只给本币——跨币种的原币相加没有意义。
        </Typography.Paragraph>
      </Card>

      <Modal
        open={!!stmt}
        title={stmt ? `${stmt.row.partner_name} · 客户对账单` : '客户对账单'}
        okText="下载 PDF"
        confirmLoading={downloading}
        onOk={downloadStatement}
        onCancel={() => setStmt(null)}
        destroyOnHidden
      >
        <Space direction="vertical" size={12} style={{ display: 'flex' }}>
          <Typography.Paragraph type="secondary" style={{ marginBottom: 0 }}>
            期初 + 本期开票 − 本期收款 = 期末，每行都带余额，客户可以照着单子自己验算。
          </Typography.Paragraph>
          <Space size={8}>
            <Typography.Text>币种</Typography.Text>
            <Input
              value={stmt?.currency ?? ''}
              onChange={(e) => setStmt((prev) => (prev ? { ...prev, currency: e.target.value } : prev))}
              placeholder="留空取第一个币种"
              style={{ width: 140 }}
            />
          </Space>
          <Space size={8}>
            <Typography.Text>账期</Typography.Text>
            <Input
              value={stmt?.from ?? ''}
              onChange={(e) => setStmt((prev) => (prev ? { ...prev, from: e.target.value } : prev))}
              placeholder="起 YYYY-MM-DD（留空不限）"
              style={{ width: 200 }}
            />
            <Input
              value={stmt?.to ?? ''}
              onChange={(e) => setStmt((prev) => (prev ? { ...prev, to: e.target.value } : prev))}
              placeholder="止 YYYY-MM-DD（留空不限）"
              style={{ width: 200 }}
            />
          </Space>
        </Space>
      </Modal>
    </Flex>
  )
}
